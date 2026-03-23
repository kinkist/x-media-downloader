// Package config loads program configuration from a YAML file.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for x-media-downloader.
// No username/password fields — authentication is handled via cookieswithchromedp.
type Config struct {
	Datadir        string `yaml:"datadir"`
	Debug          bool   `yaml:"debug"`

	Dbhost         string `yaml:"dbhost"`
	Dbuser         string `yaml:"dbuser"`
	Dbpass         string `yaml:"dbpass"`
	Dbdatabasename string `yaml:"dbdatabasename"`

	// Skip downloading media from promoted (ad) tweets.
	// Default: false (download promoted tweets too).
	Exceptpromoted bool `yaml:"exceptpromoted"`
}

// Load reads the config from path. If path is empty, it searches in order:
//  1. config.yaml in the same directory as the executable (binary)
//  2. config.yaml in the current working directory (CWD) ← fallback for go run
//
// config.yaml is optional when no explicit path is given.
// If the file is not found, default values are used:
//   - datadir  → "data" (set by main.go)
//   - db*      → DB connection skipped
//
// If an explicit -config path is provided but the file does not exist, an error is returned.
func Load(path string) (*Config, error) {
	explicit := path != ""
	if !explicit {
		path = resolveConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !explicit && os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
	}
	return &cfg, nil
}

// resolveConfigPath returns the path to config.yaml, checking executable dir
// first (for compiled binary) and then CWD (for go run).
func resolveConfigPath() string {
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "config.yaml"
}
