package commands

import (
	"fmt"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all server configurations",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		if len(cfg.Servers) == 0 {
			fmt.Println("No servers configured.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ALIAS\tUSER\tHOST\tPORT")
		for _, s := range cfg.Servers {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", s.Alias, s.User, s.Host, s.Port)
		}
		w.Flush()

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
