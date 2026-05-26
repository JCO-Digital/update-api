package repository

import (
	"database/sql"
	"os"

	"github.com/Masterminds/semver/v3"
	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	// Enable foreign key constraints
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
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

func (r *SQLiteRepository) Migrate(migrationPath string) error {
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(string(content))
	return err
}

func (r *SQLiteRepository) SavePlugin(p Plugin) error {
	query := `INSERT OR REPLACE INTO plugins (slug, name, secret, is_paid) VALUES (?, ?, ?, ?)`
	_, err := r.db.Exec(query, p.Slug, p.Name, p.Secret, p.IsPaid)
	return err
}

func (r *SQLiteRepository) GetPlugin(slug string) (*Plugin, error) {
	query := `SELECT slug, name, secret, is_paid FROM plugins WHERE slug = ?`
	row := r.db.QueryRow(query, slug)
	var p Plugin
	if err := row.Scan(&p.Slug, &p.Name, &p.Secret, &p.IsPaid); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *SQLiteRepository) SaveVersion(v Version) error {
	query := `INSERT INTO versions (plugin_slug, version, download_url, requires_wp, tested_wp, requires_php, changelog)
              VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.Exec(query, v.PluginSlug, v.Version, v.DownloadURL, v.RequiresWP, v.TestedWP, v.RequiresPHP, v.Changelog)
	return err
}

func (r *SQLiteRepository) GetLatestVersion(slug string) (*Version, error) {
	query := `SELECT id, plugin_slug, version, download_url, requires_wp, tested_wp, requires_php, changelog, created_at
              FROM versions WHERE plugin_slug = ?`
	rows, err := r.db.Query(query, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var latestVersion *Version
	var latestSemVer *semver.Version

	for rows.Next() {
		var v Version
		err := rows.Scan(&v.ID, &v.PluginSlug, &v.Version, &v.DownloadURL, &v.RequiresWP, &v.TestedWP, &v.RequiresPHP, &v.Changelog, &v.CreatedAt)
		if err != nil {
			return nil, err
		}

		currentSemVer, err := semver.NewVersion(v.Version)
		if err != nil {
			// Skip invalid semver versions in DB
			continue
		}

		if latestSemVer == nil || currentSemVer.GreaterThan(latestSemVer) {
			latestSemVer = currentSemVer
			latestVersion = &v
		}
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return latestVersion, nil
}
