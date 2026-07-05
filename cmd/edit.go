package cmd

import (
	"io"
	"path/filepath"

	"github.com/gaku/gkb/internal/kb"
	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit <slug>",
	Short: "Open an entry in $EDITOR, or overwrite it from stdin if redirected",
	Long: "Open an entry in $EDITOR. If stdin is redirected (piped or from a file), " +
		"its contents replace the entry's raw Markdown file verbatim instead, e.g. " +
		"`gkb show slug | some-transform | gkb edit slug`.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbDir := requireKbDir()
		slug := args[0]

		if !terminalStdin() {
			data, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return err
			}
			return kb.WriteRaw(kbDir, slug, data)
		}

		if _, err := kb.Load(kbDir, slug); err != nil {
			return err
		}
		return openEditor(filepath.Join(kbDir, slug+".md"))
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}
