package backup

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	prefix = "updates-"
	suffix = ".db"
)

// Backuper is implemented by the repository.
type Backuper interface {
	Backup(destPath string) error
}

// Snapshot writes one timestamped backup into dir and returns its path.
func Snapshot(db Backuper, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}

	name := prefix + time.Now().UTC().Format("20060102T150405Z") + suffix
	dest := filepath.Join(dir, name)
	// Write under a temporary name so a crash never leaves a half-written
	// file that looks like a valid backup.
	tmp := dest + ".tmp"
	os.Remove(tmp)
	if err := db.Backup(tmp); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("backup: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// Prune deletes all but the newest keep backups in dir.
func Prune(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), suffix) {
			names = append(names, e.Name())
		}
	}
	// Timestamped names sort chronologically.
	sort.Strings(names)

	for len(names) > keep {
		if err := os.Remove(filepath.Join(dir, names[0])); err != nil {
			return err
		}
		names = names[1:]
	}
	return nil
}

// Run takes a backup immediately and then every interval until ctx is done.
func Run(ctx context.Context, db Backuper, dir string, interval time.Duration, keep int) {
	if interval <= 0 {
		log.Printf("Backups disabled")
		return
	}
	if keep < 1 {
		keep = 1
	}

	run := func() {
		path, err := Snapshot(db, dir)
		if err != nil {
			log.Printf("Error: %v", err)
			return
		}
		if err := Prune(dir, keep); err != nil {
			log.Printf("Error pruning backups: %v", err)
		}
		log.Printf("Backup written to %s", path)
	}

	log.Printf("Backing up to %s every %s, keeping %d", dir, interval, keep)
	run()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
