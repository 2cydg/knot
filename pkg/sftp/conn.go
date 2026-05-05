package sftp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"knot/internal/protocol"
	"net"
	"strings"
	"sync"
)

// SFTPConn is a wrapper around net.Conn that implements io.ReadWriteCloser
// for the sftp.NewClientPipe. It handles Knot protocol messages.
type SFTPConn struct {
	Alias       string
	Conn        net.Conn
	DataCh      chan []byte
	ErrCh       chan error
	NotifyCh    chan error
	Ready       chan struct{} // Closed when "ok" is received
	Closed      chan struct{}
	StartOnce   sync.Once
	CloseOnce   sync.Once
	WriteMu     sync.Mutex
	Buf         []byte
	Interactive bool // If true, handles HostKeyConfirm interactively
	AuthHandler func(challenge protocol.AuthChallengePayload) (*protocol.AuthResponsePayload, error)
	FollowCh    chan protocol.SessionCWDNotify
}

func (s *SFTPConn) Start() {
	s.StartOnce.Do(func() {
		s.DataCh = make(chan []byte, 100)
		s.ErrCh = make(chan error, 1)
		s.NotifyCh = make(chan error, 1)
		s.Ready = make(chan struct{})
		s.Closed = make(chan struct{})
		if s.FollowCh == nil {
			s.FollowCh = make(chan protocol.SessionCWDNotify, 32)
		}
		go func() {
			defer close(s.FollowCh)
			handshakeDone := false
			for {
				msg, err := protocol.ReadMessage(s.Conn)
				if err != nil {
					if !handshakeDone {
						// Ensure Ready is not blocked if we fail during handshake
						close(s.Ready)
					}
					s.reportError(s.disconnectError(err))
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
							s.reportError(err)
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
						s.reportError(err)
						close(s.Ready)
					} else {
						s.reportError(err)
					}
					return
				case protocol.TypeSessionCWDNotify:
					var notify protocol.SessionCWDNotify
					if err := json.Unmarshal(msg.Payload, &notify); err != nil {
						continue
					}
					select {
					case s.FollowCh <- notify:
					case <-s.Closed:
						return
					default:
					}
				case protocol.TypeAuthChallenge:
					if s.AuthHandler != nil {
						var challenge protocol.AuthChallengePayload
						if err := json.Unmarshal(msg.Payload, &challenge); err != nil {
							_ = s.writeProtocolMessage(protocol.TypeAuthRetryAbort, 0, nil)
							continue
						}
						resp, err := s.AuthHandler(challenge)
						if err != nil {
							_ = s.writeProtocolMessage(protocol.TypeAuthRetryAbort, 0, nil)
							continue
						}
						payload, err := json.Marshal(resp)
						if err != nil {
							_ = s.writeProtocolMessage(protocol.TypeAuthRetryAbort, 0, nil)
							continue
						}
						_ = s.writeProtocolMessage(protocol.TypeAuthResponse, 0, payload)
					} else {
						_ = s.writeProtocolMessage(protocol.TypeAuthRetryAbort, 0, nil)
					}
				case protocol.TypeHostKeyConfirm:
					if s.Interactive {
						fmt.Printf("\n%s ", string(msg.Payload))
						var response string
						if _, err := fmt.Scanln(&response); err != nil {
							response = "no"
						}
						_ = s.writeProtocolMessage(protocol.TypeHostKeyConfirm, 0, []byte(response))
					} else {
						err := fmt.Errorf("host key verification failed. Run 'knot ssh' first to accept the key")
						if !handshakeDone {
							s.reportError(err)
							close(s.Ready)
						} else {
							s.reportError(err)
						}
						return
					}
				}
			}
		}()
	})
}

func (s *SFTPConn) disconnectError(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		if s.Alias != "" {
			return fmt.Errorf("disconnected: SSH connection lost: %s", s.Alias)
		}
		return fmt.Errorf("disconnected: SSH connection lost")
	}
	return err
}

func (s *SFTPConn) reportError(err error) {
	select {
	case s.ErrCh <- err:
	case <-s.Closed:
		return
	}
	select {
	case s.NotifyCh <- err:
	default:
	}
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
	if err := s.writeProtocolMessage(protocol.TypeData, 0, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *SFTPConn) writeProtocolMessage(msgType uint8, reserved uint8, payload []byte) error {
	s.WriteMu.Lock()
	defer s.WriteMu.Unlock()
	return protocol.WriteMessage(s.Conn, msgType, reserved, payload)
}

func (s *SFTPConn) Close() error {
	var err error
	s.CloseOnce.Do(func() {
		close(s.Closed)
		if s.Conn != nil {
			err = s.Conn.Close()
		}
	})
	return err
}
