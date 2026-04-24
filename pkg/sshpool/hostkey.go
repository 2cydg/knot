package sshpool

import (
	"errors"
	"fmt"
	"knot/internal/paths"
	"knot/pkg/config"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func buildHostKeyCallback(srv config.ServerConfig, confirmCallback func(string) bool) (ssh.HostKeyCallback, error) {
	khPath, err := resolveKnownHostsPath(srv)
	if err != nil {
		return nil, err
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		hkb, err := knownhosts.New(khPath)
		if err != nil {
			if os.IsNotExist(err) {
				f, err := os.OpenFile(khPath, os.O_CREATE|os.O_WRONLY, 0600)
				if err != nil {
					return err
				}
				f.Close()
				hkb, err = knownhosts.New(khPath)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}

		err = hkb(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			if len(keyErr.Want) > 0 {
				return fmt.Errorf("@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
					"@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\n"+
					"@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
					"IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!: %w", ErrHostKeyReject)
			}

			if confirmCallback != nil {
				prompt := fmt.Sprintf("The authenticity of host '%s' can't be established.\n"+
					"%s key fingerprint is %s.\n"+
					"Are you sure you want to continue connecting (yes/no)? ",
					hostname, key.Type(), ssh.FingerprintSHA256(key))

				if confirmCallback(prompt) {
					f, err := os.OpenFile(khPath, os.O_APPEND|os.O_WRONLY, 0600)
					if err != nil {
						return err
					}
					defer f.Close()

					line := knownhosts.Line([]string{hostname}, key)
					if _, err := f.WriteString(line + "\n"); err != nil {
						return err
					}
					return nil
				}
				return fmt.Errorf("host key verification failed (user rejected): %w", ErrHostKeyReject)
			}
		}

		return fmt.Errorf("host key verification failed: %w", ErrHostKeyReject)
	}, nil
}

func resolveKnownHostsPath(srv config.ServerConfig) (string, error) {
	if srv.KnownHostsPath != "" {
		return srv.KnownHostsPath, nil
	}
	return paths.GetKnownHostsPath()
}
