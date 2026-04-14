package commands

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/peterh/liner"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Manage SSH keys",
}

var keyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all keys",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		if len(cfg.Keys) == 0 {
			fmt.Println("No keys configured.")
			return nil
		}

		fmt.Printf("%-20s %-15s %-10s\n", "ALIAS", "TYPE", "BITS")
		fmt.Println(strings.Repeat("-", 50))

		var aliases []string
		for alias := range cfg.Keys {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)

		for _, alias := range aliases {
			k := cfg.Keys[alias]
			fmt.Printf("%-20s %-15s %-10d\n", k.Alias, k.Type, k.Length)
		}
		return nil
	},
}

func PromptForKey(line *liner.State) ([]byte, string, error) {
	fmt.Println("Enter private key file path or paste content (Ctrl+D to finish pasting):")
	var keyBytes []byte
	var input string
	var passphrase string

	for {
		lineStr, err := line.Prompt("> ")
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, "", err
		}
		trimmed := strings.TrimSpace(lineStr)
		if input == "" && (strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "~") || strings.Contains(trimmed, "\\")) {
			path := trimmed
			if strings.HasPrefix(path, "~") {
				home, _ := os.UserHomeDir()
				path = filepath.Join(home, path[1:])
			}
			data, err := os.ReadFile(path)
			if err == nil {
				keyBytes = data
				break
			}
		}
		input += lineStr + "\n"
		if strings.Contains(input, "PRIVATE KEY") && strings.Contains(input, "END") {
			keyBytes = []byte(input)
			break
		}
	}

	if keyBytes == nil && input != "" {
		keyBytes = []byte(input)
	}

	if keyBytes == nil {
		return nil, "", fmt.Errorf("no valid key input provided")
	}

	// Try to parse to see if it needs a passphrase
	_, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil && strings.Contains(err.Error(), "passphrase") {
		pass, err := line.PasswordPrompt("Enter passphrase for private key: ")
		if err != nil {
			return nil, "", err
		}
		passphrase = pass
		fmt.Println("Note: The original passphrase will be removed, and the key will be re-encrypted using Knot's secure storage.")
	}

	return keyBytes, passphrase, nil
}

