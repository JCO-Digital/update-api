package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"update-api/internal/repository"
)

func TestHandlers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_api.db")

	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("Failed to open repo: %v", err)
	}
	defer repo.Close()

	err = repo.Migrate()
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	h := NewHandler(repo)

	t.Run("PostPlugin", func(t *testing.T) {
		payload := map[string]interface{}{
			"slug":    "test-api-plugin",
			"name":    "Test API Plugin",
			"secret":  "api-secret",
			"is_paid": true,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/v1/plugins", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		h.HandlePostPlugin(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", w.Code)
		}
	})

	t.Run("UpdateCheck_NoVersion", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/update-check?slug=test-api-plugin&version=1.0.0&license_key=invalid", nil)
		w := httptest.NewRecorder()

		h.HandleUpdateCheck(w, req)

		// Should fail because license is invalid and it's a paid plugin
		if w.Code != http.StatusForbidden {
			t.Errorf("Expected status 403 for invalid license, got %d", w.Code)
		}
	})

	t.Run("ValidateLicense_RateLimit", func(t *testing.T) {
		payload := map[string]string{
			"slug":        "test-api-plugin",
			"license_key": "some-key",
		}
		body, _ := json.Marshal(payload)

		// Run 7 times to trigger rate limit (limit is 6)
		for i := 0; i < 7; i++ {
			req := httptest.NewRequest("POST", "/v1/licenses/validate", bytes.NewBuffer(body))
			req.RemoteAddr = "1.2.3.4:1234"
			w := httptest.NewRecorder()
			h.HandleValidateLicense(w, req)

			if i < 6 && w.Code == http.StatusTooManyRequests {
				t.Errorf("Rate limited too early at request %d", i+1)
			}
			if i == 6 && w.Code != http.StatusTooManyRequests {
				t.Error("Expected 429 Too Many Requests on 7th call")
			}
		}
	})

	// Add a free plugin and multiple versions to test update channels
	t.Run("UpdateChannels", func(t *testing.T) {
		freePlugin := repository.Plugin{
			Slug:   "semver-plugin",
			Name:   "Semver Plugin",
			IsPaid: false,
		}
		if err := repo.SavePlugin(freePlugin); err != nil {
			t.Fatalf("Failed to create free plugin: %v", err)
		}

		versions := []string{"1.2.0", "1.2.1", "1.2.4", "1.3.0", "1.4.3", "2.0.0", "2.0.3"}
		for _, v := range versions {
			err := repo.SaveVersion(repository.Version{
				PluginSlug:  "semver-plugin",
				Version:     v,
				DownloadURL: "http://example.com/" + v + ".zip",
				Changelog:   "Release " + v,
			})
			if err != nil {
				t.Fatalf("Failed to save version %s: %v", v, err)
			}
		}

		// 1. Default (major): client is on 1.2.0, should get 2.0.3
		req := httptest.NewRequest("GET", "/v1/update-check?slug=semver-plugin&version=1.2.0", nil)
		w := httptest.NewRecorder()
		h.HandleUpdateCheck(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", w.Code)
		}
		var resp UpdateResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if resp.NewVersion != "2.0.3" {
			t.Errorf("Expected default/major channel version 2.0.3, got %s", resp.NewVersion)
		}

		// 2. Channel = minor: client on 1.2.0, should get 1.4.3
		req = httptest.NewRequest("GET", "/v1/update-check?slug=semver-plugin&version=1.2.0&channel=minor", nil)
		w = httptest.NewRecorder()
		h.HandleUpdateCheck(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", w.Code)
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if resp.NewVersion != "1.4.3" {
			t.Errorf("Expected minor channel version 1.4.3, got %s", resp.NewVersion)
		}

		// 3. Channel = patch: client on 1.2.0, should get 1.2.4
		req = httptest.NewRequest("GET", "/v1/update-check?slug=semver-plugin&version=1.2.0&channel=patch", nil)
		w = httptest.NewRecorder()
		h.HandleUpdateCheck(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", w.Code)
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if resp.NewVersion != "1.2.4" {
			t.Errorf("Expected patch channel version 1.2.4, got %s", resp.NewVersion)
		}

		// 4. Channel = patch: client on 1.4.3, no newer patch in 1.4.x -> 204 No Content
		req = httptest.NewRequest("GET", "/v1/update-check?slug=semver-plugin&version=1.4.3&channel=patch", nil)
		w = httptest.NewRecorder()
		h.HandleUpdateCheck(w, req)
		if w.Code != http.StatusNoContent {
			t.Errorf("Expected 204 No Content for patch channel with no patch update, got %d", w.Code)
		}

		// 5. Channel = all (or all=true): returns grouped updates and version list
		req = httptest.NewRequest("GET", "/v1/update-check?slug=semver-plugin&version=1.2.0&channel=all", nil)
		w = httptest.NewRecorder()
		h.HandleUpdateCheck(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", w.Code)
		}
		var grouped GroupedUpdateResponse
		if err := json.NewDecoder(w.Body).Decode(&grouped); err != nil {
			t.Fatalf("Failed to decode grouped response: %v", err)
		}
		if grouped.Patch == nil || grouped.Patch.NewVersion != "1.2.4" {
			t.Errorf("Expected grouped patch 1.2.4, got %+v", grouped.Patch)
		}
		if grouped.Minor == nil || grouped.Minor.NewVersion != "1.4.3" {
			t.Errorf("Expected grouped minor 1.4.3, got %+v", grouped.Minor)
		}
		if grouped.Major == nil || grouped.Major.NewVersion != "2.0.3" {
			t.Errorf("Expected grouped major 2.0.3, got %+v", grouped.Major)
		}
		if len(grouped.Versions) != 6 {
			t.Errorf("Expected 6 newer versions, got %d", len(grouped.Versions))
		}
		if len(grouped.Versions) > 0 && grouped.Versions[0].NewVersion != "2.0.3" {
			t.Errorf("Expected first version in list to be 2.0.3, got %s", grouped.Versions[0].NewVersion)
		}

		// 6. all=true query parameter works the same as channel=all
		req = httptest.NewRequest("GET", "/v1/update-check?slug=semver-plugin&version=1.2.0&all=true", nil)
		w = httptest.NewRecorder()
		h.HandleUpdateCheck(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 for all=true, got %d", w.Code)
		}

		// 7. Already on newest version (2.0.3) -> 204 No Content for all modes
		req = httptest.NewRequest("GET", "/v1/update-check?slug=semver-plugin&version=2.0.3&all=true", nil)
		w = httptest.NewRecorder()
		h.HandleUpdateCheck(w, req)
		if w.Code != http.StatusNoContent {
			t.Errorf("Expected 204 No Content when already on latest version, got %d", w.Code)
		}
	})
}
