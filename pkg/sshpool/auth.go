package sshpool

import (
	"fmt"
	"io"
	"knot/pkg/config"
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type DialOptions struct {
	AgentSocket   string
	HostKeyPolicy string
}

func (o DialOptions) agentSocketPath() string {
	if o.AgentSocket != "" {
		return o.AgentSocket
	}
	return GetAgentPath()
}

func buildAuthMethods(srv config.ServerConfig, cfg *config.Config, opts DialOptions) ([]ssh.AuthMethod, io.Closer, error) {
	authMethods := []ssh.AuthMethod{}
	var agentConn net.Conn

	switch srv.AuthMethod {
	case config.AuthMethodAgent:
		socket := opts.agentSocketPath()
		if socket == "" {
			return nil, nil, fmt.Errorf("SSH_AUTH_SOCK not set")
		}

		var err error
		agentConn, err = DialAgent(socket)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to connect to SSH agent: %w", err)
		}

		agentClient := agent.NewClient(agentConn)
		signers, err := agentClient.Signers()
		if err != nil {
			agentConn.Close()
			return nil, nil, fmt.Errorf("failed to get signers from SSH agent: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signers...))

	case config.AuthMethodKey:
		if srv.KeyID != "" && cfg != nil {
			keyCfg, ok := cfg.Keys[srv.KeyID]
			if !ok {
				return nil, nil, fmt.Errorf("key %s not found in config", srv.KeyID)
			}

			signer, err := ssh.ParsePrivateKey([]byte(keyCfg.PrivateKey))
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse private key %s: %w", keyCfg.Alias, err)
			}
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}

	case config.AuthMethodPassword:
		if srv.Password != "" {
			authMethods = append(authMethods, ssh.Password(srv.Password))
		}
	}

	if len(authMethods) == 0 {
		if agentConn != nil {
			agentConn.Close()
		}
		return nil, nil, fmt.Errorf("no authentication methods provided for %s: %w", srv.Alias, ErrAuthFailed)
	}

	return authMethods, agentConn, nil
}
