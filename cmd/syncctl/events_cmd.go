package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

func newEventsCommand(configPath *string) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Query event history",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List recent events",
		RunE: func(cmd *cobra.Command, _ []string) error {
			api, closer, err := loadServiceAPI(*configPath)
			if err != nil {
				return err
			}
			defer closer()

			events, err := api.ListEvents(limit)
			if err != nil {
				return err
			}

			if len(events) == 0 {
				cmd.Println("no events found")
				return nil
			}

			printEvents(cmd, events)
			return nil
		},
	}
	listCmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of events")

	cmd.AddCommand(listCmd)
	return cmd
}

func loadServiceAPI(configPath string) (*daemon.ServiceAPI, func(), error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, err
	}

	store, err := state.NewSQLiteStore(cfg.State.DBPath)
	if err != nil {
		return nil, nil, err
	}

	return daemon.NewServiceAPI(store), func() { _ = store.Close() }, nil
}

func printEvents(cmd *cobra.Command, events []telemetry.Event) {
	for _, event := range events {
		cmd.Printf("%s\t%s\t%s\t%s\t%s\t%s\n",
			formatTime(event.CreatedAt),
			event.TraceID,
			event.Level,
			event.ReasonCode,
			event.RepoPath,
			event.Message,
		)
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}

func requireNonEmpty(value, name string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}
