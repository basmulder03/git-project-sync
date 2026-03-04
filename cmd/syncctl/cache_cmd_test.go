package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func TestCacheShowRefreshClear(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(configPath, config.Default()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	showOut, err := executeSyncctl("--config", configPath, "cache", "show")
	if err != nil {
		t.Fatalf("cache show failed: %v output=%s", err, showOut)
	}
	if !strings.Contains(showOut, "providers ttl") {
		t.Fatalf("unexpected cache show output: %s", showOut)
	}

	refreshOut, err := executeSyncctl("--config", configPath, "cache", "refresh", "all")
	if err != nil {
		t.Fatalf("cache refresh failed: %v output=%s", err, refreshOut)
	}
	if !strings.Contains(refreshOut, "refreshed cache target") {
		t.Fatalf("unexpected cache refresh output: %s", refreshOut)
	}

	clearOut, err := executeSyncctl("--config", configPath, "cache", "clear", "providers")
	if err != nil {
		t.Fatalf("cache clear failed: %v output=%s", err, clearOut)
	}
	if !strings.Contains(clearOut, "cleared cache target") {
		t.Fatalf("unexpected cache clear output: %s", clearOut)
	}
}
