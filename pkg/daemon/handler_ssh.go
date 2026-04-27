package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"knot/internal/logger"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/sshpool"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func (d *Daemon) handleSSHRequest(conn net.Conn, req *protocol.SSHRequest) {
	sendError := func(errMsg string) {
		logger.Error("SSH Request Error", "alias", req.Alias, "error", errMsg)
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: "+errMsg))
	}

	// 1. Load config
	cfg, err := config.LoadFromPath(d.configPath, d.crypto)
	if err != nil {
		sendError("failed to load config: " + err.Error())
		return
	}

	srv, ok := cfg.Servers[req.Alias]
	if !ok {
		sendError("server not found: " + req.Alias)
		return
	}

	// 2. Get client with interactive confirmation callback
	confirmCallback := func(prompt string) bool {
		// Send confirmation request to CLI
		if err := protocol.WriteMessage(conn, protocol.TypeHostKeyConfirm, 0, []byte(prompt)); err != nil {
			logger.Error("Failed to send confirmation request", "alias", req.Alias, "error", err)
			return false
		}

		// Wait for response from CLI
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			logger.Error("Failed to read confirmation response", "alias", req.Alias, "error", err)
			return false
		}

		return string(msg.Payload) == "yes" || string(msg.Payload) == "y"
	}

	client, poolKeys, isNew, err := d.dialWithRetry(conn, req.Alias, srv, cfg, req.IsInteractive, req.SSHAuthSock, req.HostKeyPolicy, confirmCallback)
	if err != nil {
		sendError("failed to connect to server: " + err.Error())
		return
	}

	// 3. Create session
	if req.Rows <= 0 || req.Rows > 10000 || req.Cols <= 0 || req.Cols > 10000 {
		sendError("invalid terminal dimensions")
		return
	}
	if len(req.Term) > 64 || len(req.Term) == 0 {
		sendError("invalid terminal type")
		return
	}

	session, err := client.NewSession()
	if err != nil {
		sendError("failed to create session: " + err.Error())
		return
	}
	defer session.Close()

	// 4. Request PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty(req.Term, req.Rows, req.Cols, modes); err != nil {
		sendError("failed to request pty: " + err.Error())
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Agent Forwarding
	if req.ForwardAgent && req.SSHAuthSock != "" {
		if err := d.setupAgentForwarding(ctx, req, client, session); err != nil {
			logger.Warn("Failed to setup agent forwarding", "alias", req.Alias, "error", err)
		}
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		sendError("failed to get stdin pipe")
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		sendError("failed to get stdout pipe")
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		sendError("failed to get stderr pipe")
		return
	}

	if err := session.Shell(); err != nil {
		sendError("failed to start shell")
		return
	}

	// Register session in SessionManager
	s := d.sm.Add(req.Alias, conn, poolKeys)
	for _, k := range poolKeys {
		d.pool.IncRef(k)
	}
	defer func() {
		d.sm.Remove(s.ID)
		for _, k := range poolKeys {
			d.pool.DecRef(k)
		}
	}()

	// Send success response with session ID
	if err := protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("ok:"+s.ID)); err != nil {
		return
	}

	// 4.5 Send port forward notifications (only on new connection)
	if isNew {
		for _, rule := range d.fm.ListRules() {
			if rule.Alias == req.Alias && rule.Enabled {
				status, errStr, _ := rule.GetStatus()
				var msgStr string
				switch rule.Config.Type {
				case "L":
					msgStr = fmt.Sprintf("[Forward] L: localhost:%d -> %s", rule.Config.LocalPort, rule.Config.RemoteAddr)
				case "R":
					msgStr = fmt.Sprintf("[Forward] R: %s:%d -> %s", req.Alias, rule.Config.LocalPort, rule.Config.RemoteAddr)
				case "D":
					msgStr = fmt.Sprintf("[Forward] D: localhost:%d -> SOCKS5", rule.Config.LocalPort)
				}

				if status == "Active" {
					msgStr += " [Active]"
				} else if status == "Error" {
					msgStr += fmt.Sprintf(" [Error: %s]", errStr)
				} else {
					msgStr += " [Inactive]"
				}

				protocol.WriteMessage(conn, protocol.TypeForwardNotify, 0, []byte(msgStr))
			}
		}
	}

	// 5. Proxy I/O
	var wg sync.WaitGroup
	wg.Add(3)

	// stdout -> conn
	go func() {
		defer wg.Done()
		defer cancel()
		buf := make([]byte, 32*1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				if err := protocol.WriteMessage(conn, protocol.TypeData, protocol.DataStdout, buf[:n]); err != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// stderr -> conn
	go func() {
		defer wg.Done()
		defer cancel()
		buf := make([]byte, 32*1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				if err := protocol.WriteMessage(conn, protocol.TypeData, protocol.DataStderr, buf[:n]); err != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// conn -> stdin/resize
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			msg, err := protocol.ReadMessage(conn)
			if err != nil {
				return
			}
			for _, k := range poolKeys {
				d.pool.Touch(k)
			}
			switch msg.Header.Type {
			case protocol.TypeData:
				if msg.Header.Reserved == protocol.DataStdin {
					if _, err := stdin.Write(msg.Payload); err != nil {
						if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "closed pipe") {
							logger.Debug("Stdin pipe closed", "alias", req.Alias)
						} else {
							logger.Error("Failed to write to stdin", "alias", req.Alias, "error", err)
						}
						return
					}
				}
			case protocol.TypeSignal:
				if msg.Header.Reserved == protocol.SignalResize {
					var payload protocol.ResizePayload
					if err := json.Unmarshal(msg.Payload, &payload); err == nil {
						session.WindowChange(payload.Rows, payload.Cols)
					} else {
						logger.Error("Failed to unmarshal resize payload", "error", err)
					}
				}
			}
		}
	}()

	// 6. Wait for completion
	sessionExit := make(chan error, 1)
	go func() {
		sessionExit <- session.Wait()
	}()

	var normalExit bool
	select {
	case err := <-sessionExit:
		normalExit = true
		if err != nil {
			logger.Debug("SSH session finished with error", "alias", req.Alias, "error", err)
		} else {
			logger.Debug("SSH session finished normally", "alias", req.Alias)
		}
	case <-d.stopCh:
		logger.Info("Daemon stopping, closing SSH session", "alias", req.Alias)
	case <-ctx.Done():
		logger.Debug("SSH handler context cancelled", "alias", req.Alias)
	}

	// Trigger cancellation and close session to break blocking reads
	// Unblock conn -> stdin goroutine by setting a deadline
	conn.SetReadDeadline(time.Now().Add(-1 * time.Second))
	cancel()
	session.Close()
	wg.Wait()

	// Final check: only send "lost" message if the session didn't exit normally AND the connection is dead
	if !normalExit {
		isAlive := false
		for _, k := range poolKeys {
			if d.pool.IsAlive(k, client) {
				isAlive = true
				break
			}
		}
		if !isAlive {
			protocol.WriteMessage(conn, protocol.TypeDisconnect, 0, []byte("SSH connection lost: "+req.Alias))
		}
	}
}

