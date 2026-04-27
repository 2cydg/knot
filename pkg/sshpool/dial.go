package sshpool

import (
	"fmt"
	"knot/pkg/config"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func dial(srv config.ServerConfig, cfg *config.Config, jumpClient *ssh.Client, confirmCallback func(string) bool, opts DialOptions) (*ssh.Client, error) {
	authMethods, authCloser, err := buildAuthMethods(srv, cfg, opts)
	if err != nil {
		return nil, err
	}
	if authCloser != nil {
		defer authCloser.Close()
	}

	hostKeyCallback, err := buildHostKeyCallback(srv, confirmCallback, opts.HostKeyPolicy)
	if err != nil {
		return nil, err
	}

	clientConfig := &ssh.ClientConfig{
		User:            srv.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(srv.Host, strconv.Itoa(srv.Port))
	conn, err := dialTransport(addr, srv, cfg, jumpClient)
	if err != nil {
		return nil, err
	}

	ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, clientConfig)
	if err != nil {
		conn.Close()
		if strings.Contains(err.Error(), "ssh: unable to authenticate") {
			return nil, fmt.Errorf("ssh: unable to authenticate: %w", ErrAuthFailed)
		}
		return nil, err
	}

	return ssh.NewClient(ncc, chans, reqs), nil
}

func dialTransport(addr string, srv config.ServerConfig, cfg *config.Config, jumpClient *ssh.Client) (net.Conn, error) {
	if jumpClient != nil {
		return jumpClient.Dial("tcp", addr)
	}
	if srv.ProxyID != "" && cfg != nil {
		return dialViaProxy(addr, srv.ProxyID, cfg)
	}

	dialer := &net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return dialer.Dial("tcp", addr)
}
