package cmd

import (
	"path/filepath"

	"github.com/gaku/gkb/internal/kb"
	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit <slug>",
	Short: "Open an entry in $EDITOR",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbDir := requireKbDir()
		e, err := kb.Load(kbDir, args[0])
		if err != nil {
			return err
		}
		return openEditor(filepath.Join(kbDir, e.Slug+".md"))
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}
