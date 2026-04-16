package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"knot/internal/protocol"
	"knot/pkg/daemon"
	knotsftp "knot/pkg/sftp"
	"knot/internal/logger"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
)

type knotSFTPConn struct {
	conn      net.Conn
	buf       []byte
	cwdCh     chan string
	dataCh    chan []byte
	errCh     chan error
	startOnce sync.Once
	closeOnce sync.Once
	closed    chan struct{}
}

func (k *knotSFTPConn) start() {
	k.startOnce.Do(func() {
		k.dataCh = make(chan []byte, 100)
		k.errCh = make(chan error, 1)
		k.closed = make(chan struct{})
		go func() {
			for {
				msg, err := protocol.ReadMessage(k.conn)
				if err != nil {
					select {
					case k.errCh <- err:
					case <-k.closed:
					}
					return
				}

				switch msg.Header.Type {
				case protocol.TypeDisconnect:
					fmt.Printf("\r\n[knot] %s\r\n", string(msg.Payload))
					select {
					case k.errCh <- fmt.Errorf("disconnected"):
					case <-k.closed:
					}
					return
				case protocol.TypeData:
					data := make([]byte, len(msg.Payload))
					copy(data, msg.Payload)
					select {
					case k.dataCh <- data:
					case <-k.closed:
						return
					}
				case protocol.TypeHostKeyConfirm:
					fmt.Printf("\n%s ", string(msg.Payload))
					var response string
					if _, err := fmt.Scanln(&response); err != nil {
						response = "no"
					}
					protocol.WriteMessage(k.conn, protocol.TypeHostKeyConfirm, 0, []byte(response))
				case protocol.TypeCWDUpdate:
					if k.cwdCh != nil {
						select {
						case k.cwdCh <- string(msg.Payload):
						default:
						}
					}
				case protocol.TypeSignal:
					continue
				default:
					logger.Warn("unexpected message type", "type", msg.Header.Type)
				}
			}
		}()
	})
}

func (k *knotSFTPConn) Read(p []byte) (n int, err error) {
	k.start()
	if len(k.buf) > 0 {
		n = copy(p, k.buf)
		k.buf = k.buf[n:]
		return n, nil
	}

	select {
	case data, ok := <-k.dataCh:
		if !ok {
			return 0, io.EOF
		}
		n = copy(p, data)
		if n < len(data) {
			k.buf = data[n:]
		}
		return n, nil
	case err := <-k.errCh:
		return 0, err
	case <-k.closed:
		return 0, io.EOF
	}
}

func (k *knotSFTPConn) Write(p []byte) (n int, err error) {
	err = protocol.WriteMessage(k.conn, protocol.TypeData, 0, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (k *knotSFTPConn) Close() error {
	k.start() // Ensure channels are initialized
	k.closeOnce.Do(func() {
		close(k.closed)
	})
	return k.conn.Close()
}

var sftpCmd = &cobra.Command{
	Use:           "sftp [alias]",
	Short:         "Interactive SFTP shell",
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]
		if len(alias) > 255 {
			return fmt.Errorf("alias too long")
		}
		follow, _ := cmd.Flags().GetBool("follow")

		client, err := daemon.NewClient()
		if err != nil {
			return err
		}

		conn, err := client.ConnectWithAutoStart()
		if err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer conn.Close()

		var initialDir string
		var sessionID string
		if follow {
			// 1. Get sessions for this alias
			if err := protocol.WriteMessage(conn, protocol.TypeSessionListReq, 0, []byte(alias)); err != nil {
				return err
			}

			// Add timeout for session list
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			msg, err := protocol.ReadMessage(conn)
			conn.SetReadDeadline(time.Time{})
			if err != nil {
				return fmt.Errorf("failed to get sessions from daemon: %w", err)
			}
			var sessions []*daemon.Session
			if err := json.Unmarshal(msg.Payload, &sessions); err != nil {
				return fmt.Errorf("failed to parse sessions: %w", err)
			}

			if len(sessions) == 0 {
				fmt.Printf("No active SSH sessions found for alias '%s'. Connecting to home directory.\n", alias)
			} else {
				var selected *daemon.Session
				if len(sessions) == 1 {
					selected = sessions[0]
				} else if len(sessions) > 0 {
					fmt.Printf("Active SSH sessions for '%s':\n", alias)
					for i, s := range sessions {
						dir := s.CurrentDir
						if dir == "" {
							dir = "(unknown)"
						}
						fmt.Printf("[%d] ID: %s, CWD: %s\n", i+1, s.ID, dir)
					}
					fmt.Print("Select session to follow (1-n, or 0 for none): ")
					var choice int
					if _, err := fmt.Scanln(&choice); err != nil {
						choice = 0
					}
					if choice > 0 && choice <= len(sessions) {
						selected = sessions[choice-1]
					}
				}

				if selected != nil {
					sessionID = selected.ID
					if selected.CurrentDir != "" {
						initialDir = selected.CurrentDir
						fmt.Printf("Following session %s, starting at %s\n", selected.ID, initialDir)
					} else {
						fmt.Printf("Following session %s, starting at remote home\n", selected.ID)
					}
				}
			}
		}

		// Send SFTP request
		sftpReqPayload := alias
		if sessionID != "" {
			sftpReqPayload = fmt.Sprintf("%s:%s", alias, sessionID)
		}
		if err := protocol.WriteMessage(conn, protocol.TypeSFTPReq, 0, []byte(sftpReqPayload)); err != nil {
			return err
		}

		// Wait for ok/error
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			return err
		}

		// Handle potential host key confirmation before "ok"
		for msg.Header.Type == protocol.TypeHostKeyConfirm {
			fmt.Printf("\n%s ", string(msg.Payload))
			var response string
			if _, err := fmt.Scanln(&response); err != nil {
				response = "no"
			}
			protocol.WriteMessage(conn, protocol.TypeHostKeyConfirm, 0, []byte(response))
			msg, err = protocol.ReadMessage(conn)
			if err != nil {
				return err
			}
		}

		if msg.Header.Type != protocol.TypeResp {
			return fmt.Errorf("unexpected response type: %d", msg.Header.Type)
		}

		resp := string(msg.Payload)
		if resp != "ok" {
			if strings.HasPrefix(resp, "error: ") {
				return fmt.Errorf("daemon error: %s", resp[7:])
			}
			return fmt.Errorf("daemon error: %s", resp)
		}

		// Create SFTP client using the proxied connection
		cwdCh := make(chan string, 1)
		sftpConn := &knotSFTPConn{conn: conn, cwdCh: cwdCh}
		sftpClient, err := sftp.NewClientPipe(sftpConn, sftpConn)
		if err != nil {
			return fmt.Errorf("failed to create sftp client: %w", err)
		}
		defer sftpClient.Close()

		err = knotsftp.RunREPL(sftpClient, alias, initialDir, cwdCh)
		if err != nil && err.Error() == "disconnected" {
			return nil
		}
		return err
	},
}

func init() {
	sftpCmd.Flags().BoolP("follow", "f", false, "Follow an active SSH session directory")
	sftpCmd.GroupID = basicGroup.ID
	rootCmd.AddCommand(sftpCmd)
}
