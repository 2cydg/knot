package sshpool

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"knot/pkg/config"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

func dialViaProxy(targetAddr, proxyAlias string, cfg *config.Config) (net.Conn, error) {
	proxyCfg, ok := cfg.Proxies[proxyAlias]
	if !ok {
		return nil, fmt.Errorf("proxy %s not found in config", proxyAlias)
	}

	proxyAddr := net.JoinHostPort(proxyCfg.Host, strconv.Itoa(proxyCfg.Port))
	dialer := &net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	switch proxyCfg.Type {
	case config.ProxyTypeSOCKS5:
		var auth *proxy.Auth
		if proxyCfg.Username != "" {
			auth = &proxy.Auth{
				User:     proxyCfg.Username,
				Password: proxyCfg.Password,
			}
		}

		socksDialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, dialer)
		if err != nil {
			return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
		}
		return socksDialer.Dial("tcp", targetAddr)

	case config.ProxyTypeHTTP:
		return dialHTTPProxy(proxyAddr, targetAddr, proxyCfg.Username, proxyCfg.Password, dialer)

	default:
		return nil, fmt.Errorf("unsupported proxy type: %s", proxyCfg.Type)
	}
}

func dialHTTPProxy(proxyAddr, targetAddr, user, pass string, dialer *net.Dialer) (net.Conn, error) {
	conn, err := dialer.Dial("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to HTTP proxy: %w", err)
	}

	authHeader := ""
	if user != "" {
		authHeader = "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)) + "\r\n"
	}

	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n%s\r\n", targetAddr, targetAddr, authHeader)
	if _, err := conn.Write([]byte(connectReq)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send CONNECT request to HTTP proxy: %w", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read response from HTTP proxy: %w", err)
	}

	statusLine = strings.TrimSpace(statusLine)
	parts := strings.SplitN(statusLine, " ", 3)
	if len(parts) < 2 || parts[1] != "200" || !strings.HasPrefix(parts[0], "HTTP/") {
		conn.Close()
		return nil, fmt.Errorf("HTTP proxy connection failed: %s", statusLine)
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to read headers from HTTP proxy: %w", err)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	if reader.Buffered() > 0 {
		return &bufferedConn{Conn: conn, reader: reader}, nil
	}
	return conn, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}
