package main

import (
	"path/filepath"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func TestLoadSourceByID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	cfg := config.Default()
	cfg.Sources = []config.SourceConfig{
		{ID: "gh-personal", Provider: "github", Account: "jane", Enabled: true},
		{ID: "az-team", Provider: "azure", Account: "org", Enabled: false},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, source, err := loadSourceByID(configPath, "gh-personal")
	if err != nil {
		t.Fatalf("loadSourceByID returned error: %v", err)
	}
	if source.ID != "gh-personal" {
		t.Fatalf("unexpected source id: %s", source.ID)
	}
	if len(loaded.Sources) != 2 {
		t.Fatalf("unexpected loaded source count: %d", len(loaded.Sources))
	}

	if _, _, err := loadSourceByID(configPath, "missing"); err == nil {
		t.Fatal("expected source not found error")
	}
}

func TestSourceMapIncludesEnabledOnly(t *testing.T) {
	t.Parallel()

	sources := []config.SourceConfig{
		{ID: "a", Enabled: true},
		{ID: "b", Enabled: false},
		{ID: "c", Enabled: true},
	}

	byID := sourceMap(sources)
	if len(byID) != 2 {
		t.Fatalf("expected 2 enabled sources, got %d", len(byID))
	}
	if _, ok := byID["b"]; ok {
		t.Fatal("disabled source should not be present")
	}
}
