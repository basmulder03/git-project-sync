package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
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
			providerRefresh, providerClear := latestCacheEvents(events, "providers")
			branchRefresh, branchClear := latestCacheEvents(events, "branches")

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

			if err := recordCacheEvent(cmd.Context(), api, target, "refresh"); err != nil {
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

			if err := recordCacheEvent(cmd.Context(), api, target, "clear"); err != nil {
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

func recordCacheEvent(ctx context.Context, api telemetryRecorder, target, action string) error {
	now := time.Now().UTC()
	traceID := fmt.Sprintf("cache-%d", now.UnixNano())
	targets := []string{target}
	if target == "all" {
		targets = []string{"providers", "branches"}
	}

	for _, t := range targets {
		reasonCode := fmt.Sprintf("cache_%s_%s", action, t)
		if err := api.RecordEvent(ctx, telemetry.Event{
			TraceID:    traceID,
			RepoPath:   "cache",
			Level:      "info",
			ReasonCode: reasonCode,
			Message:    fmt.Sprintf("cache %s executed for %s", action, t),
			CreatedAt:  now,
		}); err != nil {
			return err
		}
	}

	return nil
}

type telemetryRecorder interface {
	RecordEvent(ctx context.Context, event telemetry.Event) error
}

func latestCacheEvents(events []telemetry.Event, target string) (time.Time, time.Time) {
	var refreshed time.Time
	var cleared time.Time
	refreshCode := "cache_refresh_" + target
	clearCode := "cache_clear_" + target
	for _, event := range events {
		reasonCode := strings.TrimSpace(event.ReasonCode)
		switch reasonCode {
		case refreshCode:
			if event.CreatedAt.After(refreshed) {
				refreshed = event.CreatedAt
			}
		case clearCode:
			if event.CreatedAt.After(cleared) {
				cleared = event.CreatedAt
			}
		}
	}
	return refreshed, cleared
}
