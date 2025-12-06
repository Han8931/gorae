package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	WatchDir string `json:"watch_dir"`
	MetaDir  string `json:"meta_dir"`
}

func defaultConfigPath() (string, error) {
	cfgHome := os.Getenv("XDG_CONFIG_HOME")
	if cfgHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cfgHome = filepath.Join(home, ".config")
	}
	return filepath.Join(cfgHome, "pdf-tui", "config.json"), nil
}

// Path returns the full path to the config file, using the same rules as LoadOrInit.
func Path() (string, error) {
	return defaultConfigPath()
}

func defaultWatchDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("PDF_TUI_WATCH_DIR")); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Documents", "Papers"), nil
}

func defaultMetaDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("PDF_TUI_META_DIR")); v != "" {
		return v, nil
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "pdf-tui"), nil
}

func LoadOrInit() (*Config, error) {
	path, err := defaultConfigPath()
	if err != nil {
		return nil, err
	}

	// existing config
	if data, err := os.ReadFile(path); err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}

	// first run: create config from defaults so the app starts immediately.
	fmt.Println("No config found. Creating one with defaults.")
	watch, err := defaultWatchDir()
	if err != nil {
		return nil, err
	}
	meta, err := defaultMetaDir()
	if err != nil {
		return nil, err
	}
	cfg := &Config{WatchDir: watch, MetaDir: meta}
	fmt.Printf("  watch_dir: %s\n", cfg.WatchDir)
	fmt.Printf("  meta_dir : %s\n", cfg.MetaDir)
	fmt.Printf("Edit %s to change these paths.\n", path)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.MetaDir, 0o755); err != nil {
		return nil, err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}

	fmt.Println("Config saved to", path)
	return cfg, nil
}
