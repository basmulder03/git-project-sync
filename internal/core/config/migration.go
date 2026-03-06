package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// migrateStateDatabaseToAppData checks if the state database exists in the old location
// (relative path like "state/sync.db" resolved from workspace root or cwd)
// and migrates it to the new location in user data directory.
//
// This migration handles the transition from relative paths to absolute paths
// in the AppData/LocalAppData directory structure.
func migrateStateDatabaseToAppData(cfg *Config) error {
	newDBPath := cfg.State.DBPath

	// If the configured path is already absolute and exists, no migration needed
	if filepath.IsAbs(newDBPath) {
		if _, err := os.Stat(newDBPath); err == nil {
			// New location already has a database
			return nil
		}
	}

	// Check common old locations where the database might exist
	oldLocations := findOldDatabaseLocations(cfg)

	for _, oldPath := range oldLocations {
		if _, err := os.Stat(oldPath); err == nil {
			// Found old database, migrate it
			return migrateDatabase(oldPath, newDBPath)
		}
	}

	// No old database found, nothing to migrate
	return nil
}

// findOldDatabaseLocations returns possible locations where the old database
// might exist based on the relative path "state/sync.db"
func findOldDatabaseLocations(cfg *Config) []string {
	var locations []string

	// 1. Relative to current working directory
	if cwd, err := os.Getwd(); err == nil {
		locations = append(locations, filepath.Join(cwd, "state", "sync.db"))
	}

	// 2. Relative to workspace root (if configured)
	if cfg.Workspace.Root != "" {
		locations = append(locations, filepath.Join(cfg.Workspace.Root, "state", "sync.db"))
	}

	// 3. Check if user had customized to a relative path
	if !filepath.IsAbs(cfg.State.DBPath) && cfg.State.DBPath != "" {
		// User might have a custom relative path
		if cwd, err := os.Getwd(); err == nil {
			locations = append(locations, filepath.Join(cwd, cfg.State.DBPath))
		}
	}

	return locations
}

// migrateDatabase moves the database file from old location to new location
func migrateDatabase(oldPath, newPath string) error {
	// Create the directory for the new location
	newDir := filepath.Dir(newPath)
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return fmt.Errorf("create new database directory: %w", err)
	}

	// Check if new location already exists
	if _, err := os.Stat(newPath); err == nil {
		// New location already exists, don't overwrite
		// Just remove the old one after confirmation it's not being used
		return nil
	}

	// Copy the database file (don't move, in case of errors we want backup)
	if err := copyFile(oldPath, newPath); err != nil {
		return fmt.Errorf("copy database from %s to %s: %w", oldPath, newPath, err)
	}

	// Also copy any SQLite WAL/SHM files if they exist
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		oldWalPath := oldPath + suffix
		if _, err := os.Stat(oldWalPath); err == nil {
			newWalPath := newPath + suffix
			if err := copyFile(oldWalPath, newWalPath); err != nil {
				// Non-fatal, these are temporary files
				continue
			}
		}
	}

	// Successfully migrated, now we can remove the old files
	_ = os.Remove(oldPath)
	_ = os.Remove(oldPath + "-wal")
	_ = os.Remove(oldPath + "-shm")
	_ = os.Remove(oldPath + "-journal")

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := destFile.ReadFrom(sourceFile); err != nil {
		return err
	}

	// Sync to ensure data is written
	return destFile.Sync()
}
