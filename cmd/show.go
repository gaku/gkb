package cmd

import (
	"fmt"
	"strings"

	"github.com/gaku/gkb/internal/kb"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show <slug>",
	Short: "Display an entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		e, err := kb.Load(requireKbDir(), args[0])
		if err != nil {
			return err
		}
		fmt.Printf("# %s\n", e.Title)
		fmt.Printf("date: %s\n", e.Date.Format("2006-01-02"))
		if len(e.Tags) > 0 {
			fmt.Printf("tags: %s\n", strings.Join(e.Tags, ", "))
		}
		fmt.Printf("\n%s\n", e.Body)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(showCmd)
}
