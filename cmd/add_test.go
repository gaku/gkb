package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gaku/gkb/internal/config"
	"github.com/gaku/gkb/internal/kb"
)

func TestAddCommandReadsBodyFromStdin(t *testing.T) {
	dir := t.TempDir()
	oldCfg := cfg
	cfg = &config.Config{KbDir: dir}
	t.Cleanup(func() { cfg = oldCfg })

	var stdout bytes.Buffer
	addCmd.SetOut(&stdout)
	addCmd.SetIn(strings.NewReader("hello from stdin\n"))
	t.Cleanup(func() { addCmd.SetOut(nil); addCmd.SetIn(nil) })

	oldStdin, oldStdout := terminalStdin, terminalStdout
	terminalStdin = func() bool { return false }
	terminalStdout = func() bool { return false }
	t.Cleanup(func() { terminalStdin, terminalStdout = oldStdin, oldStdout })

	if err := addCmd.RunE(addCmd, []string{"My Page"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "created my-page") {
		t.Fatalf("unexpected output %q", stdout.String())
	}
	e, err := kb.Load(dir, "my-page")
	if err != nil {
		t.Fatal(err)
	}
	if e.Body != "hello from stdin" {
		t.Fatalf("body = %q", e.Body)
	}
}

func TestAddCommandLeavesBodyEmptyWhenStdinIsATerminal(t *testing.T) {
	dir := t.TempDir()
	oldCfg := cfg
	cfg = &config.Config{KbDir: dir}
	t.Cleanup(func() { cfg = oldCfg })

	var stdout bytes.Buffer
	addCmd.SetOut(&stdout)
	addCmd.SetIn(strings.NewReader("should not be read"))
	t.Cleanup(func() { addCmd.SetOut(nil); addCmd.SetIn(nil) })

	// stdin is a terminal but stdout isn't, so isInteractive() stays false
	// (no real $EDITOR gets launched) while still exercising the branch
	// that skips reading stdin as body content.
	oldStdin, oldStdout := terminalStdin, terminalStdout
	terminalStdin = func() bool { return true }
	terminalStdout = func() bool { return false }
	t.Cleanup(func() { terminalStdin, terminalStdout = oldStdin, oldStdout })

	if err := addCmd.RunE(addCmd, []string{"My Page"}); err != nil {
		t.Fatal(err)
	}
	e, err := kb.Load(dir, "my-page")
	if err != nil {
		t.Fatal(err)
	}
	if e.Body != "" {
		t.Fatalf("body = %q, want empty", e.Body)
	}
}
