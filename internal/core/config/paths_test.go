package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultDataDir(t *testing.T) {
	dataDir := DefaultDataDir()

	if dataDir == "" {
		t.Fatal("DefaultDataDir returned empty string")
	}

	if !filepath.IsAbs(dataDir) {
		t.Errorf("DefaultDataDir should return absolute path, got: %s", dataDir)
	}

	// Should end with git-project-sync
	if filepath.Base(dataDir) != "git-project-sync" {
		t.Errorf("DefaultDataDir should end with 'git-project-sync', got: %s", dataDir)
	}

	// Platform-specific checks
	if runtime.GOOS == "windows" {
		// Should be in LocalAppData or UserProfile
		if !containsPath(dataDir, "AppData\\Local") && !containsPath(dataDir, "AppData/Local") {
			t.Errorf("Windows DefaultDataDir should be in LocalAppData, got: %s", dataDir)
		}
	} else if runtime.GOOS == "darwin" {
		// Should be in Library/Application Support
		if !containsPath(dataDir, "Library/Application Support") {
			t.Errorf("macOS DefaultDataDir should be in Library/Application Support, got: %s", dataDir)
		}
	} else {
		// Linux: should be in .local/share
		if !containsPath(dataDir, ".local/share") && !containsPath(dataDir, os.Getenv("XDG_DATA_HOME")) {
			t.Errorf("Linux DefaultDataDir should be in .local/share or XDG_DATA_HOME, got: %s", dataDir)
		}
	}
}

func TestDefaultDBPath(t *testing.T) {
	dbPath := DefaultDBPath()

	if dbPath == "" {
		t.Fatal("DefaultDBPath returned empty string")
	}

	if !filepath.IsAbs(dbPath) {
		t.Errorf("DefaultDBPath should return absolute path, got: %s", dbPath)
	}

	// Should end with state/sync.db
	dir := filepath.Dir(dbPath)
	base := filepath.Base(dbPath)

	if base != "sync.db" {
		t.Errorf("DefaultDBPath should end with 'sync.db', got: %s", base)
	}

	if filepath.Base(dir) != "state" {
		t.Errorf("DefaultDBPath should be in 'state' directory, got: %s", dir)
	}

	// Should contain git-project-sync in the path
	if !containsPath(dbPath, "git-project-sync") {
		t.Errorf("DefaultDBPath should contain 'git-project-sync', got: %s", dbPath)
	}
}

func TestDefaultConfig_UsesAbsoluteDBPath(t *testing.T) {
	cfg := Default()

	if cfg.State.DBPath == "" {
		t.Fatal("Default config has empty DBPath")
	}

	if !filepath.IsAbs(cfg.State.DBPath) {
		t.Errorf("Default config DBPath should be absolute, got: %s", cfg.State.DBPath)
	}

	// Should not be relative paths like "state/sync.db"
	if cfg.State.DBPath == "state/sync.db" {
		t.Error("Default config DBPath should not be relative path 'state/sync.db'")
	}
}

func containsPath(fullPath, substring string) bool {
	// Normalize separators for comparison
	fullPath = filepath.ToSlash(fullPath)
	substring = filepath.ToSlash(substring)

	// Check if substring is in the path (case-insensitive on Windows)
	if runtime.GOOS == "windows" {
		fullPath = filepath.ToSlash(filepath.Clean(fullPath))
		substring = filepath.ToSlash(filepath.Clean(substring))
		return containsCaseInsensitive(fullPath, substring)
	}

	return containsSubstring(fullPath, substring)
}

func containsSubstring(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func containsCaseInsensitive(s, substr string) bool {
	sLower := toLower(s)
	substrLower := toLower(substr)
	return containsSubstring(sLower, substrLower)
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + ('a' - 'A')
		}
		b[i] = c
	}
	return string(b)
}
