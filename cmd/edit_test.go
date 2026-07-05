package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gaku/gkb/internal/config"
	"github.com/gaku/gkb/internal/kb"
)

func TestEditCommandOverwritesFromStdinWhenRedirected(t *testing.T) {
	dir := t.TempDir()
	if err := kb.Save(dir, &kb.Entry{Slug: "my-page", Title: "My Page", Date: time.Now(), Body: "old body"}); err != nil {
		t.Fatal(err)
	}

	oldCfg := cfg
	cfg = &config.Config{KbDir: dir}
	t.Cleanup(func() { cfg = oldCfg })

	newRaw := "---\ntitle: My Page (renamed)\ndate: 2026-07-05\n---\n\nnew body\n"
	editCmd.SetIn(strings.NewReader(newRaw))
	t.Cleanup(func() { editCmd.SetIn(nil) })

	oldStdin := terminalStdin
	terminalStdin = func() bool { return false }
	t.Cleanup(func() { terminalStdin = oldStdin })

	if err := editCmd.RunE(editCmd, []string{"my-page"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "my-page.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != newRaw {
		t.Fatalf("got %q, want %q", got, newRaw)
	}
}

func TestEditCommandRejectsStdinForMissingEntry(t *testing.T) {
	dir := t.TempDir()
	oldCfg := cfg
	cfg = &config.Config{KbDir: dir}
	t.Cleanup(func() { cfg = oldCfg })

	editCmd.SetIn(strings.NewReader("whatever"))
	t.Cleanup(func() { editCmd.SetIn(nil) })

	oldStdin := terminalStdin
	terminalStdin = func() bool { return false }
	t.Cleanup(func() { terminalStdin = oldStdin })

	if err := editCmd.RunE(editCmd, []string{"nope"}); err == nil {
		t.Fatal("expected an error for a missing entry")
	}
}
