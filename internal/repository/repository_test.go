package repository

import (
	"path/filepath"
	"testing"
)

func TestPluginAndVersionRepository(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_updates.db")

	repo, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("Failed to open repo: %v", err)
	}
	defer repo.Close()

	// Run migration
	err = repo.Migrate()
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Test Save/Get Plugin
	p := Plugin{
		Slug:   "test-plugin",
		Name:   "Test Plugin",
		Secret: "secret123",
		IsPaid: true,
	}
	err = repo.SavePlugin(p)
	if err != nil {
		t.Fatalf("SavePlugin failed: %v", err)
	}

	retrieved, err := repo.GetPlugin(p.Slug)
	if err != nil || retrieved == nil {
		t.Fatalf("GetPlugin failed: %v", err)
	}
	if retrieved.Name != p.Name {
		t.Errorf("Expected name %s, got %s", p.Name, retrieved.Name)
	}

	// Test Save/List Versions
	v1 := Version{
		PluginSlug:  p.Slug,
		Version:     "1.0.0",
		DownloadURL: "http://example.com/1.0.0.zip",
	}
	v2 := Version{
		PluginSlug:  p.Slug,
		Version:     "1.1.0",
		DownloadURL: "http://example.com/1.1.0.zip",
	}
	_ = repo.SaveVersion(v1)
	_ = repo.SaveVersion(v2)

	versions, err := repo.ListVersions(p.Slug)
	if err != nil || len(versions) != 2 {
		t.Errorf("Expected 2 versions, got %d", len(versions))
	}

	// Test SemVer GetLatest
	latest, err := repo.GetLatestVersion(p.Slug)
	if err != nil || latest == nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}
	if latest.Version != "1.1.0" {
		t.Errorf("Expected latest version 1.1.0, got %s", latest.Version)
	}

	// Test Cascade Delete
	_, _ = repo.db.Exec("DELETE FROM plugins WHERE slug = ?", p.Slug)
	versionsAfterDelete, _ := repo.ListVersions(p.Slug)
	if len(versionsAfterDelete) != 0 {
		t.Errorf("Versions should have been cascaded, but %d remain", len(versionsAfterDelete))
	}
}
