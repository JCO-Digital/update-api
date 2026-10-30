package backup

import (
	"os"
	"path/filepath"
	"testing"
	"update-api/internal/repository"
)

func TestSnapshotAndPrune(t *testing.T) {
	dir := t.TempDir()
	repo, err := repository.NewSQLiteRepository(filepath.Join(dir, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(); err != nil {
		t.Fatal(err)
	}
	_ = repo.SavePlugin(repository.Plugin{Slug: "p", Name: "P"})

	backups := filepath.Join(dir, "backups")
	path, err := Snapshot(repo, backups)
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	// The backup is a working database with the data in it.
	restored, err := repository.NewSQLiteRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := restored.GetPlugin("p")
	restored.Close()
	if err != nil || p == nil {
		t.Fatalf("Backup does not contain the plugin: %v", err)
	}

	for _, name := range []string{"updates-20260101T000000Z.db", "updates-20260102T000000Z.db", "unrelated.txt"} {
		os.WriteFile(filepath.Join(backups, name), nil, 0o600)
	}
	if err := Prune(backups, 2); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(backups)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	// Kept: the newest two backups and the unrelated file.
	if len(names) != 3 || names[0] != "unrelated.txt" || names[1] != "updates-20260102T000000Z.db" || names[2] != filepath.Base(path) {
		t.Errorf("Unexpected files after prune: %v", names)
	}
}
