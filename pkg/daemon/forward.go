package daemon

import (
	"context"
	"fmt"
	"io"
	"knot/internal/logger"
	"knot/pkg/config"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ForwardRule represents an active or inactive forwarding rule.
type ForwardRule struct {
	mu        sync.RWMutex
	Config    config.ForwardConfig
	Alias     string
	IsTemp    bool
	Status    string // Active, Inactive, Error
	Error     string
	listener  net.Listener
	ctx       context.Context
	cancel    context.CancelFunc
}

// GetStatus returns a snapshot of the rule status.
func (r *ForwardRule) GetStatus() (string, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Status, r.Error, r.Config.Enabled
}

// ForwardManager manages all port forwarding rules.
type ForwardManager struct {
	mu     sync.RWMutex
	rules  map[string]*ForwardRule // key is "Alias:Type:LocalPort"
	crypto interface{}             // Placeholder for crypto provider if needed for saving
}

// NewForwardManager creates a new ForwardManager.
func NewForwardManager() *ForwardManager {
	return &ForwardManager{
		rules: make(map[string]*ForwardRule),
	}
}

func (fm *ForwardManager) GetRuleKey(alias string, fType string, port int) string {
	return fmt.Sprintf("%s:%s:%d", alias, fType, port)
}

// AddRule adds a new rule. If it's enabled and a connection exists, it starts it.
func (fm *ForwardManager) AddRule(alias string, cfg config.ForwardConfig, isTemp bool, sshClient *ssh.Client) error {
	key := fm.GetRuleKey(alias, cfg.Type, cfg.LocalPort)
	
	fm.mu.Lock()
	rule, exists := fm.rules[key]
	if !exists {
		rule = &ForwardRule{
			Config: cfg,
			Alias:  alias,
			IsTemp: isTemp,
			Status: "Inactive",
		}
		fm.rules[key] = rule
	}
	fm.mu.Unlock()

	if exists {
		rule.mu.Lock()
		if rule.Status == "Active" {
			rule.mu.Unlock()
			return fmt.Errorf("forwarding rule %s is already active", key)
		}
		rule.Config = cfg
		rule.mu.Unlock()
	}

	if cfg.Enabled && sshClient != nil {
		return fm.StartRule(rule, sshClient)
	}
	return nil
}

// GetRule returns a rule by key.
func (fm *ForwardManager) GetRule(alias string, fType string, port int) (*ForwardRule, bool) {
	key := fm.GetRuleKey(alias, fType, port)
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	rule, ok := fm.rules[key]
	return rule, ok
}

// RemoveRule stops and removes a rule.
func (fm *ForwardManager) RemoveRule(alias string, fType string, port int) {
	key := fm.GetRuleKey(alias, fType, port)
	fm.mu.Lock()
	rule, ok := fm.rules[key]
	if ok {
		delete(fm.rules, key)
	}
	fm.mu.Unlock()

	if ok {
		fm.StopRule(rule)
	}
}

// StartRule starts a forwarding rule on the given SSH client.
func (fm *ForwardManager) StartRule(rule *ForwardRule, sshClient *ssh.Client) error {
	rule.mu.Lock()
	defer rule.mu.Unlock()

	if rule.Status == "Active" {
		return nil
	}

	rule.ctx, rule.cancel = context.WithCancel(context.Background())
	rule.Config.Enabled = true // Mark as enabled when starting

	var err error
	switch rule.Config.Type {
	case "L":
		err = fm.startLocalForward(rule, sshClient)
	case "R":
		err = fm.startRemoteForward(rule, sshClient)
	case "D":
		err = fm.startDynamicForward(rule, sshClient)
	default:
		err = fmt.Errorf("unsupported forward type: %s", rule.Config.Type)
	}

	if err != nil {
		rule.Status = "Error"
		rule.Error = err.Error()
		if rule.cancel != nil {
			rule.cancel()
			rule.cancel = nil
		}
		return err
	}

	rule.Status = "Active"
	rule.Error = ""
	return nil
}

// StopRule stops an active forwarding rule.
func (fm *ForwardManager) StopRule(rule *ForwardRule) {
	rule.mu.Lock()
	defer rule.mu.Unlock()

	if rule.listener != nil {
		rule.listener.Close()
		rule.listener = nil
	}
	if rule.cancel != nil {
		rule.cancel()
		rule.cancel = nil
	}
	rule.Status = "Inactive"
	// Note: We don't set Config.Enabled = false here because StopRule 
	// might be called during disconnect, where we want to keep it enabled for reconnect.
	// Use SetEnabled(false) for manual disable.
}

// SetEnabled updates the enabled state of a rule.
func (fm *ForwardManager) SetEnabled(rule *ForwardRule, enabled bool, sshClient *ssh.Client) error {
	rule.mu.Lock()
	rule.Config.Enabled = enabled
	rule.mu.Unlock()

	if enabled {
		if sshClient != nil {
			return fm.StartRule(rule, sshClient)
		}
		return nil
	}
	fm.StopRule(rule)
	return nil
}

func (fm *ForwardManager) startLocalForward(rule *ForwardRule, sshClient *ssh.Client) error {
	addr := fmt.Sprintf("127.0.0.1:%d", rule.Config.LocalPort)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	rule.listener = l

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go fm.handleLocalConn(conn, sshClient, rule.Config.RemoteAddr, rule.ctx)
		}
	}()
	return nil
}

