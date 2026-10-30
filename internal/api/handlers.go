package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"update-api/internal/auth"
	"update-api/internal/repository"

	"github.com/Masterminds/semver/v3"
)

type Handler struct {
	repo         *repository.SQLiteRepository
	rateLimitMu  sync.Mutex
	rateLimitMap map[string][]time.Time
	lastSweep    time.Time
}

func NewHandler(repo *repository.SQLiteRepository) *Handler {
	return &Handler{
		repo:         repo,
		rateLimitMap: make(map[string][]time.Time),
	}
}

// Admin handlers

func (h *Handler) HandlePostPlugin(w http.ResponseWriter, r *http.Request) {
	var p repository.Plugin
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if p.Slug == "" || p.Name == "" {
		http.Error(w, "slug and name are required", http.StatusBadRequest)
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

	if p.Secret == "" {
		http.Error(w, "plugin does not have a secret configured, license generation disabled", http.StatusForbidden)
		return
	}

	key, err := auth.GenerateLicenseKey(p.Slug, req.ClientID, time.Unix(req.ExpiresAt, 0), p.Secret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log license creation
	auth.LogLicenseCreated(key, req.ClientID, time.Unix(req.ExpiresAt, 0), auth.AnonymizeIP(clientIP(r)))

	json.NewEncoder(w).Encode(map[string]string{"license_key": key})
}

// Public handlers

// allow records a license validation attempt from ip and reports whether it
// is within 6 per minute.
func (h *Handler) allow(ip string) bool {
	h.rateLimitMu.Lock()
	defer h.rateLimitMu.Unlock()

	now := time.Now()
	minuteAgo := now.Add(-time.Minute)

	// Drop IPs that have gone quiet so the map doesn't grow forever.
	if now.Sub(h.lastSweep) > time.Minute {
		for k, times := range h.rateLimitMap {
			if len(times) == 0 || !times[len(times)-1].After(minuteAgo) {
				delete(h.rateLimitMap, k)
			}
		}
		h.lastSweep = now
	}

	var recent []time.Time
	for _, t := range h.rateLimitMap[ip] {
		if t.After(minuteAgo) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= 6 {
		h.rateLimitMap[ip] = recent
		return false
	}

	h.rateLimitMap[ip] = append(recent, now)
	return true
}

func (h *Handler) HandleValidateLicense(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !h.allow(ip) {
		http.Error(w, "rate limit exceeded (6 requests per minute)", http.StatusTooManyRequests)
		return
	}

	var req struct {
		Slug       string `json:"slug"`
		LicenseKey string `json:"license_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Slug == "" || req.LicenseKey == "" {
		http.Error(w, "slug and license_key are required", http.StatusBadRequest)
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

	valid := true
	if p.Secret == "" {
		valid = false
	} else {
		_, err := auth.ValidateLicenseKey(req.LicenseKey, req.Slug, p.Secret)
		if err != nil {
			valid = false
		}
	}

	if valid {
		auth.LogLicenseValidated(req.LicenseKey, auth.AnonymizeIP(ip))
	}

	json.NewEncoder(w).Encode(map[string]bool{"valid": valid})
}

type UpdateResponse struct {
	Slug        string            `json:"slug"`
	NewVersion  string            `json:"new_version"`
	URL         string            `json:"url"`
	Package     string            `json:"package"`
	Tested      string            `json:"tested"`
	Requires    string            `json:"requires"`
	RequiresPHP string            `json:"requires_php"`
	Sections    map[string]string `json:"sections"`
}

type GroupedUpdateResponse struct {
	Patch    *UpdateResponse  `json:"patch"`
	Minor    *UpdateResponse  `json:"minor"`
	Major    *UpdateResponse  `json:"major"`
	Versions []UpdateResponse `json:"versions"`
}

func formatUpdateResponse(v repository.Version) UpdateResponse {
	return UpdateResponse{
		Slug:        v.PluginSlug,
		NewVersion:  v.Version,
		URL:         "", // Can be a link to a plugin info page if added
		Package:     v.DownloadURL,
		Tested:      v.TestedWP,
		Requires:    v.RequiresWP,
		RequiresPHP: v.RequiresPHP,
		Sections: map[string]string{
			"changelog": v.Changelog,
		},
	}
}

func (h *Handler) HandleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	slug := r.URL.Query().Get("slug")
	currentVersionStr := r.URL.Query().Get("version")
	licenseKey := r.URL.Query().Get("license_key")
	channel := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("channel")))
	allParam := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("all")))

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

	currV, err := semver.NewVersion(currentVersionStr)
	if err != nil {
		http.Error(w, "invalid current version format", http.StatusBadRequest)
		return
	}

	versions, err := h.repo.ListVersions(slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type parsedVersion struct {
		ver    repository.Version
		semver *semver.Version
	}

	var newer []parsedVersion
	for _, v := range versions {
		sv, err := semver.NewVersion(v.Version)
		if err != nil {
			continue
		}
		if sv.GreaterThan(currV) {
			newer = append(newer, parsedVersion{ver: v, semver: sv})
		}
	}

	if len(newer) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Sort newer versions descending by semver
	sort.Slice(newer, func(i, j int) bool {
		return newer[i].semver.GreaterThan(newer[j].semver)
	})

	// highest returns the newest version matching match, or nil.
	highest := func(match func(sv *semver.Version) bool) *UpdateResponse {
		for _, item := range newer {
			if match(item.semver) {
				resp := formatUpdateResponse(item.ver)
				return &resp
			}
		}
		return nil
	}
	samePatchLine := func(sv *semver.Version) bool {
		return sv.Major() == currV.Major() && sv.Minor() == currV.Minor()
	}
	sameMajor := func(sv *semver.Version) bool { return sv.Major() == currV.Major() }

	w.Header().Set("Content-Type", "application/json")

	if allParam == "true" || allParam == "1" || channel == "all" {
		resp := GroupedUpdateResponse{
			Patch: highest(samePatchLine),
			Minor: highest(func(sv *semver.Version) bool { return sameMajor(sv) && sv.Minor() > currV.Minor() }),
			Major: highest(func(sv *semver.Version) bool { return sv.Major() > currV.Major() }),
		}
		for _, item := range newer {
			resp.Versions = append(resp.Versions, formatUpdateResponse(item.ver))
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Single version response based on channel
	var target *UpdateResponse
	switch channel {
	case "patch":
		target = highest(samePatchLine)
	case "minor":
		target = highest(sameMajor)
	default: // "major" or unspecified: highest version overall
		target = highest(func(*semver.Version) bool { return true })
	}

	if target == nil {
		w.Header().Del("Content-Type")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	json.NewEncoder(w).Encode(target)
}
