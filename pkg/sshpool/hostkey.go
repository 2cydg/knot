package sshpool

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"knot/internal/fileutil"
	"knot/internal/paths"
	"knot/pkg/config"
	"net"
	"os"
	"path/filepath"
	"strings"

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
				if err := ensureKnownHostsFile(khPath); err != nil {
					return err
				}
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
					return changedHostKeyError(khPath, policy)
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
				return changedHostKeyError(khPath, policy)
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

		return fmt.Errorf("host key verification failed (known_hosts: %s, policy: %s): %w", khPath, displayHostKeyPolicy(policy), ErrHostKeyReject)
	}, nil
}

func changedHostKeyError(khPath string, policy string) error {
	return fmt.Errorf("@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
		"@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\n"+
		"@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
		"IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!\n"+
		"Known hosts file: %s\n"+
		"Host key policy: %s: %w", khPath, displayHostKeyPolicy(policy), ErrHostKeyReject)
}

func displayHostKeyPolicy(policy string) string {
	if policy == "" {
		return "ask"
	}
	return policy
}

func appendKnownHost(khPath string, hostname string, key ssh.PublicKey) error {
	return withKnownHostsLock(khPath, func() error {
		if err := os.MkdirAll(filepath.Dir(khPath), 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(khPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer f.Close()

		line := knownhosts.Line([]string{hostname}, key)
		if _, err := f.WriteString(line + "\n"); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}
		return nil
	})
}

func replaceKnownHost(khPath string, hostname string, key ssh.PublicKey, want []knownhosts.KnownKey) error {
	return withKnownHostsLock(khPath, func() error {
		lineSet := make(map[int]struct{})
		for _, known := range want {
			if known.Filename == khPath && known.Line > 0 {
				lineSet[known.Line] = struct{}{}
			}
		}

		data, err := os.ReadFile(khPath)
		if err != nil {
			return err
		}

		var lines []string
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for lineNo := 1; scanner.Scan(); lineNo++ {
			if shouldRemoveKnownHostLine(scanner.Text(), hostname, lineNo, lineSet) {
				continue
			}
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		lines = append(lines, knownhosts.Line([]string{hostname}, key))

		var buf bytes.Buffer
		for _, line := range lines {
			if _, err := buf.WriteString(line + "\n"); err != nil {
				return err
			}
		}
		return fileutil.AtomicWriteFile(khPath, buf.Bytes(), 0600)
	})
}

func shouldRemoveKnownHostLine(line string, hostname string, lineNo int, lineSet map[int]struct{}) bool {
	if _, remove := lineSet[lineNo]; remove {
		return true
	}
	fields := strings.Fields(line)
	if len(fields) < 3 || strings.HasPrefix(fields[0], "#") || strings.HasPrefix(fields[0], "@") || strings.HasPrefix(fields[0], "|") {
		return false
	}
	target := knownhosts.Normalize(hostname)
	for _, host := range strings.Split(fields[0], ",") {
		if strings.HasPrefix(host, "!") {
			host = strings.TrimPrefix(host, "!")
		}
		if knownhosts.Normalize(host) == target {
			return true
		}
	}
	return false
}

func ensureKnownHostsFile(khPath string) error {
	return withKnownHostsLock(khPath, func() error {
		if err := os.MkdirAll(filepath.Dir(khPath), 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(khPath, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		return f.Close()
	})
}

func appendKnownHostLocked(khPath string, hostname string, key ssh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(khPath), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(khPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	line := knownhosts.Line([]string{hostname}, key)
	if _, err := f.WriteString(line + "\n"); err != nil {
		return err
	}
	return f.Sync()
}

func withKnownHostsLock(khPath string, fn func() error) error {
	return fileutil.WithLock(khPath+".lock", fn)
}

func resolveKnownHostsPath(srv config.ServerConfig) (string, error) {
	if srv.KnownHostsPath != "" {
		return srv.KnownHostsPath, nil
	}
	return paths.GetKnownHostsPath()
}
