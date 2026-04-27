package daemon

import (
	"context"
	"encoding/json"
	"knot/internal/logger"
	"knot/internal/protocol"
	"knot/pkg/config"
	"net"
	"sync"
)

func (d *Daemon) handleSFTPRequest(conn net.Conn, requestPayload []byte) {
	var sftpReq protocol.SFTPRequest
	if err := json.Unmarshal(requestPayload, &sftpReq); err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: invalid sftp request"))
		return
	}

	alias := sftpReq.Alias

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

	// 4. Start SFTP subsystem
	if err := session.RequestSubsystem("sftp"); err != nil {
		sendError("failed to request sftp subsystem: " + err.Error())
		return
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

	// Send success response
	if err := protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("ok")); err != nil {
		return
	}

	// 5. Proxy I/O
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
			protocol.WriteMessage(conn, protocol.TypeDisconnect, 0, []byte("SSH connection lost: "+alias))
		}
	}
}
