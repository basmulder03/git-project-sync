package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/app/commands"
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

			api, closer, err := loadServiceAPI(*configPath)
			if err != nil {
				return err
			}
			defer closer()

			events, err := api.ListEvents(1000)
			if err != nil {
				return err
			}
			providerRefresh, providerClear := commands.LatestCacheEvents(events, commands.CacheTargetProviders)
			branchRefresh, branchClear := commands.LatestCacheEvents(events, commands.CacheTargetBranches)

			cmd.Printf("providers ttl: %s\n", time.Duration(cfg.Cache.ProviderTTLSeconds)*time.Second)
			cmd.Printf("providers last_refresh: %s\n", formatTime(providerRefresh))
			cmd.Printf("providers last_clear: %s\n", formatTime(providerClear))
			cmd.Printf("branches ttl: %s\n", time.Duration(cfg.Cache.BranchTTLSeconds)*time.Second)
			cmd.Printf("branches last_refresh: %s\n", formatTime(branchRefresh))
			cmd.Printf("branches last_clear: %s\n", formatTime(branchClear))
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

			api, closer, err := loadServiceAPI(*configPath)
			if err != nil {
				return err
			}
			defer closer()

			svc := commands.NewCacheService(api)
			if err := svc.Refresh(cmd.Context(), commands.CacheTarget(target)); err != nil {
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

			api, closer, err := loadServiceAPI(*configPath)
			if err != nil {
				return err
			}
			defer closer()

			svc := commands.NewCacheService(api)
			if err := svc.Clear(cmd.Context(), commands.CacheTarget(target)); err != nil {
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
