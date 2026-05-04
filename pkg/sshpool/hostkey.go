package sshpool

import (
	"bufio"
	"errors"
	"fmt"
	"knot/internal/paths"
	"knot/pkg/config"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	HostKeyPolicyAsk          = ""
	HostKeyPolicyFail         = "fail"
	HostKeyPolicyAcceptNew    = "accept-new"
	HostKeyPolicyStrict       = "strict"
	HostKeyPolicyInsecureSkip = "insecure-skip"
)

func normalizeHostKeyPolicy(policy string) (string, error) {
	switch policy {
	case HostKeyPolicyAsk, HostKeyPolicyFail, HostKeyPolicyAcceptNew, HostKeyPolicyStrict, HostKeyPolicyInsecureSkip:
		return policy, nil
	default:
		return "", fmt.Errorf("invalid host key policy %q (expected fail, accept-new, strict, or insecure-skip)", policy)
	}
}

func buildHostKeyCallback(srv config.ServerConfig, confirmCallback func(string) bool, policy string) (ssh.HostKeyCallback, error) {
	policy, err := normalizeHostKeyPolicy(policy)
	if err != nil {
		return nil, err
	}
	if policy == HostKeyPolicyInsecureSkip {
		return ssh.InsecureIgnoreHostKey(), nil
	}

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
				if policy == HostKeyPolicyFail || policy == HostKeyPolicyStrict || policy == HostKeyPolicyAcceptNew {
					return changedHostKeyError()
				}
				if confirmCallback != nil {
					prompt := fmt.Sprintf("@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
						"@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\n"+
						"@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
						"The host key for '%s' has changed.\n"+
						"New %s key fingerprint is %s.\n"+
						"Overwrite the saved host key and continue connecting (yes/no)? ",
						hostname, key.Type(), ssh.FingerprintSHA256(key))

					if confirmCallback(prompt) {
						if err := replaceKnownHost(khPath, hostname, key, keyErr.Want); err != nil {
							return err
						}
						return nil
					}
					return fmt.Errorf("host key verification failed (user rejected changed key): %w", ErrHostKeyReject)
				}
				return changedHostKeyError()
			}

			if policy == HostKeyPolicyAcceptNew {
				return appendKnownHost(khPath, hostname, key)
			}

			if policy == HostKeyPolicyFail || policy == HostKeyPolicyStrict {
				return fmt.Errorf("host key verification failed (unknown host): %w", ErrHostKeyReject)
			}

			if confirmCallback != nil {
				prompt := fmt.Sprintf("The authenticity of host '%s' can't be established.\n"+
					"%s key fingerprint is %s.\n"+
					"Are you sure you want to continue connecting (yes/no)? ",
					hostname, key.Type(), ssh.FingerprintSHA256(key))

				if confirmCallback(prompt) {
					return appendKnownHost(khPath, hostname, key)
				}
				return fmt.Errorf("host key verification failed (user rejected): %w", ErrHostKeyReject)
			}
		}

		return fmt.Errorf("host key verification failed: %w", ErrHostKeyReject)
	}, nil
}

func changedHostKeyError() error {
	return fmt.Errorf("@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
		"@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\n"+
		"@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
		"IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!: %w", ErrHostKeyReject)
}

func appendKnownHost(khPath string, hostname string, key ssh.PublicKey) error {
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

func replaceKnownHost(khPath string, hostname string, key ssh.PublicKey, want []knownhosts.KnownKey) error {
	lineSet := make(map[int]struct{})
	for _, known := range want {
		if known.Filename == khPath && known.Line > 0 {
			lineSet[known.Line] = struct{}{}
		}
	}
	if len(lineSet) == 0 {
		return appendKnownHost(khPath, hostname, key)
	}

	f, err := os.Open(khPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		if _, remove := lineSet[lineNo]; remove {
			continue
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	lines = append(lines, knownhosts.Line([]string{hostname}, key))

	out, err := os.OpenFile(khPath, os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(out)
	for _, line := range lines {
		if _, err := w.WriteString(line + "\n"); err != nil {
			out.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return nil
}

func resolveKnownHostsPath(srv config.ServerConfig) (string, error) {
	if srv.KnownHostsPath != "" {
		return srv.KnownHostsPath, nil
	}
	return paths.GetKnownHostsPath()
}
