package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// DefaultDataDir returns the platform-specific default directory for application data
// Windows: %LOCALAPPDATA%\git-project-sync
// Linux/macOS: ~/.local/share/git-project-sync
func DefaultDataDir() string {
	if runtime.GOOS == "windows" {
		// Use LOCALAPPDATA for machine-specific data (not roaming)
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "git-project-sync")
		}
		if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
			return filepath.Join(userProfile, "AppData", "Local", "git-project-sync")
		}
	}

	// XDG Base Directory Specification for Linux
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "git-project-sync")
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "git-project-sync")
	}

	if runtime.GOOS == "darwin" {
		// macOS: ~/Library/Application Support
		return filepath.Join(home, "Library", "Application Support", "git-project-sync")
	}

	// Linux default: ~/.local/share
	return filepath.Join(home, ".local", "share", "git-project-sync")
}

// DefaultDBPath returns the default absolute path for the state database
func DefaultDBPath() string {
	return filepath.Join(DefaultDataDir(), "state", "sync.db")
}
