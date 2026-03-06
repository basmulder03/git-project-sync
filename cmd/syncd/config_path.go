package main

import (
	"os"
	"path/filepath"
	"runtime"
)

func defaultConfigPath() string {
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "git-project-sync", "config.yaml")
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "git-project-sync", "config.yaml")
		}
		if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
			return filepath.Join(userProfile, "AppData", "Roaming", "git-project-sync", "config.yaml")
		}
	}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "git-project-sync", "config.yaml")
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "git-project-sync", "config.yaml")
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Roaming", "git-project-sync", "config.yaml")
	}
	return filepath.Join(home, ".config", "git-project-sync", "config.yaml")
}
