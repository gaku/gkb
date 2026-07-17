package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	KbDir string `toml:"kb_dir"`
	// ServeUser/ServePass gate `gkb serve` with HTTP Basic Auth. When either is
	// empty the server runs unauthenticated (with a warning). TLS is expected to
	// be terminated by a reverse proxy (Tailscale/Caddy), so these credentials
	// only travel over the encrypted browser->proxy hop and the loopback
	// proxy->gkb hop.
	ServeUser string `toml:"serve_user"`
	ServePass string `toml:"serve_pass"`
	// ServeURL is the externally reachable base URL for `gkb serve` (e.g. a
	// Tailscale or Caddy address), since gkb itself only knows its local bind
	// address. Purely informational -- printed by `gkb status`.
	ServeURL string `toml:"serve_url"`
}

func configPath() string {
	return filepath.Join(os.Getenv("HOME"), ".gkb")
}

func Load() (*Config, error) {
	cfg := &Config{}
	if _, err := toml.DecodeFile(configPath(), cfg); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if cfg.KbDir != "" {
		cfg.KbDir = expandHome(cfg.KbDir)
	}
	return cfg, nil
}

func Save(cfg *Config) error {
	f, err := os.Create(configPath())
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

func expandHome(path string) string {
	if len(path) > 1 && path[:2] == "~/" {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	return path
}
