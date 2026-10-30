package repository

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil {
		return nil, err
	}

	// Pragmas go in the DSN so every pooled connection gets them, not just
	// the first one. WAL lets the CLI write while the server is reading.
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?"+q.Encode())
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

type Plugin struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Secret string `json:"secret"`
	IsPaid bool   `json:"is_paid"`
}

type Version struct {
	ID          int    `json:"id"`
	PluginSlug  string `json:"slug"`
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	RequiresWP  string `json:"requires_wp"`
	TestedWP    string `json:"tested_wp"`
	RequiresPHP string `json:"requires_php"`
	Changelog   string `json:"changelog"`
	CreatedAt   string `json:"created_at"`
}

// Migrate applies the embedded migrations newer than the database's
// user_version, each in its own transaction.
func (r *SQLiteRepository) Migrate() error {
	files, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)

	var current int
	if err := r.db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return err
	}

	for _, file := range files {
		n, err := strconv.Atoi(strings.SplitN(strings.TrimPrefix(file, "migrations/"), "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: name must start with a number", file)
		}
		if n <= current {
			continue
		}

		content, err := migrationFS.ReadFile(file)
		if err != nil {
			return err
		}

		tx, err := r.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", file, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", n)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Backup writes a consistent copy of the live database to destPath, which
// must not exist yet.
func (r *SQLiteRepository) Backup(destPath string) error {
	_, err := r.db.Exec("VACUUM INTO ?", destPath)
	return err
}

// SavePlugin creates or updates a plugin. It must not be INSERT OR REPLACE:
// REPLACE deletes the old row, and the ON DELETE CASCADE takes every version
// of the plugin with it.
func (r *SQLiteRepository) SavePlugin(p Plugin) error {
	query := `INSERT INTO plugins (slug, name, secret, is_paid) VALUES (?, ?, ?, ?)
              ON CONFLICT(slug) DO UPDATE SET name = excluded.name, secret = excluded.secret, is_paid = excluded.is_paid`
	_, err := r.db.Exec(query, p.Slug, p.Name, p.Secret, p.IsPaid)
	return err
}

// EnsurePlugin creates the plugin if it does not exist and leaves an
// existing one (and its secret) untouched. It reports whether it created it.
func (r *SQLiteRepository) EnsurePlugin(p Plugin) (bool, error) {
	query := `INSERT INTO plugins (slug, name, secret, is_paid) VALUES (?, ?, ?, ?)
              ON CONFLICT(slug) DO NOTHING`
	res, err := r.db.Exec(query, p.Slug, p.Name, p.Secret, p.IsPaid)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *SQLiteRepository) GetPlugin(slug string) (*Plugin, error) {
	query := `SELECT slug, name, secret, is_paid FROM plugins WHERE slug = ?`
	row := r.db.QueryRow(query, slug)
	var p Plugin
	var secret sql.NullString
	if err := row.Scan(&p.Slug, &p.Name, &secret, &p.IsPaid); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	p.Secret = secret.String
	return &p, nil
}

// SaveVersion adds a version, or updates it if the plugin already has that
// version, so re-publishing a release is safe. CreatedAt is optional and
// defaults to now.
func (r *SQLiteRepository) SaveVersion(v Version) error {
	query := `INSERT INTO versions (plugin_slug, version, download_url, requires_wp, tested_wp, requires_php, changelog, created_at)
              VALUES (?, ?, ?, ?, ?, ?, ?, COALESCE(NULLIF(?, ''), CURRENT_TIMESTAMP))
              ON CONFLICT(plugin_slug, version) DO UPDATE SET
                  download_url = excluded.download_url,
                  requires_wp = excluded.requires_wp,
                  tested_wp = excluded.tested_wp,
                  requires_php = excluded.requires_php,
                  changelog = excluded.changelog`
	_, err := r.db.Exec(query, v.PluginSlug, v.Version, v.DownloadURL, v.RequiresWP, v.TestedWP, v.RequiresPHP, v.Changelog, v.CreatedAt)
	return err
}

func (r *SQLiteRepository) ListPlugins() ([]Plugin, error) {
	query := `SELECT slug, name, secret, is_paid FROM plugins ORDER BY slug`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plugins []Plugin
	for rows.Next() {
		var p Plugin
		var secret sql.NullString
		if err := rows.Scan(&p.Slug, &p.Name, &secret, &p.IsPaid); err != nil {
			return nil, err
		}
		p.Secret = secret.String
		plugins = append(plugins, p)
	}
	return plugins, rows.Err()
}

func (r *SQLiteRepository) ListVersions(slug string) ([]Version, error) {
	query := `SELECT id, plugin_slug, version, download_url,
                     COALESCE(requires_wp, ''), COALESCE(tested_wp, ''), COALESCE(requires_php, ''), COALESCE(changelog, ''), created_at
              FROM versions WHERE plugin_slug = ? ORDER BY id DESC`
	rows, err := r.db.Query(query, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []Version
	for rows.Next() {
		var v Version
		err := rows.Scan(&v.ID, &v.PluginSlug, &v.Version, &v.DownloadURL, &v.RequiresWP, &v.TestedWP, &v.RequiresPHP, &v.Changelog, &v.CreatedAt)
		if err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// GetLatestVersion returns the highest SemVer version of a plugin, or nil.
func (r *SQLiteRepository) GetLatestVersion(slug string) (*Version, error) {
	versions, err := r.ListVersions(slug)
	if err != nil {
		return nil, err
	}

	var latest *Version
	var latestSemVer *semver.Version
	for i, v := range versions {
		sv, err := semver.NewVersion(v.Version)
		if err != nil {
			// Skip invalid semver versions in DB
			continue
		}
		if latestSemVer == nil || sv.GreaterThan(latestSemVer) {
			latestSemVer = sv
			latest = &versions[i]
		}
	}
	return latest, nil
}
