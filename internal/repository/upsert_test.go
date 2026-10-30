package repository

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Failed to open repo: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	if err := repo.Migrate(); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}
	return repo
}

func TestSavePluginKeepsVersions(t *testing.T) {
	repo := newTestRepo(t)

	_ = repo.SavePlugin(Plugin{Slug: "p", Name: "P"})
	_ = repo.SaveVersion(Version{PluginSlug: "p", Version: "1.0.0", DownloadURL: "x"})
	if err := repo.SavePlugin(Plugin{Slug: "p", Name: "Renamed", Secret: "s", IsPaid: true}); err != nil {
		t.Fatal(err)
	}

	versions, _ := repo.ListVersions("p")
	if len(versions) != 1 {
		t.Errorf("Re-saving a plugin must keep its versions, got %d", len(versions))
	}
	p, _ := repo.GetPlugin("p")
	if p.Name != "Renamed" || p.Secret != "s" || !p.IsPaid {
		t.Errorf("Plugin not updated: %+v", p)
	}
}

func TestEnsurePluginKeepsExisting(t *testing.T) {
	repo := newTestRepo(t)

	_ = repo.SavePlugin(Plugin{Slug: "p", Name: "P", Secret: "s", IsPaid: true})
	created, err := repo.EnsurePlugin(Plugin{Slug: "p", Name: "Other"})
	if err != nil || created {
		t.Fatalf("Expected existing plugin to be kept, created=%v err=%v", created, err)
	}
	p, _ := repo.GetPlugin("p")
	if p.Name != "P" || p.Secret != "s" || !p.IsPaid {
		t.Errorf("Existing plugin was changed: %+v", p)
	}

	created, _ = repo.EnsurePlugin(Plugin{Slug: "q", Name: "Q"})
	if !created {
		t.Error("Expected new plugin to be created")
	}
}

func TestSaveVersionUpserts(t *testing.T) {
	repo := newTestRepo(t)

	_ = repo.SavePlugin(Plugin{Slug: "p", Name: "P"})
	_ = repo.SaveVersion(Version{PluginSlug: "p", Version: "1.0.0", DownloadURL: "old", CreatedAt: "2026-01-02 03:04:05"})
	if err := repo.SaveVersion(Version{PluginSlug: "p", Version: "1.0.0", DownloadURL: "new"}); err != nil {
		t.Fatal(err)
	}

	versions, _ := repo.ListVersions("p")
	if len(versions) != 1 {
		t.Fatalf("Expected 1 version, got %d", len(versions))
	}
	if versions[0].DownloadURL != "new" {
		t.Errorf("Expected updated download URL, got %s", versions[0].DownloadURL)
	}
	if versions[0].CreatedAt != "2026-01-02T03:04:05Z" {
		t.Errorf("Expected original created_at to be kept, got %s", versions[0].CreatedAt)
	}
}

func TestMigrateDedupesLegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// A database as the old server left it: schema 001, no user_version,
	// and the same version published twice.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	schema, _ := migrationFS.ReadFile("migrations/001_initial_schema.sql")
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO plugins (slug, name) VALUES ('p', 'P');
		INSERT INTO versions (plugin_slug, version, download_url) VALUES ('p', '1.0.0', 'first'), ('p', '1.0.0', 'second'), ('p', '1.1.0', 'x');`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	repo, err := NewSQLiteRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}
	// Running again is a no-op.
	if err := repo.Migrate(); err != nil {
		t.Fatalf("Second migration failed: %v", err)
	}

	versions, _ := repo.ListVersions("p")
	if len(versions) != 2 {
		t.Fatalf("Expected 2 versions after dedupe, got %d", len(versions))
	}
	for _, v := range versions {
		if v.Version == "1.0.0" && v.DownloadURL != "second" {
			t.Errorf("Expected newest duplicate to be kept, got %s", v.DownloadURL)
		}
	}
}
