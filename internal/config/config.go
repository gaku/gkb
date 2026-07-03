package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	KbDir string `toml:"kb_dir"`
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
