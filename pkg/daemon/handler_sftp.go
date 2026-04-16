package daemon

import (
	"context"
	"fmt"
	"knot/internal/logger"
	"knot/internal/protocol"
	"knot/pkg/config"
	"net"
	"strconv"
	"strings"
	"sync"
)

func (d *Daemon) handleSFTPRequest(conn net.Conn, payload string) {
	// Parse alias and optional sessionID (format: "alias[:sessionID]")
	parts := strings.SplitN(payload, ":", 2)
	alias := parts[0]
	var followSessionID string
	if len(parts) > 1 {
		followSessionID = parts[1]
		if followSessionID != "" {
			if _, err := strconv.Atoi(followSessionID); err != nil {
				protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: invalid session ID format"))
				return
			}
		}
	}

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

	srv, ok := cfg.Servers[alias]
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

	client, poolKey, _, err := d.pool.GetClient(srv, cfg, confirmCallback)
	if err != nil {
		sendError("failed to connect to server: " + err.Error())
		return
	}

	d.pool.IncRef(poolKey)
	defer d.pool.DecRef(poolKey)

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

	// Register as follower if requested
	if followSessionID != "" {
		// Use encapsulated Get method
		s, ok := d.sm.Get(followSessionID)

		if !ok {
			sendError("session not found: " + followSessionID)
			return
		}
		if s.Alias != alias {
			sendError(fmt.Sprintf("session %s does not belong to alias %s", followSessionID, alias))
			return
		}

		d.sm.AddFollower(followSessionID, conn)
		defer d.sm.RemoveFollower(followSessionID, conn)
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
			d.pool.Touch(poolKey)
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

	select {
	case <-done:
	case <-ctx.Done():
	case <-d.stopCh:
	}

	// Trigger cancellation and close session to break blocking reads
	cancel()
	session.Close()
	wg.Wait()

	// Final check: if client is no longer in the pool, the connection was lost
	if !d.pool.IsAlive(poolKey, client) {
		protocol.WriteMessage(conn, protocol.TypeDisconnect, 0, []byte("SSH connection lost: "+alias))
	}
}
