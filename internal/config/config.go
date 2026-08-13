package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	KbDir    string `toml:"kb_dir"`
	ServeURL string `toml:"serve_url"`
}

// ServeConfig holds the credentials for gkb serve. It is separate from Config
// because ~/.gkb is read by agents that need access to the knowledge base.
type ServeConfig struct {
	ServeUser string `toml:"serve_user"`
	ServePass string `toml:"serve_pass"`
}

func configPath() string {
	return filepath.Join(os.Getenv("HOME"), ".gkb")
}

func ServeConfigPath() string {
	return filepath.Join(os.Getenv("HOME"), ".gkb-serve")
}

func Load() (*Config, error) {
	return LoadFile(configPath())
}

func LoadFile(path string) (*Config, error) {
	cfg := &Config{}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
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

func LoadServe(path string) (*ServeConfig, error) {
	cfg := &ServeConfig{}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	return cfg, nil
}

func Save(cfg *Config) error {
	return SaveFile(configPath(), cfg)
}

func SaveFile(path string, cfg *Config) error {
	f, err := os.Create(path)
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
