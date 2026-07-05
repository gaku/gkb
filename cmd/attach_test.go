package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gaku/gkb/internal/config"
	"github.com/gaku/gkb/internal/kb"
)

func TestAttachCommandPrintsMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := kb.Save(dir, &kb.Entry{Slug: "my-page", Title: "My Page", Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(dir, "diagram.png")
	if err := os.WriteFile(imagePath, []byte("\x89PNG\r\n\x1a\nimage data"), 0644); err != nil {
		t.Fatal(err)
	}

	oldCfg := cfg
	cfg = &config.Config{KbDir: dir}
	t.Cleanup(func() { cfg = oldCfg })
	var stdout bytes.Buffer
	attachCmd.SetOut(&stdout)
	t.Cleanup(func() { attachCmd.SetOut(nil) })

	if err := attachCmd.RunE(attachCmd, []string{"my-page", imagePath}); err != nil {
		t.Fatal(err)
	}
	markup := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(markup, "![](attachments/my-page--") || !strings.HasSuffix(markup, "--diagram.png)") {
		t.Fatalf("unexpected output %q", markup)
	}
}

func TestAttachCommandReadsStdin(t *testing.T) {
	dir := t.TempDir()
	if err := kb.Save(dir, &kb.Entry{Slug: "my-page", Title: "My Page", Date: time.Now()}); err != nil {
		t.Fatal(err)
	}

	oldCfg := cfg
	cfg = &config.Config{KbDir: dir}
	t.Cleanup(func() { cfg = oldCfg })
	var stdout bytes.Buffer
	attachCmd.SetOut(&stdout)
	attachCmd.SetIn(bytes.NewReader([]byte("\x89PNG\r\n\x1a\nimage data")))
	t.Cleanup(func() { attachCmd.SetOut(nil); attachCmd.SetIn(nil) })

	if err := attachCmd.RunE(attachCmd, []string{"my-page", "-"}); err != nil {
		t.Fatal(err)
	}
	markup := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(markup, "![](attachments/my-page--") || !strings.HasSuffix(markup, "--image.png)") {
		t.Fatalf("unexpected output %q", markup)
	}
}
