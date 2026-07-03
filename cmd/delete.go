package cmd

import (
	"fmt"

	"github.com/gaku/gkb/internal/kb"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <slug>",
	Short: "Remove an entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := kb.Delete(requireKbDir(), args[0]); err != nil {
			return err
		}
		fmt.Printf("deleted %s\n", args[0])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
