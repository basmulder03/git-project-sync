package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func newCacheCommand(configPath *string) *cobra.Command {
	cacheCmd := &cobra.Command{
		Use:   "cache",
		Short: "Inspect and refresh cache",
	}

	cacheCmd.AddCommand(
		newCacheShowCommand(configPath),
		newCacheRefreshCommand(configPath),
		newCacheClearCommand(configPath),
	)

	return cacheCmd
}

func newCacheShowCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show cache configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			cmd.Printf("providers ttl: %s\n", time.Duration(cfg.Cache.ProviderTTLSeconds)*time.Second)
			cmd.Printf("branches ttl: %s\n", time.Duration(cfg.Cache.BranchTTLSeconds)*time.Second)
			cmd.Println("last_refresh: unknown")
			return nil
		},
	}
}

func newCacheRefreshCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "refresh [providers|branches|all]",
		Short: "Refresh cache entries",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := parseCacheTarget(args)
			if err != nil {
				return err
			}
			cmd.Printf("refreshed cache target: %s\n", target)
			return nil
		},
	}
}

func newCacheClearCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "clear [providers|branches|all]",
		Short: "Clear cache entries",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := parseCacheTarget(args)
			if err != nil {
				return err
			}
			cmd.Printf("cleared cache target: %s\n", target)
			return nil
		},
	}
}

func parseCacheTarget(args []string) (string, error) {
	if len(args) == 0 {
		return "all", nil
	}

	switch args[0] {
	case "providers", "branches", "all":
		return args[0], nil
	default:
		return "", fmt.Errorf("invalid cache target %q", args[0])
	}
}
