package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"update-api/internal/repository"
)

func TestHandlers(t *testing.T) {
	dbPath := "test_api.db"
	defer os.Remove(dbPath)

	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("Failed to open repo: %v", err)
	}
	defer repo.Close()

	err = repo.Migrate("../../migrations/001_initial_schema.sql")
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
}
