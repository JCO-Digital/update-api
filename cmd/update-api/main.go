package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
	"update-api/internal/api"
	"update-api/internal/auth"
	"update-api/internal/backup"
	"update-api/internal/config"
	"update-api/internal/repository"
)

func main() {
	cfg, cfgPath, err := config.Load()
	if err != nil {
		log.Fatalf("Fatal: could not load config: %v", err)
	}
	if cfgPath == "" {
		log.Printf("Warning: no config file found, using defaults; admin endpoints are disabled until admin_api_keys is set")
	} else {
		log.Printf("Using config %s", cfgPath)
	}
	log.Printf("Using database %s", cfg.Server.DBPath)
	for _, msg := range cfg.StrayFiles(cfgPath) {
		log.Printf("Warning: %s", msg)
	}
	auth.LogDir = cfg.Licenses.LogDir

	repo, err := repository.NewSQLiteRepository(cfg.Server.DBPath)
	if err != nil {
		log.Fatalf("Fatal: could not initialize database: %v", err)
	}
	defer repo.Close()

	if err := repo.Migrate(); err != nil {
		log.Fatalf("Fatal: could not run migrations: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Go(func() {
		backup.Run(ctx, repo, cfg.Backup.Dir, cfg.Backup.Interval, cfg.Backup.Keep)
	})

	h := api.NewHandler(repo)
	adminAuth := api.AdminAuthMiddleware(cfg.Auth.AdminAPIKeys)

	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("GET /v1/update-check", h.HandleUpdateCheck)
	mux.HandleFunc("POST /v1/licenses/validate", h.HandleValidateLicense)

	// Admin routes
	mux.Handle("POST /v1/plugins", adminAuth(http.HandlerFunc(h.HandlePostPlugin)))
	mux.Handle("POST /v1/versions", adminAuth(http.HandlerFunc(h.HandlePostVersion)))
	mux.Handle("POST /v1/licenses/generate", adminAuth(http.HandlerFunc(h.HandleGenerateLicense)))

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Server.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	log.Printf("Server starting on port %d", cfg.Server.Port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Fatal: server failed: %v", err)
	}

	// Let an in-flight backup finish before the database closes.
	wg.Wait()
	log.Printf("Server stopped")
}
