package cmd

import (
	"fmt"
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
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbDir := requireKbDir()
		title := strings.Join(args, " ")
		e, err := kb.Create(kbDir, title, addSlug, addTags)
		if err != nil {
			return err
		}
		fmt.Printf("created %s\n", e.Slug)

		if isInteractive() {
			return openEditor(filepath.Join(kbDir, e.Slug+".md"))
		}
		return nil
	},
}

func isInteractive() bool {
	// Only launch an editor when both input and output are attached to a
	// terminal. If either is redirected (piped, captured, etc.), an
	// interactive editor like vim cannot run and would fail.
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
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
	addCmd.Flags().StringVarP(&addSlug, "slug", "s", "", "slug (required for non-ASCII titles)")
	rootCmd.AddCommand(addCmd)
}
