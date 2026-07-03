package cmd

import (
	"fmt"

	"github.com/gaku/gkb/internal/kb"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show knowledge base info",
	RunE: func(cmd *cobra.Command, args []string) error {
		kbDir := requireKbDir()
		entries, err := kb.List(kbDir)
		if err != nil {
			return err
		}
		fmt.Printf("kb_dir: %s\n", kbDir)
		fmt.Printf("entries: %d\n", len(entries))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
