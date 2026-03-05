// Package config loads program configuration from a JSON file.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all configuration for x-media-downloader.
// No username/password fields — authentication is handled via cookieswithchromedp.
type Config struct {
	Datadir        string `json:"datadir"`
	Debug          bool   `json:"debug"`

	Dbhost         string `json:"dbhost"`
	Dbuser         string `json:"dbuser"`
	Dbpass         string `json:"dbpass"`
	Dbdatabasename string `json:"dbdatabasename"`

	// NSFW detection (optional)
	// nsfwmodelpath  : ONNX model file path (empty = NSFW detection disabled)
	// onnxlibpath    : ONNX Runtime shared library path (empty = system default)
	// nsfwinputname  : ONNX model input tensor name
	// nsfwoutputname : ONNX model output tensor name
	Nsfwmodelpath  string `json:"nsfwmodelpath"`
	Onnxlibpath    string `json:"onnxlibpath"`
	Nsfwinputname  string `json:"nsfwinputname"`
	Nsfwoutputname string `json:"nsfwoutputname"`
}

// Load reads the config from path. If path is empty, it searches in order:
//  1. config.json in the same directory as the executable (binary)
//  2. config.json in the current working directory (CWD) ← fallback for go run
//
// Returns an error if the file is not found.
func Load(path string) (*Config, error) {
	if path == "" {
		path = resolveConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
	}
	return &cfg, nil
}

// resolveConfigPath returns the path to config.json, checking executable dir
// first (for compiled binary) and then CWD (for go run).
func resolveConfigPath() string {
	// 1. executable directory
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "config.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// 2. CWD fallback (for go run)
	return "config.json"
}
