package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gaku/gkb/internal/config"
)

func TestShowCommandPrintsRawFile(t *testing.T) {
	dir := t.TempDir()
	raw := "---\ntitle: My Page\ndate: 2026-07-05\ntags: a, b\n---\n\nhello **world**\n"
	if err := os.WriteFile(filepath.Join(dir, "my-page.md"), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	oldCfg := cfg
	cfg = &config.Config{KbDir: dir}
	t.Cleanup(func() { cfg = oldCfg })

	var stdout bytes.Buffer
	showCmd.SetOut(&stdout)
	t.Cleanup(func() { showCmd.SetOut(nil) })

	if err := showCmd.RunE(showCmd, []string{"my-page"}); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != raw {
		t.Fatalf("got %q, want %q", stdout.String(), raw)
	}
}
