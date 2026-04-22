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
	
	rl.SetPrompt("Choice (1-3, default 1): ")
	choice, err := rl.Readline()
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
		srv.KeyAlias = ""
	case "2":
		srv.AuthMethod = config.AuthMethodKey
		srv.Password = ""
		if len(cfg.Keys) == 0 {
			fmt.Print("No keys configured. Add one now? (Y/n): ")
			rl.SetPrompt("")
			resp, _ := rl.Readline()
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
				rl.SetPrompt("New Key Alias: ")
				kAlias, err = rl.Readline()
				if err != nil {
					return err
				}
				kAlias = strings.TrimSpace(kAlias)
				if kAlias != "" {
					break
				}
			}

			kConfig, err := ValidateAndPrepareKey(kAlias, kb, pass)
			if err != nil {
				return err
			}
			cfg.Keys[kAlias] = *kConfig
			if err := cfg.Save(provider); err != nil {
				return err
			}
			srv.KeyAlias = kAlias
			fmt.Printf("Key '%s' added and selected.\n", srv.KeyAlias)
		} else {
			fmt.Println("Available keys:")
			var keyAliases []string
			for k := range cfg.Keys {
				keyAliases = append(keyAliases, k)
			}
			sort.Strings(keyAliases)
			for i, k := range keyAliases {
				fmt.Printf("%d) %s\n", i+1, k)
			}
			for {
				prompt := "Select key"
				if challenge != nil && srv.KeyAlias != "" {
					prompt = fmt.Sprintf("Select key (current: %s)", srv.KeyAlias)
				}
				rl.SetPrompt(fmt.Sprintf("%s (1-%d): ", prompt, len(keyAliases)))
				kChoice, _ := rl.Readline()
				idx, err := strconv.Atoi(kChoice)
				if err == nil && idx > 0 && idx <= len(keyAliases) {
					srv.KeyAlias = keyAliases[idx-1]
					break
				}
				fmt.Println("Invalid selection.")
			}
		}
	case "3":
		if sshpool.GetAgentPath() == "" {
			fmt.Println("Warning: SSH Agent (SSH_AUTH_SOCK) not detected. Please ensure your agent is running.")
			fmt.Print("Continue anyway? (y/N): ")
			rl.SetPrompt("")
			resp, _ := rl.Readline()
			if strings.ToLower(resp) != "y" {
				return fmt.Errorf("ssh agent not available")
			}
		}
		srv.AuthMethod = config.AuthMethodAgent
		srv.Password = ""
		srv.KeyAlias = ""
	default:
		return fmt.Errorf("invalid choice")
	}

	return nil
}
