package cmd

import (
	"fmt"
	"strings"

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
		fmt.Printf("entries: %d\n", len(entries))
		warnDuplicateTitles(entries)
		return nil
	},
}

// warnDuplicateTitles prints a warning for any title shared by more than one
// entry, since [[title]] wikilinks silently resolve to just one of them.
func warnDuplicateTitles(entries []*kb.Entry) {
	for title, slugs := range kb.DuplicateTitles(entries) {
		fmt.Printf("warning: title %q is shared by %s — [[%s]] wikilinks resolve to the most recently modified\n",
			title, strings.Join(slugs, ", "), title)
	}
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