func (fm *ForwardManager) handleLocalConn(localConn net.Conn, sshClient *ssh.Client, remoteAddr string, ctx context.Context) {
	defer localConn.Close()
	
	// Create a dial context with timeout
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// ssh.Client doesn't have DialContext yet in older versions, but we can use a channel
	type dialRes struct {
		conn net.Conn
		err  error
	}
	resCh := make(chan dialRes, 1)
	go func() {
		conn, err := sshClient.Dial("tcp", remoteAddr)
		resCh <- dialRes{conn, err}
	}()

	var remoteConn net.Conn
	select {
	case res := <-resCh:
		if res.err != nil {
			logger.Error("Local forward: failed to dial remote", "remote", remoteAddr, "error", res.err)
			return
		}
		remoteConn = res.conn
	case <-dialCtx.Done():
		logger.Error("Local forward: dial remote timeout", "remote", remoteAddr)
		return
	}
	defer remoteConn.Close()

	fm.proxy(localConn, remoteConn, ctx)
}

func (fm *ForwardManager) startRemoteForward(rule *ForwardRule, sshClient *ssh.Client) error {
	l, err := sshClient.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", rule.Config.LocalPort))
	if err != nil {
		return err
	}
	rule.listener = l

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go fm.handleRemoteConn(conn, rule.Config.RemoteAddr, rule.ctx)
		}
	}()
	return nil
}

func (fm *ForwardManager) handleRemoteConn(remoteConn net.Conn, localAddr string, ctx context.Context) {
	defer remoteConn.Close()
	
	d := net.Dialer{Timeout: 15 * time.Second}
	localConn, err := d.DialContext(ctx, "tcp", localAddr)
	if err != nil {
		logger.Error("Remote forward: failed to dial local", "local", localAddr, "error", err)
		return
	}
	defer localConn.Close()

	fm.proxy(remoteConn, localConn, ctx)
}

func (fm *ForwardManager) startDynamicForward(rule *ForwardRule, sshClient *ssh.Client) error {
	addr := fmt.Sprintf("127.0.0.1:%d", rule.Config.LocalPort)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	rule.listener = l

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go fm.handleDynamicConn(conn, sshClient, rule.ctx)
		}
	}()
	return nil
}

func (fm *ForwardManager) handleDynamicConn(conn net.Conn, sshClient *ssh.Client, ctx context.Context) {
	defer conn.Close()
	
	// Set initial deadlines for handshake
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	// 1. Read greeting
	buf := make([]byte, 512)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	if buf[0] != 0x05 {
		return
	}
	nMethods := int(buf[1])
	if nMethods > 255 || nMethods <= 0 {
		return
	}
	if _, err := io.ReadFull(conn, buf[:nMethods]); err != nil {
		return
	}

	foundNoAuth := false
	for i := 0; i < nMethods; i++ {
		if buf[i] == 0x00 {
			foundNoAuth = true
			break
		}
	}

	if !foundNoAuth {
		conn.Write([]byte{0x05, 0xFF})
		return
	}

	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// 3. Read request
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return
	}
	if buf[0] != 0x05 || buf[1] != 0x01 {
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	var addr string
	switch buf[3] {
	case 0x01: // IPv4
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return
		}
		addr = net.IP(buf[:4]).String()
	case 0x03: // Domain name
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return
		}
		addrLen := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:addrLen]); err != nil {
			return
		}
		addr = string(buf[:addrLen])
	case 0x04: // IPv6
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return
		}
		addr = net.IP(buf[:16]).String()
	default:
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	port := int(buf[0])<<8 | int(buf[1])
	destAddr := net.JoinHostPort(addr, fmt.Sprintf("%d", port))

	// Reset deadline for dial
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	destConn, err := sshClient.Dial("tcp", destAddr)
	if err != nil {
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer destConn.Close()

	// 5. Send success response
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	// Remove deadlines for proxying
	conn.SetDeadline(time.Time{})
	fm.proxy(conn, destConn, ctx)
}

func (fm *ForwardManager) proxy(c1, c2 net.Conn, ctx context.Context) {
	// Better proxy with context awareness and dual-close
	done := make(chan struct{})
	
	go func() {
		select {
		case <-ctx.Done():
			c1.Close()
			c2.Close()
		case <-done:
		}
	}()

	cp := func(dst, src net.Conn) {
		io.Copy(dst, src)
		// Close the other side to break its io.Copy
		dst.Close()
		src.Close()
	}

	go cp(c1, c2)
	cp(c2, c1)
	close(done)
}

// StopAllForAlias stops all rules for a specific alias.
func (fm *ForwardManager) StopAllForAlias(alias string) {
	fm.mu.RLock()
	var rulesToStop []*ForwardRule
	for _, rule := range fm.rules {
		if rule.Alias == alias {
			rulesToStop = append(rulesToStop, rule)
		}
	}
	fm.mu.RUnlock()

	for _, rule := range rulesToStop {
		fm.StopRule(rule)
	}
}

// ListRules returns all rules.
func (fm *ForwardManager) ListRules() []*ForwardRule {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	res := make([]*ForwardRule, 0, len(fm.rules))
	for _, r := range fm.rules {
		res = append(res, r)
	}
	return res
}
