package sftp

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"net"
	"strings"
	"sync"
)

// SFTPConn is a wrapper around net.Conn that implements io.ReadWriteCloser
// for the sftp.NewClientPipe. It handles Knot protocol messages.
type SFTPConn struct {
	Conn       net.Conn
	DataCh     chan []byte
	ErrCh      chan error
	Ready      chan struct{} // Closed when "ok" is received
	Closed     chan struct{}
	StartOnce  sync.Once
	CloseOnce  sync.Once
	Buf        []byte
	Interactive bool // If true, handles HostKeyConfirm interactively
	AuthHandler func(challenge protocol.AuthChallengePayload) (*protocol.AuthResponsePayload, error)
}

func (s *SFTPConn) Start() {
	s.StartOnce.Do(func() {
		s.DataCh = make(chan []byte, 100)
		s.ErrCh = make(chan error, 1)
		s.Ready = make(chan struct{})
		s.Closed = make(chan struct{})
		go func() {
			handshakeDone := false
			for {
				msg, err := protocol.ReadMessage(s.Conn)
				if err != nil {
					if !handshakeDone {
						// Ensure Ready is not blocked if we fail during handshake
						close(s.Ready)
					}
					select {
					case s.ErrCh <- err:
					case <-s.Closed:
					}
					return
				}

				switch msg.Header.Type {
				case protocol.TypeResp:
					resp := string(msg.Payload)
					if resp == "ok" {
						handshakeDone = true
						close(s.Ready)
					} else {
						err := fmt.Errorf("daemon error: %s", resp)
						if strings.HasPrefix(resp, "error: ") {
							err = fmt.Errorf("daemon error: %s", resp[7:])
						}
						if !handshakeDone {
							s.ErrCh <- err
							close(s.Ready)
							return
						}
					}
				case protocol.TypeData:
					data := make([]byte, len(msg.Payload))
					copy(data, msg.Payload)
					select {
					case s.DataCh <- data:
					case <-s.Closed:
						return
					}
				case protocol.TypeDisconnect:
					err := fmt.Errorf("disconnected: %s", string(msg.Payload))
					if !handshakeDone {
						s.ErrCh <- err
						close(s.Ready)
					} else {
						select {
						case s.ErrCh <- err:
						case <-s.Closed:
						}
					}
					return
				case protocol.TypeAuthChallenge:
					if s.AuthHandler != nil {
						var challenge protocol.AuthChallengePayload
						if err := json.Unmarshal(msg.Payload, &challenge); err != nil {
							_ = protocol.WriteMessage(s.Conn, protocol.TypeAuthRetryAbort, 0, nil)
							continue
						}
						resp, err := s.AuthHandler(challenge)
						if err != nil {
							_ = protocol.WriteMessage(s.Conn, protocol.TypeAuthRetryAbort, 0, nil)
							continue
						}
						payload, _ := json.Marshal(resp)
						_ = protocol.WriteMessage(s.Conn, protocol.TypeAuthResponse, 0, payload)
					} else {
						_ = protocol.WriteMessage(s.Conn, protocol.TypeAuthRetryAbort, 0, nil)
					}
				case protocol.TypeHostKeyConfirm:
					if s.Interactive {
						fmt.Printf("\n%s ", string(msg.Payload))
						var response string
						if _, err := fmt.Scanln(&response); err != nil {
							response = "no"
						}
						_ = protocol.WriteMessage(s.Conn, protocol.TypeHostKeyConfirm, 0, []byte(response))
					} else {
						err := fmt.Errorf("host key verification failed. Run 'knot ssh' first to accept the key")
						if !handshakeDone {
							s.ErrCh <- err
							close(s.Ready)
						} else {
							select {
							case s.ErrCh <- err:
							case <-s.Closed:
							}
						}
						return
					}
				}
			}
		}()
	})
}

func (s *SFTPConn) Read(p []byte) (int, error) {
	s.Start()
	// Wait for handshake to complete before first read
	<-s.Ready

	if len(s.Buf) > 0 {
		n := copy(p, s.Buf)
		s.Buf = s.Buf[n:]
		return n, nil
	}

	select {
	case data, ok := <-s.DataCh:
		if !ok {
			return 0, net.ErrClosed
		}
		n := copy(p, data)
		if n < len(data) {
			s.Buf = data[n:]
		}
		return n, nil
	case err := <-s.ErrCh:
		return 0, err
	case <-s.Closed:
		return 0, net.ErrClosed
	}
}

func (s *SFTPConn) Write(p []byte) (int, error) {
	s.Start()
	// Wait for handshake to complete before first write
	<-s.Ready

	// Subtype 0 is used for SFTP data (standard)
	if err := protocol.WriteMessage(s.Conn, protocol.TypeData, 0, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *SFTPConn) Close() error {
	s.CloseOnce.Do(func() {
		close(s.Closed)
	})
	return nil
}
