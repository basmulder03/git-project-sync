package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

func newEventsCommand(configPath *string) *cobra.Command {
	var limit int
	var format string
	var sourceID string
	var repoPath string
	var since string
	var until string

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Query event history",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List recent events",
		RunE: func(cmd *cobra.Command, _ []string) error {
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

			events, err := api.ListEvents(limit)
			if err != nil {
				return err
			}

			if len(events) == 0 {
				cmd.Println("no events found")
				return nil
			}

			rows := filterEvents(events, repoPath, sourceID, repoToSource(cfg.Repos), sinceTime, untilTime)
			if len(rows) == 0 {
				cmd.Println("no events found")
				return nil
			}

			if err := printEvents(cmd, rows, format, repoToSource(cfg.Repos)); err != nil {
				return err
			}
			return nil
		},
	}
	listCmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of events")
	listCmd.Flags().StringVar(&format, "format", "table", "Output format: table, json, csv")
	listCmd.Flags().StringVar(&sourceID, "source-id", "", "Filter by source ID")
	listCmd.Flags().StringVar(&repoPath, "repo-path", "", "Filter by repository path")
	listCmd.Flags().StringVar(&since, "since", "", "Filter events since RFC3339 timestamp")
	listCmd.Flags().StringVar(&until, "until", "", "Filter events until RFC3339 timestamp")

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

func printEvents(cmd *cobra.Command, events []telemetry.Event, format string, sourceByRepo map[string]string) error {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "table", "":
		for _, event := range events {
			cmd.Printf("%s\t%s\t%s\t%s\t%s\t%s\n",
				formatTime(event.CreatedAt),
				event.TraceID,
				event.Level,
				event.ReasonCode,
				event.RepoPath,
				redactExportMessage(event.Message),
			)
		}
		return nil
	case "json":
		rows := make([]map[string]string, 0, len(events))
		for _, event := range events {
			rows = append(rows, map[string]string{
				"time":        formatTime(event.CreatedAt),
				"trace_id":    event.TraceID,
				"level":       event.Level,
				"reason_code": event.ReasonCode,
				"repo_path":   event.RepoPath,
				"source_id":   sourceByRepo[event.RepoPath],
				"message":     redactExportMessage(event.Message),
			})
		}
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return err
		}
		cmd.Println(string(data))
		return nil
	case "csv":
		w := csv.NewWriter(cmd.OutOrStdout())
		if err := w.Write([]string{"time", "trace_id", "level", "reason_code", "repo_path", "source_id", "message"}); err != nil {
			return err
		}
		for _, event := range events {
			if err := w.Write([]string{formatTime(event.CreatedAt), event.TraceID, event.Level, event.ReasonCode, event.RepoPath, sourceByRepo[event.RepoPath], redactExportMessage(event.Message)}); err != nil {
				return err
			}
		}
		w.Flush()
		return w.Error()
	default:
		return fmt.Errorf("invalid output format %q", format)
	}
}

func repoToSource(repos []config.RepoConfig) map[string]string {
	out := make(map[string]string, len(repos))
	for _, repo := range repos {
		out[repo.Path] = repo.SourceID
	}
	return out
}

func filterEvents(events []telemetry.Event, repoPath, sourceID string, sourceByRepo map[string]string, sinceTime, untilTime *time.Time) []telemetry.Event {
	repoPath = strings.TrimSpace(repoPath)
	sourceID = strings.TrimSpace(sourceID)
	out := make([]telemetry.Event, 0, len(events))
	for _, event := range events {
		if repoPath != "" && event.RepoPath != repoPath {
			continue
		}
		if sourceID != "" && sourceByRepo[event.RepoPath] != sourceID {
			continue
		}
		if sinceTime != nil && event.CreatedAt.Before(*sinceTime) {
			continue
		}
		if untilTime != nil && event.CreatedAt.After(*untilTime) {
			continue
		}
		out = append(out, event)
	}
	return out
}

func parseTimeWindow(sinceRaw, untilRaw string) (*time.Time, *time.Time, error) {
	var sinceTime *time.Time
	var untilTime *time.Time
	if strings.TrimSpace(sinceRaw) != "" {
		t, err := time.Parse(time.RFC3339, sinceRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid --since value: %w", err)
		}
		sinceTime = &t
	}
	if strings.TrimSpace(untilRaw) != "" {
		t, err := time.Parse(time.RFC3339, untilRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid --until value: %w", err)
		}
		untilTime = &t
	}
	if sinceTime != nil && untilTime != nil && sinceTime.After(*untilTime) {
		return nil, nil, fmt.Errorf("invalid time window: --since must be <= --until")
	}
	return sinceTime, untilTime, nil
}

func redactExportMessage(message string) string {
	replacer := strings.NewReplacer(
		"token=", "token=[redacted]",
		"pat=", "pat=[redacted]",
		"password=", "password=[redacted]",
		"secret=", "secret=[redacted]",
	)
	cleaned := replacer.Replace(message)
	if strings.Contains(cleaned, "ghp_") {
		parts := strings.Split(cleaned, " ")
		for i, part := range parts {
			if strings.HasPrefix(part, "ghp_") && len(part) > 8 {
				parts[i] = "ghp_[redacted]"
			}
		}
		cleaned = strings.Join(parts, " ")
	}
	if strings.Contains(cleaned, "Bearer ") {
		idx := strings.Index(cleaned, "Bearer ")
		tail := cleaned[idx+len("Bearer "):]
		space := strings.IndexAny(tail, " \t\n")
		if space >= 0 {
			cleaned = cleaned[:idx+len("Bearer ")] + "[redacted]" + tail[space:]
		} else {
			cleaned = cleaned[:idx+len("Bearer ")] + "[redacted]"
		}
	}
	return cleaned
}

func intToString(v int) string {
	return strconv.Itoa(v)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}

func requireNonEmpty(value, name string) error {
	if value == "" {
		return fmt.Errorf("required argument: %s", name)
	}
	return nil
}
