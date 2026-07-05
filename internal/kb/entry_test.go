package kb

import "testing"

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
