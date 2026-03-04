package main

import (
	"fmt"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func loadSourceByID(configPath, sourceID string) (config.Config, config.SourceConfig, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return config.Config{}, config.SourceConfig{}, err
	}

	for _, source := range cfg.Sources {
		if source.ID == sourceID {
			return cfg, source, nil
		}
	}

	return config.Config{}, config.SourceConfig{}, fmt.Errorf("source %q not found", sourceID)
}

func sourceMap(sources []config.SourceConfig) map[string]config.SourceConfig {
	byID := make(map[string]config.SourceConfig, len(sources))
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		byID[source.ID] = source
	}
	return byID
}
