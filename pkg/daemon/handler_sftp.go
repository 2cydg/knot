package daemon

import (
	"context"
	"encoding/json"
	"knot/internal/logger"
	"knot/internal/protocol"
	"knot/pkg/config"
	"net"
	"sync"
	"time"
)

func (d *Daemon) handleSFTPRequest(conn net.Conn, requestPayload []byte) {
	var sftpReq protocol.SFTPRequest
	if err := json.Unmarshal(requestPayload, &sftpReq); err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: invalid sftp request"))
		return
	}

	alias := sftpReq.Alias
	var followCh chan protocol.SessionCWDNotify
	var followSession *Session

	sendError := func(errMsg string) {
		logger.Error("SFTP Request Error", "alias", alias, "error", errMsg)
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: "+errMsg))
	}

	// 1. Load config
	cfg, err := config.LoadFromPath(d.configPath, d.crypto)
	if err != nil {
		sendError("failed to load config: " + err.Error())
		return
	}

	serverID, srv, ok := cfg.FindServerByAlias(alias)
	if !ok {
		sendError("server not found: " + alias)
		return
	}
	if sftpReq.FollowSessionID != "" {
		s, ok := d.sm.Get(sftpReq.FollowSessionID)
		if !ok || s.ServerID != serverID {
			sendError("follow session not found: " + sftpReq.FollowSessionID)
			return
		}
		followSession = s
	}

	// 2. Get client with interactive confirmation callback
	confirmCallback := func(prompt string) bool {
		// Send confirmation request to CLI
		if err := protocol.WriteMessage(conn, protocol.TypeHostKeyConfirm, 0, []byte(prompt)); err != nil {
			logger.Error("Failed to send confirmation request", "alias", alias, "error", err)
			return false
		}

		// Wait for response from CLI
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			logger.Error("Failed to read confirmation response", "alias", alias, "error", err)
			return false
		}

		return string(msg.Payload) == "yes" || string(msg.Payload) == "y"
	}

	client, poolKeys, _, err := d.dialWithRetry(conn, serverID, alias, srv, cfg, sftpReq.IsInteractive, sftpReq.SSHAuthSock, sftpReq.HostKeyPolicy, confirmCallback)
	if err != nil {
		sendError("failed to connect to server: " + err.Error())
		return
	}

	for _, k := range poolKeys {
		d.pool.IncRef(k)
	}
	defer func() {
		for _, k := range poolKeys {
			d.pool.DecRef(k)
		}
	}()

	// 3. Create session
	session, err := client.NewSession()
	if err != nil {
		sendError("failed to create session: " + err.Error())
		return
	}
	defer session.Close()

	// Pipes must be created before the subsystem starts.
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

	// 4. Start SFTP subsystem
	if err := session.RequestSubsystem("sftp"); err != nil {
		sendError("failed to request sftp subsystem: " + err.Error())
		return
	}

	// Send success response
	if err := protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("ok")); err != nil {
		return
	}

	var writeMu sync.Mutex
	writeMessage := func(msgType uint8, reserved uint8, payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return protocol.WriteMessage(conn, msgType, reserved, payload)
	}

	if followSession != nil {
		var info protocol.SessionInfo
		var ok bool
		followCh, info, ok = followSession.AddFollower()
		if !ok {
			sendError("follow session closed: " + sftpReq.FollowSessionID)
			return
		}
		defer followSession.RemoveFollower(followCh)
		if err := injectOSC7Hook(followSession); err != nil {
			logger.Warn("Failed to inject OSC 7 hook into followed session", "alias", alias, "session", followSession.ID, "error", err)
		}
		if info.CurrentDir != "" {
			payload, _ := json.Marshal(protocol.SessionCWDNotify{SessionID: info.ID, Path: info.CurrentDir})
			if err := writeMessage(protocol.TypeSessionCWDNotify, 0, payload); err != nil {
				return
			}
		}
	}

	// 5. Proxy I/O
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(3)
	if followCh != nil {
		wg.Add(1)
	}

	// stdout -> conn
	go func() {
		defer wg.Done()
		defer cancel()
		buf := make([]byte, 32*1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				if err := writeMessage(protocol.TypeData, protocol.DataStdout, buf[:n]); err != nil {
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
				if err := writeMessage(protocol.TypeData, protocol.DataStderr, buf[:n]); err != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// conn -> stdin
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
			if msg.Header.Type == protocol.TypeData {
				if _, err := stdin.Write(msg.Payload); err != nil {
					return
				}
			}
		}
	}()

	if followCh != nil {
		go func() {
			defer wg.Done()
			defer cancel()
			for {
				var notify protocol.SessionCWDNotify
				select {
				case <-ctx.Done():
					return
				case n, ok := <-followCh:
					if !ok {
						return
					}
					notify = n
				}
				payload, err := json.Marshal(notify)
				if err != nil {
					continue
				}
				if err := writeMessage(protocol.TypeSessionCWDNotify, 0, payload); err != nil || notify.Closed {
					return
				}
			}
		}()
	}

	// Wait for completion or context cancellation
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	var normalExit bool
	select {
	case <-done:
		normalExit = true
	case <-ctx.Done():
	case <-d.stopCh:
	}

	// Trigger cancellation and close session to break blocking reads
	cancel()
	session.Close()
	conn.SetReadDeadline(time.Now().Add(-1 * time.Second))
	wg.Wait()

	// Final check: if client is no longer in the pool, the connection was lost
	if !normalExit {
		isAlive := false
		for _, k := range poolKeys {
			if d.pool.IsAlive(k, client) {
				isAlive = true
				break
			}
		}
		if !isAlive {
			writeMessage(protocol.TypeDisconnect, 0, []byte("SSH connection lost: "+alias))
		}
	}
}
