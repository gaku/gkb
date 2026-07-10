package cmd

import (
	"fmt"
	"os"

	"github.com/gaku/gkb/internal/config"
	"github.com/spf13/cobra"
)

var cfg *config.Config

var rootCmd = &cobra.Command{
	Use:   "gkb",
	Short: "gaku's knowledge base",
	Long: `gkb is a personal knowledge base: a directory of plain Markdown files, one
per entry, with frontmatter for metadata:

    ---
    title: Auth Strategy
    date: 2026-07-05
    tags: auth, infra
    ---

    the Markdown body goes here

The filename (the entry's slug, e.g. auth-strategy.md) must be ASCII even
when the title isn't. A title that's already ASCII derives one
automatically; for a non-ASCII title (e.g. Japanese), pass -s/--slug
explicitly (gkb add, or the slug field in the web editor).

AI agents and scripts: always go through this CLI rather than reading or
writing entry files directly on disk with a general-purpose file tool.
"gkb show" prints a file exactly as stored -- frontmatter included, no
rendering or reformatting. "gkb edit" is its write-side counterpart: it
reads stdin and overwrites the file verbatim. That symmetry is the pattern
to use to read and update an entry -- see the examples below. "gkb add"
supports the same trick for the initial body on create.`,
	Example: `  gkb show <slug> | some-transform | gkb edit <slug>
  gkb add "New Page" -t tag1,tag2 < notes.md`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(loadConfig)
}

func loadConfig() {
	var err error
	cfg, err = config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error loading config:", err)
		os.Exit(1)
	}
}

func requireKbDir() string {
	if cfg.KbDir == "" {
		fmt.Fprintln(os.Stderr, "kb_dir not set. Run: gkb init")
		os.Exit(1)
	}
	return cfg.KbDir
}
