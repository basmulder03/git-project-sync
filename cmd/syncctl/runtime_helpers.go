package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func loadOrInitConfig(configPath string) (config.Config, error) {
	cfg, err := config.Load(configPath)
	if err == nil {
		return cfg, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return config.Config{}, err
	}

	defaultCfg := config.Default()
	if saveErr := config.Save(configPath, defaultCfg); saveErr != nil {
		return config.Config{}, saveErr
	}
	return defaultCfg, nil
}

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
