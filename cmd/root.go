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
