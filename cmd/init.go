package cmd

import (
	"fmt"

	"github.com/gaku/gkb/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize gkb with a knowledge base directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg.KbDir = args[0]
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("kb_dir set to %s\n", cfg.KbDir)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
