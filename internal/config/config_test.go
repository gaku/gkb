package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServeSeparatesCredentialsFromMainConfig(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "gkb")
	servePath := filepath.Join(dir, "gkb-serve")
	if err := os.WriteFile(mainPath, []byte("kb_dir = \"~/kb\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(servePath, []byte("serve_user = \"user\"\nserve_pass = \"password\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	main, err := LoadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	serve, err := LoadServe(servePath)
	if err != nil {
		t.Fatal(err)
	}
	if main.KbDir != filepath.Join(os.Getenv("HOME"), "kb") {
		t.Fatalf("KbDir = %q", main.KbDir)
	}
	if serve.ServeUser != "user" || serve.ServePass != "password" {
		t.Fatalf("serve config = %#v", serve)
	}
}

func TestLoadServeMissingFileReturnsEmptyConfig(t *testing.T) {
	cfg, err := LoadServe(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServeUser != "" || cfg.ServePass != "" {
		t.Fatalf("serve config = %#v", cfg)
	}
}
