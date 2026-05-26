package api

import (
	"encoding/json"
	"net/http"
	"time"
	"update-api/internal/auth"
	"update-api/internal/repository"

	"github.com/Masterminds/semver/v3"
)

type Handler struct {
	repo *repository.SQLiteRepository
}

func NewHandler(repo *repository.SQLiteRepository) *Handler {
	return &Handler{repo: repo}
}

// Admin handlers

func (h *Handler) HandlePostPlugin(w http.ResponseWriter, r *http.Request) {
	var p repository.Plugin
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if p.Slug == "" || p.Name == "" || p.Secret == "" {
		http.Error(w, "slug, name, and secret are required", http.StatusBadRequest)
		return
	}

	if err := h.repo.SavePlugin(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) HandlePostVersion(w http.ResponseWriter, r *http.Request) {
	var v repository.Version
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if v.PluginSlug == "" || v.Version == "" || v.DownloadURL == "" {
		http.Error(w, "slug, version, and download_url are required", http.StatusBadRequest)
		return
	}

	// Validate version format
	if _, err := semver.NewVersion(v.Version); err != nil {
		http.Error(w, "invalid version format (must be SemVer)", http.StatusBadRequest)
		return
	}

	if err := h.repo.SaveVersion(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) HandleGenerateLicense(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug      string `json:"slug"`
		ClientID  string `json:"client_id"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Slug == "" || req.ClientID == "" || req.ExpiresAt <= time.Now().Unix() {
		http.Error(w, "slug, client_id, and a future expires_at are required", http.StatusBadRequest)
		return
	}

	p, err := h.repo.GetPlugin(req.Slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.Error(w, "plugin not found", http.StatusNotFound)
		return
	}

	key, err := auth.GenerateLicenseKey(p.Slug, req.ClientID, time.Unix(req.ExpiresAt, 0), p.Secret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"license_key": key})
}

// Public handlers

func (h *Handler) HandleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	slug := r.URL.Query().Get("slug")
	currentVersionStr := r.URL.Query().Get("version")
	licenseKey := r.URL.Query().Get("license_key")

	if slug == "" || currentVersionStr == "" {
		http.Error(w, "slug and version are required", http.StatusBadRequest)
		return
	}

	p, err := h.repo.GetPlugin(slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.Error(w, "plugin not found", http.StatusNotFound)
		return
	}

	if p.IsPaid {
		if licenseKey == "" {
			http.Error(w, "license key required", http.StatusForbidden)
			return
		}
		_, err := auth.ValidateLicenseKey(licenseKey, slug, p.Secret)
		if err != nil {
			http.Error(w, "invalid license: "+err.Error(), http.StatusForbidden)
			return
		}
	}

	latest, err := h.repo.GetLatestVersion(slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if latest == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	currV, err := semver.NewVersion(currentVersionStr)
	if err != nil {
		http.Error(w, "invalid current version format", http.StatusBadRequest)
		return
	}

	latestV, err := semver.NewVersion(latest.Version)
	if err != nil {
		http.Error(w, "invalid latest version format in database", http.StatusInternalServerError)
		return
	}

	if latestV.GreaterThan(currV) {
		resp := map[string]interface{}{
			"slug":         latest.PluginSlug,
			"new_version":  latest.Version,
			"url":          "", // Can be a link to a plugin info page if we add one
			"package":      latest.DownloadURL,
			"tested":       latest.TestedWP,
			"requires":     latest.RequiresWP,
			"requires_php": latest.RequiresPHP,
			"sections": map[string]string{
				"changelog": latest.Changelog,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}
