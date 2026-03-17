package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/maintenance"
)

func newMaintenanceCommand(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "maintenance",
		Short: "Inspect and manage maintenance windows",
	}
	cmd.AddCommand(
		newMaintenanceStatusCommand(configPath),
		newMaintenanceListCommand(configPath),
	)
	return cmd
}

// newMaintenanceStatusCommand returns `syncctl maintenance status`.
// It reports whether a maintenance window is currently active and, if so,
// when sync will next be allowed.
func newMaintenanceStatusCommand(configPath *string) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current maintenance window status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			now := time.Now().UTC()
			mw, desc := maintenance.ActiveWindow(cfg.Daemon.MaintenanceWindows, now)

			active := mw != nil
			nextAllowed := now
			if active {
				nextAllowed = maintenance.NextAllowed(cfg.Daemon.MaintenanceWindows, now)
			}

			switch strings.ToLower(strings.TrimSpace(format)) {
			case "", "kv":
				if active {
					cmd.Printf("status: active\n")
					cmd.Printf("window: %s\n", desc)
					cmd.Printf("next_allowed: %s\n", nextAllowed.Format(time.RFC3339))
				} else {
					cmd.Printf("status: inactive\n")
					cmd.Printf("windows_configured: %d\n", len(cfg.Daemon.MaintenanceWindows))
				}
			case "json":
				out := map[string]any{
					"active":             active,
					"window":             desc,
					"next_allowed":       nextAllowed.Format(time.RFC3339),
					"windows_configured": len(cfg.Daemon.MaintenanceWindows),
				}
				data, err := json.MarshalIndent(out, "", "  ")
				if err != nil {
					return err
				}
				cmd.Println(string(data))
			default:
				return fmt.Errorf("invalid format %q (use kv or json)", format)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "kv", "Output format: kv, json")
	return cmd
}

// newMaintenanceListCommand returns `syncctl maintenance list`.
// It prints all configured maintenance windows.
func newMaintenanceListCommand(configPath *string) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all configured maintenance windows",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			windows := cfg.Daemon.MaintenanceWindows

			switch strings.ToLower(strings.TrimSpace(format)) {
			case "", "table":
				if len(windows) == 0 {
					cmd.Println("no maintenance windows configured")
					return nil
				}
				cmd.Printf("%-20s  %-30s  %s-%s\n", "NAME", "DAYS", "START", "END")
				for _, w := range windows {
					name := w.Name
					if name == "" {
						name = "(unnamed)"
					}
					cmd.Printf("%-20s  %-30s  %s-%s\n",
						name,
						strings.Join(w.Days, ","),
						w.Start,
						w.End,
					)
				}
			case "json":
				type row struct {
					Name  string   `json:"name"`
					Days  []string `json:"days"`
					Start string   `json:"start"`
					End   string   `json:"end"`
				}
				rows := make([]row, 0, len(windows))
				for _, w := range windows {
					rows = append(rows, row{Name: w.Name, Days: w.Days, Start: w.Start, End: w.End})
				}
				data, err := json.MarshalIndent(rows, "", "  ")
				if err != nil {
					return err
				}
				cmd.Println(string(data))
			default:
				return fmt.Errorf("invalid format %q (use table or json)", format)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "table", "Output format: table, json")
	return cmd
}
