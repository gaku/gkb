package cmd

import (
	"fmt"
	"os"

	"github.com/gaku/gkb/internal/kb"
	"github.com/spf13/cobra"
)

var attachCmd = &cobra.Command{
	Use:   "attach <slug> <image-file|->",
	Short: "Attach an image to an entry",
	Long:  "Attach an image to an entry. Pass - as the image file to read image bytes from stdin instead of a file on disk.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		src := cmd.InOrStdin()
		if args[1] != "-" {
			file, err := os.Open(args[1])
			if err != nil {
				return err
			}
			defer file.Close()
			src = file
		}

		a, err := kb.StoreAttachment(requireKbDir(), args[0], args[1], src)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), a.Markup)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(attachCmd)
}
