package cmd

import (
	"fmt"
	"strings"

	"github.com/gaku/gkb/internal/kb"
	"github.com/spf13/cobra"
)

var searchTag string

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search entries by text or tag",
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		entries, err := kb.Search(requireKbDir(), query, searchTag)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("no results")
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
	searchCmd.Flags().StringVarP(&searchTag, "tag", "t", "", "filter by tag")
	rootCmd.AddCommand(searchCmd)
}
