package main

import (
	"log"
	"net/http"
	"strconv"
	"time"
	"update-api/internal/api"
	"update-api/internal/config"
	"update-api/internal/repository"
)

func main() {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Printf("Warning: config.yaml not found, using config.yaml.dist")
		cfg, err = config.LoadConfig("config.yaml.dist")
		if err != nil {
			log.Fatalf("Fatal: could not load config: %v", err)
		}
	}

	repo, err := repository.NewSQLiteRepository(cfg.Server.DBPath)
	if err != nil {
		log.Fatalf("Fatal: could not initialize database: %v", err)
	}
	defer repo.Close()

	if err := repo.Migrate("migrations/001_initial_schema.sql"); err != nil {
		log.Fatalf("Fatal: could not run migrations: %v", err)
	}

	h := api.NewHandler(repo)
	adminAuth := api.AdminAuthMiddleware(cfg.Auth.AdminAPIKeys)

	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("/v1/update-check", h.HandleUpdateCheck)
	mux.HandleFunc("/v1/licenses/validate", h.HandleValidateLicense)

	// Admin routes
	mux.Handle("/v1/plugins", adminAuth(http.HandlerFunc(h.HandlePostPlugin)))
	mux.Handle("/v1/versions", adminAuth(http.HandlerFunc(h.HandlePostVersion)))
	mux.Handle("/v1/licenses/generate", adminAuth(http.HandlerFunc(h.HandleGenerateLicense)))

	log.Printf("Server starting on port %d", cfg.Server.Port)
	addr := ":" + strconv.Itoa(cfg.Server.Port)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Fatal: server failed: %v", err)
	}
}
