package main

import (
	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func newStatsCommand(configPath *string) *cobra.Command {
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "View runtime stats",
	}

	statsCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show runtime summary",
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

			repoStates, err := api.RepoStatuses(1000)
			if err != nil {
				return err
			}
			runs, err := api.InFlightRuns(1000)
			if err != nil {
				return err
			}
			events, err := api.ListEvents(1000)
			if err != nil {
				return err
			}

			enabledRepos := 0
			for _, repo := range cfg.Repos {
				if repo.Enabled {
					enabledRepos++
				}
			}

			errors := 0
			warns := 0
			for _, event := range events {
				switch event.Level {
				case "error":
					errors++
				case "warn", "warning":
					warns++
				}
			}

			cmd.Printf("repos_configured: %d\n", len(cfg.Repos))
			cmd.Printf("repos_enabled: %d\n", enabledRepos)
			cmd.Printf("repo_states: %d\n", len(repoStates))
			cmd.Printf("in_flight_runs: %d\n", len(runs))
			cmd.Printf("events_total: %d\n", len(events))
			cmd.Printf("events_warn: %d\n", warns)
			cmd.Printf("events_error: %d\n", errors)
			return nil
		},
	})

	return statsCmd
}
