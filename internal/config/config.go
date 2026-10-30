package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const appName = "update-api"

type Config struct {
	Server struct {
		Port   int    `yaml:"port"`
		DBPath string `yaml:"db_path"`
	} `yaml:"server"`
	Auth struct {
		AdminAPIKeys []string `yaml:"admin_api_keys"`
	} `yaml:"auth"`
	Backup struct {
		// Dir holds the periodic snapshots. Keep it out of anything that
		// gets cleaned or redeployed.
		Dir string `yaml:"dir"`
		// Interval between snapshots, e.g. "6h". "0s" disables backups.
		Interval time.Duration `yaml:"interval"`
		// Keep is how many snapshots to retain; older ones are deleted.
		Keep int `yaml:"keep"`
	} `yaml:"backup"`
	Licenses struct {
		// LogDir holds the "created" and "validated" license logs.
		LogDir string `yaml:"log_dir"`
	} `yaml:"licenses"`
}

// Load reads the config from $UPDATE_API_CONFIG if set, otherwise from the
// first update-api/config.yaml in the XDG config directories. With no config
// file it returns the defaults, which have no admin API keys, and an empty
// path.
func Load() (*Config, string, error) {
	if path := os.Getenv("UPDATE_API_CONFIG"); path != "" {
		cfg, err := LoadConfig(path)
		return cfg, path, err
	}

	paths, err := ConfigPaths()
	if err != nil {
		return nil, "", err
	}
	for _, path := range paths {
		cfg, err := LoadConfig(path)
		if err == nil {
			return cfg, path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, path, err
		}
	}

	cfg, err := defaults()
	return cfg, "", err
}

// ConfigPaths lists where Load looks for config.yaml, most specific first:
// $XDG_CONFIG_HOME (default ~/.config), then each of $XDG_CONFIG_DIRS
// (default /etc/xdg).
func ConfigPaths() ([]string, error) {
	home, err := xdgDir("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return nil, err
	}
	paths := []string{filepath.Join(home, appName, "config.yaml")}

	dirs := os.Getenv("XDG_CONFIG_DIRS")
	if dirs == "" {
		dirs = "/etc/xdg"
	}
	for _, dir := range strings.Split(dirs, ":") {
		// The XDG spec says to ignore relative paths.
		if filepath.IsAbs(dir) {
			paths = append(paths, filepath.Join(dir, appName, "config.yaml"))
		}
	}
	return paths, nil
}

// DataDir is where the database, backups and license logs live unless the
// config says otherwise: $XDG_DATA_HOME/update-api, or
// ~/.local/share/update-api.
func DataDir() (string, error) {
	dir, err := xdgDir("XDG_DATA_HOME", filepath.Join(".local", "share"))
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName), nil
}

// xdgDir returns $env if it is absolute, otherwise ~/fallback.
func xdgDir(env, fallback string) (string, error) {
	// The XDG spec says to ignore relative paths.
	if dir := os.Getenv(env); filepath.IsAbs(dir) {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding %s: %w", env, err)
	}
	return filepath.Join(home, fallback), nil
}

func defaults() (*Config, error) {
	dataDir, err := DataDir()
	if err != nil {
		return nil, err
	}

	var cfg Config
	cfg.Server.Port = 8080
	cfg.Server.DBPath = filepath.Join(dataDir, "updates.db")
	cfg.Backup.Dir = filepath.Join(dataDir, "backups")
	cfg.Backup.Interval = 6 * time.Hour
	cfg.Backup.Keep = 28
	cfg.Licenses.LogDir = filepath.Join(dataDir, "licenses")
	return &cfg, nil
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cfg, err := defaults()
	if err != nil {
		return nil, err
	}

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return cfg, nil
}

// StrayFiles describes files in the working directory where older versions
// kept the config, database and license logs, when they are not the ones in
// use. Starting on an empty database next to a full one looks like lost
// data, so callers should log these.
func (c *Config) StrayFiles(configPath string) []string {
	var msgs []string
	check := func(legacy, inUse, what string) {
		stray, err := filepath.Abs(legacy)
		if err != nil {
			return
		}
		if abs, err := filepath.Abs(inUse); err == nil && abs == stray {
			return
		}
		if _, err := os.Stat(stray); err == nil {
			msgs = append(msgs, fmt.Sprintf("%s exists but is not in use; move it to %s, or point %s at it", stray, inUse, what))
		}
	}

	wantConfig := configPath
	if wantConfig == "" {
		if paths, err := ConfigPaths(); err == nil {
			wantConfig = paths[0]
		}
	}
	check("config.yaml", wantConfig, "UPDATE_API_CONFIG")
	check("updates.db", c.Server.DBPath, "server.db_path")
	check("licenses", c.Licenses.LogDir, "licenses.log_dir")
	return msgs
}
