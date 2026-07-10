package cmd

import (
	"fmt"
	"io"

	"github.com/gaku/gkb/internal/kb"
	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit <slug>",
	Short: "Overwrite an entry's raw Markdown file from stdin",
	Long: "Overwrite an entry's raw Markdown file verbatim with stdin, e.g. " +
		"`gkb show slug | some-transform | gkb edit slug`.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbDir := requireKbDir()
		slug := args[0]

		if terminalStdin() {
			return fmt.Errorf("gkb edit requires input on stdin; e.g. `gkb show %s | ... | gkb edit %s`", slug, slug)
		}

		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return err
		}
		return kb.WriteRaw(kbDir, slug, data)
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}
