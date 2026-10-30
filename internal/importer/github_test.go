package importer

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"update-api/internal/repository"
)

const testHeader = `<?php
/**
 * Plugin Name:       JCORE Example
 * Version:           1.1.0
 * Requires at least: 6.7
 * Tested up to:      7.1
 * Requires PHP:      8.2
 */
`

func TestRun(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /search/repositories", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "org:JCO-Digital topic:jcore-plugin" {
			t.Errorf("Unexpected search query %q", r.URL.Query().Get("q"))
		}
		w.Write([]byte(`{"items":[{"name":"jcore-example"},{"name":"jcore-empty"},{"name":"jcore-sub"}]}`))
	})
	mux.HandleFunc("GET /repos/JCO-Digital/jcore-example/releases", func(w http.ResponseWriter, r *http.Request) {
		// Newest first, like GitHub. v0.9.0 has no zip, v2.0.0 is a draft.
		w.Write([]byte(`[
			{"tag_name":"v2.0.0","draft":true,"published_at":"2026-03-01T00:00:00Z","assets":[{"name":"jcore-example.zip","browser_download_url":"https://dl/2.0.0.zip"}]},
			{"tag_name":"v1.1.0","body":"= 1.1.0 =\n* Feature","published_at":"2026-02-01T00:00:00Z","assets":[{"name":"jcore-example.zip","browser_download_url":"https://dl/1.1.0.zip"}]},
			{"tag_name":"v1.0.0","body":"= 1.0.0 =","published_at":"2026-01-01T00:00:00Z","assets":[{"name":"jcore-example.zip","browser_download_url":"https://dl/1.0.0.zip"}]},
			{"tag_name":"v0.9.0","published_at":"2025-12-01T00:00:00Z","assets":[]}
		]`))
	})
	mux.HandleFunc("GET /repos/JCO-Digital/jcore-empty/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	})
	mux.HandleFunc("GET /raw/JCO-Digital/jcore-example/{tag}/jcore-example.php", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testHeader))
	})
	var srv *httptest.Server
	mux.HandleFunc("GET /repos/JCO-Digital/jcore-sub/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"tag_name":"v3.0.0","published_at":"2026-01-01T00:00:00Z","assets":[{"name":"jcore-sub.zip","browser_download_url":"` + srv.URL + `/dl/jcore-sub.zip"}]}]`))
	})
	mux.HandleFunc("GET /dl/jcore-sub.zip", func(w http.ResponseWriter, r *http.Request) {
		zw := zip.NewWriter(w)
		f, _ := zw.Create("jcore-sub/jcore-sub.php")
		f.Write([]byte(strings.Replace(testHeader, "JCORE Example", "JCORE Sub", 1)))
		zw.Close()
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "import.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(); err != nil {
		t.Fatal(err)
	}

	gh := GitHub{Org: "JCO-Digital", Topic: "jcore-plugin", APIBase: srv.URL, RawBase: srv.URL + "/raw"}

	sum, err := gh.Run(context.Background(), repo, true)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Versions != 3 {
		t.Errorf("Dry run: expected 3 versions, got %d", sum.Versions)
	}
	if p, _ := repo.GetPlugin("jcore-example"); p != nil {
		t.Error("Dry run must not write")
	}

	sum, err = gh.Run(context.Background(), repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Repos != 3 || sum.PluginsCreated != 2 || sum.Versions != 3 {
		t.Errorf("Unexpected summary: %+v", sum)
	}

	p, _ := repo.GetPlugin("jcore-example")
	if p == nil || p.Name != "JCORE Example" || p.IsPaid || p.Secret != "" {
		t.Fatalf("Unexpected plugin: %+v", p)
	}

	versions, _ := repo.ListVersions("jcore-example")
	if len(versions) != 2 {
		t.Fatalf("Expected 2 versions, got %d", len(versions))
	}
	// ListVersions is newest id first; ids follow release order.
	v := versions[0]
	if v.Version != "1.1.0" || v.DownloadURL != "https://dl/1.1.0.zip" || v.Changelog != "= 1.1.0 =\n* Feature" {
		t.Errorf("Unexpected version: %+v", v)
	}
	if v.RequiresWP != "6.7" || v.TestedWP != "7.1" || v.RequiresPHP != "8.2" {
		t.Errorf("Unexpected requirements: %+v", v)
	}
	if v.CreatedAt != "2026-02-01T00:00:00Z" {
		t.Errorf("Expected release date as created_at, got %s", v.CreatedAt)
	}

	sub, _ := repo.GetLatestVersion("jcore-sub")
	if p, _ := repo.GetPlugin("jcore-sub"); p == nil || p.Name != "JCORE Sub" || sub == nil || sub.RequiresPHP != "8.2" {
		t.Errorf("Header not read from zip: plugin %+v, version %+v", p, sub)
	}

	// Importing again changes nothing.
	if _, err := gh.Run(context.Background(), repo, false); err != nil {
		t.Fatal(err)
	}
	if again, _ := repo.ListVersions("jcore-example"); len(again) != 2 {
		t.Errorf("Re-import duplicated versions: %d", len(again))
	}
}
