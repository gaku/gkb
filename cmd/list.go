package cmd

import (
	"fmt"
	"strings"

	"github.com/gaku/gkb/internal/kb"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := kb.List(requireKbDir())
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("no entries")
			return nil
		}
		for _, e := range entries {
			tags := ""
			if len(e.Tags) > 0 {
				tags = "  [" + strings.Join(e.Tags, ", ") + "]"
			}
			fmt.Printf("%-30s %s%s\n", e.Slug, e.Date.Format("2006-01-02"), tags)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
