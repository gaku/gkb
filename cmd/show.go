package cmd

import (
	"github.com/gaku/gkb/internal/kb"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show <slug>",
	Short: "Print an entry's raw Markdown file",
	Long: "Print an entry's raw Markdown file exactly as stored on disk (frontmatter " +
		"and body, byte for byte) -- no rendering. Pairs with \"gkb edit\", which " +
		"overwrites an entry from stdin: gkb show <slug> | some-transform | gkb edit <slug>.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := kb.Raw(requireKbDir(), args[0])
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(data)
		return err
	},
}

func init() {
	rootCmd.AddCommand(showCmd)
}
