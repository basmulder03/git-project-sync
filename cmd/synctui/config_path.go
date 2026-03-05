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
	}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "git-project-sync", "config.yaml")
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "configs/config.example.yaml"
	}
	return filepath.Join(home, ".config", "git-project-sync", "config.yaml")
}
