package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func newStatsCommand(configPath *string) *cobra.Command {
	var format string
	var sourceID string
	var repoPath string
	var since string
	var until string

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

			sinceTime, untilTime, err := parseTimeWindow(since, until)
			if err != nil {
				return err
			}
			sourceByRepo := repoToSource(cfg.Repos)
			events = filterEvents(events, repoPath, sourceID, sourceByRepo, sinceTime, untilTime)

			enabledRepos := 0
			for _, repo := range cfg.Repos {
				if repo.Enabled {
					if repoPath != "" && repo.Path != repoPath {
						continue
					}
					if sourceID != "" && repo.SourceID != sourceID {
						continue
					}
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

			summary := map[string]int{
				"repos_configured": len(cfg.Repos),
				"repos_enabled":    enabledRepos,
				"repo_states":      len(repoStates),
				"in_flight_runs":   len(runs),
				"events_total":     len(events),
				"events_warn":      warns,
				"events_error":     errors,
			}

			switch strings.ToLower(strings.TrimSpace(format)) {
			case "", "table", "kv":
				cmd.Printf("repos_configured: %d\n", summary["repos_configured"])
				cmd.Printf("repos_enabled: %d\n", summary["repos_enabled"])
				cmd.Printf("repo_states: %d\n", summary["repo_states"])
				cmd.Printf("in_flight_runs: %d\n", summary["in_flight_runs"])
				cmd.Printf("events_total: %d\n", summary["events_total"])
				cmd.Printf("events_warn: %d\n", summary["events_warn"])
				cmd.Printf("events_error: %d\n", summary["events_error"])
				return nil
			case "json":
				payload := map[string]any{
					"summary": summary,
					"filters": map[string]string{"source_id": sourceID, "repo_path": repoPath, "since": since, "until": until},
				}
				data, err := json.MarshalIndent(payload, "", "  ")
				if err != nil {
					return err
				}
				cmd.Println(string(data))
				return nil
			case "csv":
				w := csv.NewWriter(cmd.OutOrStdout())
				if err := w.Write([]string{"metric", "value"}); err != nil {
					return err
				}
				for _, key := range []string{"repos_configured", "repos_enabled", "repo_states", "in_flight_runs", "events_total", "events_warn", "events_error"} {
					if err := w.Write([]string{key, intToString(summary[key])}); err != nil {
						return err
					}
				}
				w.Flush()
				return w.Error()
			default:
				return fmt.Errorf("invalid output format %q", format)
			}
		},
	})

	showCmd := statsCmd.Commands()[0]
	showCmd.Flags().StringVar(&format, "format", "kv", "Output format: kv, json, csv")
	showCmd.Flags().StringVar(&sourceID, "source-id", "", "Filter by source ID")
	showCmd.Flags().StringVar(&repoPath, "repo-path", "", "Filter by repository path")
	showCmd.Flags().StringVar(&since, "since", "", "Filter events since RFC3339 timestamp")
	showCmd.Flags().StringVar(&until, "until", "", "Filter events until RFC3339 timestamp")

	return statsCmd
}
