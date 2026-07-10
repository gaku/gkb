package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gaku/gkb/internal/kb"
	"github.com/spf13/cobra"
)

var addTags []string
var addSlug string

var addCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Create a new entry",
	Long: "Create a new entry. If stdin is redirected (piped or from a file), " +
		"its contents become the entry's body, e.g. `gkb add \"Title\" < notes.md`.",
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbDir := requireKbDir()
		title := strings.Join(args, " ")

		var body string
		if !terminalStdin() {
			data, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return err
			}
			body = string(data)
		}

		e, err := kb.Create(kbDir, title, addSlug, addTags, body)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", e.Slug)
		return nil
	},
}

// terminalStdin is a var, not a direct isTerminal(os.Stdin) call, so tests
// can fake a non-terminal stdin — cmd.SetIn doesn't change what the real
// os.Stdin file descriptor looks like to Stat.
var terminalStdin = func() bool { return isTerminal(os.Stdin) }

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func init() {
	addCmd.Flags().StringSliceVarP(&addTags, "tag", "t", nil, "tags (comma-separated)")
	addCmd.Flags().StringVarP(&addSlug, "slug", "s", "", "slug (ASCII only; required for non-ASCII titles)")
	rootCmd.AddCommand(addCmd)
}
