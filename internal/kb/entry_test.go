package kb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidSlug(t *testing.T) {
	valid := []string{"rome", "rome-2", "rome_2", "R2D2", "a"}
	for _, s := range valid {
		if !ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = false, want true", s)
		}
	}

	invalid := []string{"", "../../etc/passwd", "a/b", "a b", "ローマ", "a.md", "."}
	for _, s := range invalid {
		if ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = true, want false", s)
		}
	}
}

func TestRawReturnsFileByteForByte(t *testing.T) {
	dir := t.TempDir()
	raw := "---\ntitle: My Page\ndate: 2026-07-05\n---\n\nhello **world**\n"
	if err := os.WriteFile(filepath.Join(dir, "my-page.md"), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := Raw(dir, "my-page")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != raw {
		t.Fatalf("got %q, want %q", got, raw)
	}
}

func TestRawMissingEntry(t *testing.T) {
	if _, err := Raw(t.TempDir(), "nope"); err == nil {
		t.Fatal("expected an error for a missing entry")
	}
}

func TestWriteRawOverwritesExistingEntry(t *testing.T) {
	dir := t.TempDir()
	original := "---\ntitle: My Page\ndate: 2026-07-05\n---\n\noriginal\n"
	if err := os.WriteFile(filepath.Join(dir, "my-page.md"), []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	updated := "---\ntitle: My Page\ndate: 2026-07-05\n---\n\nupdated\n"
	if err := WriteRaw(dir, "my-page", []byte(updated)); err != nil {
		t.Fatal(err)
	}
	got, err := Raw(dir, "my-page")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != updated {
		t.Fatalf("got %q, want %q", got, updated)
	}
}

func TestWriteRawRejectsMissingEntry(t *testing.T) {
	if err := WriteRaw(t.TempDir(), "nope", []byte("x")); err == nil {
		t.Fatal("expected an error for a missing entry")
	}
}

func TestParseReadsAliases(t *testing.T) {
	dir := t.TempDir()
	raw := "---\ntitle: My Page\ndate: 2026-07-05\naliases: Old Name, 別名\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "my-page.md"), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	e, err := Load(dir, "my-page")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Old Name", "別名"}
	if len(e.Aliases) != len(want) || e.Aliases[0] != want[0] || e.Aliases[1] != want[1] {
		t.Fatalf("got Aliases %q, want %q", e.Aliases, want)
	}
}

func TestMarshalWritesAliases(t *testing.T) {
	e := &Entry{Slug: "my-page", Title: "My Page", Aliases: []string{"Old Name", "別名"}}
	got := e.Marshal()
	if !strings.Contains(got, "\naliases: Old Name, 別名\n") {
		t.Fatalf("Marshal output missing aliases line: %q", got)
	}
}
