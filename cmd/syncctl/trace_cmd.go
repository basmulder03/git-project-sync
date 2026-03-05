package main

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func newTraceCommand(configPath *string) *cobra.Command {
	var limit int
	var format string
	var sourceID string
	var repoPath string
	var since string
	var until string

	cmd := &cobra.Command{
		Use:   "trace",
		Short: "Query trace details",
	}

	showCmd := &cobra.Command{
		Use:   "show <trace-id>",
		Short: "Show events for one trace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNonEmpty(args[0], "trace-id"); err != nil {
				return err
			}

			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			sinceTime, untilTime, err := parseTimeWindow(since, until)
			if err != nil {
				return err
			}

			api, closer, err := loadServiceAPI(*configPath)
			if err != nil {
				return err
			}
			defer closer()

			events, err := api.Trace(strings.TrimSpace(args[0]), limit)
			if err != nil {
				return err
			}

			if len(events) == 0 {
				cmd.Printf("no events found for trace %s\n", args[0])
				return nil
			}

			rows := filterEvents(events, repoPath, sourceID, repoToSource(cfg.Repos), sinceTime, untilTime)
			if len(rows) == 0 {
				cmd.Printf("no events found for trace %s\n", args[0])
				return nil
			}

			return printEvents(cmd, rows, format, repoToSource(cfg.Repos))
		},
	}

	showCmd.Flags().IntVar(&limit, "limit", 200, "Maximum number of events for this trace")
	showCmd.Flags().StringVar(&format, "format", "table", "Output format: table, json, csv")
	showCmd.Flags().StringVar(&sourceID, "source-id", "", "Filter by source ID")
	showCmd.Flags().StringVar(&repoPath, "repo-path", "", "Filter by repository path")
	showCmd.Flags().StringVar(&since, "since", "", "Filter events since RFC3339 timestamp")
	showCmd.Flags().StringVar(&until, "until", "", "Filter events until RFC3339 timestamp")
	cmd.AddCommand(showCmd)
	return cmd
}