var keyAddCmd = &cobra.Command{
	Use:   "add [alias]",
	Short: "Add a new SSH key",
	Long: `Add a new SSH key to managed keys.
Note: If using --passphrase, it may be visible in process lists. Use interactive mode for better security.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var alias string
		if len(args) > 0 {
			alias = args[0]
		}

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		fileFlag, _ := cmd.Flags().GetString("file")
		contentFlag, _ := cmd.Flags().GetString("content")
		passphraseFlag, _ := cmd.Flags().GetString("passphrase")

		var keyBytes []byte
		if fileFlag != "" {
			var err error
			keyBytes, err = os.ReadFile(fileFlag)
			if err != nil {
				return fmt.Errorf("failed to read key file: %w", err)
			}
		} else if contentFlag != "" {
			keyBytes = []byte(contentFlag)
		}

		if keyBytes != nil {
			if alias == "" {
				return fmt.Errorf("alias is required in non-interactive mode")
			}
			kConfig, err := ValidateAndPrepareKey(alias, keyBytes, passphraseFlag)
			if err != nil {
				return err
			}
			cfg.Keys[alias] = *kConfig
			if err := cfg.Save(provider); err != nil {
				return err
			}
			fmt.Printf("Key '%s' added successfully.\n", alias)
			return nil
		}

		// Interactive mode
		line := liner.NewLiner()
		defer line.Close()
		line.SetCtrlCAborts(true)

		if alias == "" {
			for {
				aliasStr, err := line.Prompt("Key Alias: ")
				if err != nil {
					return err
				}
				aliasStr = strings.TrimSpace(aliasStr)
				if aliasStr != "" {
					alias = aliasStr
					break
				}
			}
		}

		if _, exists := cfg.Keys[alias]; exists {
			fmt.Printf("Key alias '%s' already exists. Overwrite? (y/N): ", alias)
			resp, _ := line.Prompt("")
			if strings.ToLower(resp) != "y" {
				return nil
			}
		}

		kb, pass, err := PromptForKey(line)
		if err != nil {
			return err
		}

		kConfig, err := ValidateAndPrepareKey(alias, kb, pass)
		if err != nil {
			return err
		}

		cfg.Keys[alias] = *kConfig
		if err := cfg.Save(provider); err != nil {
			return err
		}
		fmt.Printf("Key '%s' added successfully.\n", alias)
		return nil
	},
}

var keyRemoveCmd = &cobra.Command{
	Use:   "remove [alias]",
	Short: "Remove a key",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("alias is required")
		}
		alias := args[0]

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		if _, exists := cfg.Keys[alias]; !exists {
			return fmt.Errorf("key '%s' not found", alias)
		}

		var usedBy []string
		for sAlias, srv := range cfg.Servers {
			if srv.KeyAlias == alias {
				usedBy = append(usedBy, sAlias)
			}
		}

		if len(usedBy) > 0 {
			fmt.Printf("Warning: Key '%s' is used by the following servers:\n", alias)
			for _, s := range usedBy {
				fmt.Printf("- %s\n", s)
			}
			fmt.Print("If you delete it, these servers' key settings will be cleared. Continue? (y/N): ")
			line := liner.NewLiner()
			defer line.Close()
			resp, _ := line.Prompt("")
			if strings.ToLower(resp) != "y" {
				return nil
			}

			for _, s := range usedBy {
				srv := cfg.Servers[s]
				srv.KeyAlias = ""
				cfg.Servers[s] = srv
			}
		}

		delete(cfg.Keys, alias)
		if err := cfg.Save(provider); err != nil {
			return err
		}
		fmt.Printf("Key '%s' removed successfully.\n", alias)
		return nil
	},
}

var keyEditCmd = &cobra.Command{
	Use:   "edit [alias]",
	Short: "Edit a key",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("alias is required")
		}
		alias := args[0]

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		if _, exists := cfg.Keys[alias]; !exists {
			return fmt.Errorf("key '%s' not found", alias)
		}

		fmt.Printf("Editing key '%s'.\n", alias)
		line := liner.NewLiner()
		defer line.Close()
		line.SetCtrlCAborts(true)

		kb, pass, err := PromptForKey(line)
		if err != nil {
			return err
		}

		kConfig, err := ValidateAndPrepareKey(alias, kb, pass)
		if err != nil {
			return err
		}

		cfg.Keys[alias] = *kConfig
		if err := cfg.Save(provider); err != nil {
			return err
		}
		fmt.Printf("Key '%s' updated successfully.\n", alias)
		return nil
	},
}

func ValidateAndPrepareKey(alias string, keyBytes []byte, passphrase string) (*config.KeyConfig, error) {
	var rawKey interface{}
	var err error

	if passphrase != "" {
		rawKey, err = ssh.ParseRawPrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
	} else {
		rawKey, err = ssh.ParseRawPrivateKey(keyBytes)
	}

	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	// Get type and bits
	var keyType string
	var bits int
	switch k := rawKey.(type) {
	case *rsa.PrivateKey:
		keyType = "RSA"
		bits = k.N.BitLen()
	case *ecdsa.PrivateKey:
		keyType = "ECDSA"
		bits = k.Curve.Params().BitSize
	case ed25519.PrivateKey:
		keyType = "ED25519"
		bits = 256
	default:
		// Try via ssh.PublicKey if possible
		signer, err := ssh.NewSignerFromKey(rawKey)
		if err == nil {
			keyType = signer.PublicKey().Type()
		} else {
			keyType = "Unknown"
		}
	}

	// Convert back to PEM (decrypted)
	var pemBlock *pem.Block
	if rsaKey, ok := rawKey.(*rsa.PrivateKey); ok {
		pemBlock = &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
		}
	} else {
		// Generic PKCS8 for others
		b, err := x509.MarshalPKCS8PrivateKey(rawKey)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal private key: %w", err)
		}
		pemBlock = &pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: b,
		}
	}

	decryptedPEM := pem.EncodeToMemory(pemBlock)

	return &config.KeyConfig{
		Alias:      alias,
		Type:       keyType,
		Length:     bits,
		PrivateKey: string(decryptedPEM),
	}, nil
}

func init() {
	keyAddCmd.Flags().String("file", "", "Private key file path")
	keyAddCmd.Flags().String("content", "", "Private key content")
	keyAddCmd.Flags().String("passphrase", "", "Passphrase for the private key")

	keyCmd.AddCommand(keyListCmd)
	keyCmd.AddCommand(keyAddCmd)
	keyCmd.AddCommand(keyRemoveCmd)
	keyCmd.AddCommand(keyEditCmd)
	rootCmd.AddCommand(keyCmd)
}
