package commands

import (
	"fmt"
	"strconv"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func GetConfigValue(cfg config.Config, key string) (string, error) {
	switch key {
	case "workspace.root":
		return cfg.Workspace.Root, nil
	case "state.db_path":
		return cfg.State.DBPath, nil
	case "daemon.interval_seconds":
		return strconv.Itoa(cfg.Daemon.IntervalSeconds), nil
	case "cache.provider_ttl_seconds":
		return strconv.Itoa(cfg.Cache.ProviderTTLSeconds), nil
	case "cache.branch_ttl_seconds":
		return strconv.Itoa(cfg.Cache.BranchTTLSeconds), nil
	default:
		return "", fmt.Errorf("unsupported key %q", key)
	}
}

func SetConfigValue(cfg *config.Config, key, value string) error {
	switch key {
	case "workspace.root":
		cfg.Workspace.Root = value
	case "state.db_path":
		cfg.State.DBPath = value
	case "daemon.interval_seconds":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer for %s: %w", key, err)
		}
		cfg.Daemon.IntervalSeconds = v
	case "cache.provider_ttl_seconds":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer for %s: %w", key, err)
		}
		cfg.Cache.ProviderTTLSeconds = v
	case "cache.branch_ttl_seconds":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer for %s: %w", key, err)
		}
		cfg.Cache.BranchTTLSeconds = v
	default:
		return fmt.Errorf("unsupported key %q", key)
	}
	return nil
}