func (d *Daemon) setupAgentForwarding(ctx context.Context, req *protocol.SSHRequest, client *ssh.Client, session *ssh.Session) error {
	logger.Debug("Initializing agent forwarding support", "alias", req.Alias, "local_socket", req.SSHAuthSock)

	// Request agent forwarding on the session first
	if err := agent.RequestAgentForwarding(session); err != nil {
		return fmt.Errorf("request agent forwarding on session: %w", err)
	}

	// Handle incoming agent forwarding channel requests
	channels := client.HandleChannelOpen("auth-agent@openssh.com")

	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.Debug("Agent forwarding context cancelled", "alias", req.Alias)
				return
			case newCh, ok := <-channels:
				if !ok {
					logger.Debug("Agent forwarding channel listener closed", "alias", req.Alias)
					return
				}

				go func(newCh ssh.NewChannel) {
					logger.Info("Received agent forwarding channel request from server", "alias", req.Alias)

					agentConn, err := sshpool.DialAgent(req.SSHAuthSock)
					if err != nil {
						logger.Error("Failed to connect to local agent", "error", err, "path", req.SSHAuthSock)
						newCh.Reject(ssh.ConnectionFailed, "could not open agent connection: "+err.Error())
						return
					}
					defer agentConn.Close()

					ch, reqs, err := newCh.Accept()
					if err != nil {
						logger.Error("Failed to accept agent channel", "error", err)
						return
					}
					go ssh.DiscardRequests(reqs)

					agentClient := agent.NewClient(agentConn)
					if err := agent.ServeAgent(agentClient, ch); err != nil && err != io.EOF {
						logger.Error("Agent server error during session", "error", err)
					}
					logger.Info("Agent forwarding channel closed", "alias", req.Alias)
				}(newCh)
			}
		}
	}()

	logger.Info("Agent forwarding initialized and listening for requests", "alias", req.Alias)
	return nil
}
