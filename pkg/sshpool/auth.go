package sshpool

import (
	"fmt"
	"io"
	"knot/pkg/config"
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func buildAuthMethods(srv config.ServerConfig, cfg *config.Config) ([]ssh.AuthMethod, io.Closer, error) {
	authMethods := []ssh.AuthMethod{}
	var agentConn net.Conn

	switch srv.AuthMethod {
	case config.AuthMethodAgent:
		socket := GetAgentPath()
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
		if srv.KeyAlias != "" && cfg != nil {
			keyCfg, ok := cfg.Keys[srv.KeyAlias]
			if !ok {
				return nil, nil, fmt.Errorf("key %s not found in config", srv.KeyAlias)
			}

			signer, err := ssh.ParsePrivateKey([]byte(keyCfg.PrivateKey))
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse private key %s: %w", srv.KeyAlias, err)
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
