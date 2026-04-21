package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"knot/internal/logger"
	"knot/internal/protocol"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func (d *Daemon) handleExecRequest(conn net.Conn, payload []byte) {
	var req protocol.ExecRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		logger.Error("Failed to unmarshal exec request", "error", err)
		d.sendExecResponse(conn, -1, "", "", "failed to unmarshal request: "+err.Error(), false, 0)
		return
	}

	logger.Info("Executing command", "alias", req.Alias, "command", req.Command)

	// 1. Get SSH client
	client, err := d.getSSHClient(req.Alias)
	if err != nil {
		d.sendExecResponse(conn, -1, "", "", err.Error(), false, 0)
		return
	}

	// 2. Create session
	session, err := client.NewSession()
	if err != nil {
		d.sendExecResponse(conn, -1, "", "", fmt.Sprintf("failed to create session: %v", err), false, 0)
		return
	}
	defer session.Close()

	// 3. Set up buffers
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// 4. Run command with timeout
	ctx := context.Background()
	var timeout time.Duration
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Second
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	done := make(chan error, 1)
	go func() {
		done <- session.Run(req.Command)
	}()

	var exitCode int
	var runErr error
	var truncated bool
	var truncatedSize int

	select {
	case runErr = <-done:
		if runErr != nil {
			if exitErr, ok := runErr.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			} else {
				exitCode = -1
			}
		}
	case <-ctx.Done():
		if req.Timeout > 0 {
			session.Signal(ssh.SIGKILL) // Try to kill the remote process
			exitCode = -1
			runErr = fmt.Errorf("command timed out after %v", timeout)
		}
	case <-d.stopCh:
		exitCode = -1
		runErr = fmt.Errorf("daemon stopping")
	}

	// 5. Handle truncation if needed
	stdoutStr := stdout.String()
	stderrStr := stderr.String()
	
	const maxOutput = protocol.MaxPayloadSize / 2 // Leave room for other fields
	if len(stdoutStr) > maxOutput {
		truncatedSize += len(stdoutStr)
		stdoutStr = stdoutStr[:maxOutput] + "\n[stdout truncated]"
		truncated = true
	}
	if len(stderrStr) > maxOutput {
		truncatedSize += len(stderrStr)
		stderrStr = stderrStr[:maxOutput] + "\n[stderr truncated]"
		truncated = true
	}

	errMsg := ""
	if runErr != nil {
		errMsg = runErr.Error()
	}

	d.sendExecResponse(conn, exitCode, stdoutStr, stderrStr, errMsg, truncated, truncatedSize)
}

func (d *Daemon) sendExecResponse(conn net.Conn, exitCode int, stdout, stderr, err string, truncated bool, truncatedSize int) {
	resp := protocol.ExecResponse{
		ExitCode:      exitCode,
		Stdout:        stdout,
		Stderr:        stderr,
		Error:         err,
		Truncated:     truncated,
		TruncatedSize: truncatedSize,
	}
	payload, _ := json.Marshal(resp)
	_ = protocol.WriteMessage(conn, protocol.TypeExecResp, 0, payload)
}

func (d *Daemon) getSSHClient(alias string) (*ssh.Client, error) {
	// Try pool first
	client, ok := d.pool.GetClientForAlias(alias)
	if ok {
		return client, nil
	}

	// Load config to connect
	cfg, err := d.loadConfig()
	if err != nil {
		return nil, err
	}

	srv, ok := cfg.Servers[alias]
	if !ok {
		return nil, fmt.Errorf("server alias '%s' not found", alias)
	}

	// Connect using pool
	client, _, _, err = d.pool.GetClient(srv, cfg, func(msg string) bool {
		// exec is non-interactive
		return false
	})
	if err != nil {
		// If it's a host key verification failure, give a helpful message
		if strings.Contains(err.Error(), "host key") {
			return nil, fmt.Errorf("host key verification failed for '%s'. Run 'knot ssh %s' first to accept the key", alias, alias)
		}
		return nil, err
	}
	return client, nil
}
