package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigBackupDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("XDG_DATA_HOME", "/data")

	os.WriteFile(path, []byte("server:\n  port: 9000\n"), 0o600)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9000 || cfg.Server.DBPath != "/data/update-api/updates.db" {
		t.Errorf("Unexpected server config: %+v", cfg.Server)
	}
	if cfg.Backup.Interval != 6*time.Hour || cfg.Backup.Keep != 28 || cfg.Backup.Dir != "/data/update-api/backups" {
		t.Errorf("Unexpected backup defaults: %+v", cfg.Backup)
	}

	os.WriteFile(path, []byte("backup:\n  interval: 30m\n  keep: 5\n"), 0o600)
	cfg, err = LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backup.Interval != 30*time.Minute || cfg.Backup.Keep != 5 {
		t.Errorf("Unexpected backup config: %+v", cfg.Backup)
	}

	os.WriteFile(path, []byte("backup:\n  interval: 0s\n"), 0o600)
	cfg, err = LoadConfig(path)
	if err != nil || cfg.Backup.Interval != 0 {
		t.Errorf("Expected interval 0s to disable backups, got %v (%v)", cfg.Backup.Interval, err)
	}
}

func TestDataDir(t *testing.T) {
	t.Setenv("HOME", "/home/test")

	t.Setenv("XDG_DATA_HOME", "/var/lib")
	if dir, _ := DataDir(); dir != "/var/lib/update-api" {
		t.Errorf("Expected XDG_DATA_HOME to be used, got %s", dir)
	}

	for _, xdg := range []string{"", "relative/path"} {
		t.Setenv("XDG_DATA_HOME", xdg)
		if dir, _ := DataDir(); dir != "/home/test/.local/share/update-api" {
			t.Errorf("XDG_DATA_HOME=%q: expected ~/.local/share fallback, got %s", xdg, dir)
		}
	}
}

func TestLoadSearchesXDGConfigDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("UPDATE_API_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "home"))
	t.Setenv("XDG_CONFIG_DIRS", "relative:"+filepath.Join(tmp, "etc1")+":"+filepath.Join(tmp, "etc2"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	paths, err := ConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(tmp, "home", "update-api", "config.yaml"),
		filepath.Join(tmp, "etc1", "update-api", "config.yaml"),
		filepath.Join(tmp, "etc2", "update-api", "config.yaml"),
	}
	if len(paths) != len(want) {
		t.Fatalf("ConfigPaths() = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("ConfigPaths()[%d] = %s, want %s", i, paths[i], want[i])
		}
	}

	// No file anywhere: defaults, no admin keys.
	cfg, path, err := Load()
	if err != nil || path != "" {
		t.Fatalf("Expected defaults, got path %q err %v", path, err)
	}
	if len(cfg.Auth.AdminAPIKeys) != 0 || cfg.Licenses.LogDir != filepath.Join(tmp, "data", "update-api", "licenses") {
		t.Errorf("Unexpected defaults: %+v", cfg)
	}

	// A system-wide file is found, and the user's own file wins over it.
	write := func(p, port string) {
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, []byte("server:\n  port: "+port+"\n"), 0o600)
	}
	write(want[2], "9002")
	if cfg, path, _ := Load(); path != want[2] || cfg.Server.Port != 9002 {
		t.Errorf("Expected %s, got %s (port %d)", want[2], path, cfg.Server.Port)
	}
	write(want[0], "9000")
	if cfg, path, _ := Load(); path != want[0] || cfg.Server.Port != 9000 {
		t.Errorf("Expected %s, got %s (port %d)", want[0], path, cfg.Server.Port)
	}

	// UPDATE_API_CONFIG overrides the search.
	explicit := filepath.Join(tmp, "explicit.yaml")
	os.WriteFile(explicit, nil, 0o600)
	t.Setenv("UPDATE_API_CONFIG", explicit)
	if cfg, path, err := Load(); err != nil || path != explicit || cfg.Server.Port != 8080 {
		t.Errorf("Expected empty explicit config with defaults, got %s port %d err %v", path, cfg.Server.Port, err)
	}
}

func TestStrayFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	cfg, err := defaults()
	if err != nil {
		t.Fatal(err)
	}
	if msgs := cfg.StrayFiles(""); len(msgs) != 0 {
		t.Errorf("Expected no warnings, got %v", msgs)
	}

	os.WriteFile("config.yaml", nil, 0o600)
	os.WriteFile("updates.db", nil, 0o600)
	os.Mkdir("licenses", 0o700)
	if msgs := cfg.StrayFiles(""); len(msgs) != 3 {
		t.Errorf("Expected 3 warnings, got %v", msgs)
	}

	// Files that are the ones in use are not stray.
	cfg.Server.DBPath = "updates.db"
	cfg.Licenses.LogDir = filepath.Join(tmp, "licenses")
	if msgs := cfg.StrayFiles("config.yaml"); len(msgs) != 0 {
		t.Errorf("Expected no warnings, got %v", msgs)
	}
}
