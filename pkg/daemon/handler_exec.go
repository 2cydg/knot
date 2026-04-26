package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"knot/internal/logger"
	"knot/internal/protocol"
	"knot/pkg/sshpool"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type limitedWriter struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
	totalSize int
}

func (w *limitedWriter) Write(p []byte) (n int, err error) {
	w.totalSize += len(p)
	if w.buf.Len() < w.limit {
		remaining := w.limit - w.buf.Len()
		if len(p) <= remaining {
			n, err = w.buf.Write(p)
		} else {
			n, err = w.buf.Write(p[:remaining])
			w.truncated = true
		}
	} else {
		w.truncated = true
	}
	return len(p), nil
}

func (w *limitedWriter) String() string {
	return w.buf.String()
}

func (d *Daemon) handleExecRequest(conn net.Conn, payload []byte) {
	var req protocol.ExecRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		logger.Error("Failed to unmarshal exec request", "error", err)
		d.sendExecResponse(conn, -1, "", "", "failed to unmarshal request: "+err.Error(), false, 0)
		return
	}

	logger.Info("Executing command", "alias", req.Alias, "command", req.Command)

	// 1. Get SSH client
	client, poolKeys, err := d.getSSHClient(req.Alias, req.SSHAuthSock, req.HostKeyPolicy)
	if err != nil {
		d.sendExecResponse(conn, -1, "", "", err.Error(), false, 0)
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

	// 2. Create session
	session, err := client.NewSession()
	if err != nil {
		d.sendExecResponse(conn, -1, "", "", fmt.Sprintf("failed to create session: %v", err), false, 0)
		return
	}
	defer session.Close()

	// 3. Set up limited buffers
	const maxOutputPerStream = protocol.MaxPayloadSize / 4 // Total max output is protocol.MaxPayloadSize / 2
	stdout := &limitedWriter{limit: maxOutputPerStream}
	stderr := &limitedWriter{limit: maxOutputPerStream}
	session.Stdout = stdout
	session.Stderr = stderr

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

	// 5. Prepare response
	stdoutStr := stdout.String()
	stderrStr := stderr.String()
	truncated := stdout.truncated || stderr.truncated
	truncatedSize := 0
	if stdout.truncated {
		truncatedSize += stdout.totalSize - stdout.buf.Len()
		stdoutStr += "\n[stdout truncated]"
	}
	if stderr.truncated {
		truncatedSize += stderr.totalSize - stderr.buf.Len()
		stderrStr += "\n[stderr truncated]"
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

func (d *Daemon) getSSHClient(alias string, agentSocket string, hostKeyPolicy string) (*ssh.Client, []string, error) {
	// Load config to connect
	cfg, err := d.loadConfig()
	if err != nil {
		return nil, nil, err
	}

	srv, ok := cfg.Servers[alias]
	if !ok {
		return nil, nil, fmt.Errorf("server alias '%s' not found", alias)
	}

	// Connect using pool
	client, keys, _, err := d.pool.GetClient(srv, cfg, func(msg string) bool {
		// exec is non-interactive
		return false
	}, sshpool.DialOptions{AgentSocket: agentSocket, HostKeyPolicy: hostKeyPolicy})
	if err != nil {
		// If it's a host key verification failure, give a helpful message
		if strings.Contains(err.Error(), "invalid host key policy") {
			return nil, nil, err
		}
		if strings.Contains(err.Error(), "REMOTE HOST IDENTIFICATION HAS CHANGED") {
			return nil, nil, fmt.Errorf("host key changed for '%s': %w", alias, err)
		}
		if strings.Contains(err.Error(), "host key") {
			return nil, nil, fmt.Errorf("host key verification failed for '%s'. Run 'knot ssh %s' first to accept the key", alias, alias)
		}
		return nil, nil, err
	}
	return client, keys, nil
}
