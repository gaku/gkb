package kb

import (
	"os"
	"path/filepath"
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
