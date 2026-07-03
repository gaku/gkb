package cmd

import (
	"fmt"

	"github.com/gaku/gkb/internal/kb"
	"github.com/spf13/cobra"
)

var renameCmd = &cobra.Command{
	Use:   "rename <old_slug> <new_slug>",
	Short: "Rename an entry",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := kb.Rename(requireKbDir(), args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("renamed %s -> %s\n", args[0], args[1])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(renameCmd)
}
