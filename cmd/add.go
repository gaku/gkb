package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

		if isInteractive() {
			return openEditor(filepath.Join(kbDir, e.Slug+".md"))
		}
		return nil
	},
}

// terminalStdin/terminalStdout are vars, not direct isTerminal(os.Stdin)
// calls, so tests can fake a non-terminal stdin/stdout — cmd.SetIn doesn't
// change what the real os.Stdin file descriptor looks like to Stat.
var (
	terminalStdin  = func() bool { return isTerminal(os.Stdin) }
	terminalStdout = func() bool { return isTerminal(os.Stdout) }
)

func isInteractive() bool {
	// Only launch an editor when both input and output are attached to a
	// terminal. If either is redirected (piped, captured, etc.), an
	// interactive editor like vim cannot run and would fail.
	return terminalStdin() && terminalStdout()
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func openEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	c := exec.Command(editor, path)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func init() {
	addCmd.Flags().StringSliceVarP(&addTags, "tag", "t", nil, "tags (comma-separated)")
	addCmd.Flags().StringVarP(&addSlug, "slug", "s", "", "slug (ASCII only; required for non-ASCII titles)")
	rootCmd.AddCommand(addCmd)
}
