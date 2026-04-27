package commands

import (
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/sshpool"
	"sort"
	"strconv"
	"strings"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
)

// PromptAuthUpdate handles interactive authentication configuration.
// challenge: if not nil, contains info about the authentication failure (retry mode).
func PromptAuthUpdate(rl *readline.Instance, srv *config.ServerConfig, cfg *config.Config, provider crypto.Provider, challenge *protocol.AuthChallengePayload) error {
	if challenge != nil {
		color.Red("Authentication failed for [%s]! (Attempt %d/%d)", srv.Alias, challenge.Attempt, challenge.MaxAttempts)
		fmt.Printf("Error: %s\n", challenge.Error)
		fmt.Printf("Current auth method: %s\n", srv.AuthMethod)
	}

	fmt.Println("Choose authentication method:")
	fmt.Println("1) Password")
	fmt.Println("2) Private Key (managed)")
	fmt.Println("3) SSH Agent")

	choice, err := readLineWithPrompt(rl, "Choice (1-3, default 1): ")
	if err != nil {
		return err
	}
	if choice == "" {
		choice = "1"
	}

	switch choice {
	case "1":
		srv.AuthMethod = config.AuthMethodPassword
		prompt := "Password: "
		if challenge != nil {
			prompt = "New Password: "
		}
		pass, err := rl.ReadPassword(prompt)
		if err != nil {
			return err
		}
		srv.Password = string(pass)
		srv.KeyID = ""
	case "2":
		fmt.Println()
		srv.AuthMethod = config.AuthMethodKey
		srv.Password = ""
		if len(cfg.Keys) == 0 {
			resp, _ := readLineWithPrompt(rl, "No keys configured. Add one now? (Y/n): ")
			if resp != "" && strings.ToLower(resp) != "y" {
				return fmt.Errorf("no keys available")
			}

			// Add key on the fly
			kb, pass, err := PromptForKey(rl)
			if err != nil {
				return err
			}

			var kAlias string
			for {
				kAlias, err = readLineWithPrompt(rl, "New Key Alias: ")
				if err != nil {
					return err
				}
				kAlias = strings.TrimSpace(kAlias)
				if kAlias != "" {
					break
				}
			}

			keyID, err := cfg.NewKeyID()
			if err != nil {
				return err
			}
			kConfig, err := ValidateAndPrepareKey(keyID, kAlias, kb, pass)
			if err != nil {
				return err
			}
			cfg.Keys[keyID] = *kConfig
			if err := cfg.Save(provider); err != nil {
				return err
			}
			srv.KeyID = keyID
			fmt.Printf("Key '%s' added and selected.\n", kAlias)
		} else {
			fmt.Println("Available keys:")
			var keyIDs []string
			for id := range cfg.Keys {
				keyIDs = append(keyIDs, id)
			}
			sort.Slice(keyIDs, func(i, j int) bool {
				return cfg.Keys[keyIDs[i]].Alias < cfg.Keys[keyIDs[j]].Alias
			})
			for i, id := range keyIDs {
				fmt.Printf("%d) %s\n", i+1, cfg.Keys[id].Alias)
			}
			for {
				prompt := "Select key"
				if challenge != nil && srv.KeyID != "" {
					prompt = fmt.Sprintf("Select key (current: %s)", cfg.KeyAlias(srv.KeyID))
				}
				kChoice, _ := readLineWithPrompt(rl, fmt.Sprintf("%s (1-%d): ", prompt, len(keyIDs)))
				idx, err := strconv.Atoi(kChoice)
				if err == nil && idx > 0 && idx <= len(keyIDs) {
					srv.KeyID = keyIDs[idx-1]
					break
				}
				fmt.Println("Invalid selection.")
			}
		}
	case "3":
		if sshpool.GetAgentPath() == "" {
			fmt.Println("Warning: SSH Agent (SSH_AUTH_SOCK) not detected. Please ensure your agent is running.")
			resp, _ := readLineWithPrompt(rl, "Continue anyway? (y/N): ")
			if strings.ToLower(resp) != "y" {
				return fmt.Errorf("ssh agent not available")
			}
		}
		srv.AuthMethod = config.AuthMethodAgent
		srv.Password = ""
		srv.KeyID = ""
	default:
		return fmt.Errorf("invalid choice")
	}

	return nil
}
