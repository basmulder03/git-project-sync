package main

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

func newDoctorCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostics",
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

			events, err := api.ListEvents(500)
			if err != nil {
				return err
			}
			runs, err := api.InFlightRuns(200)
			if err != nil {
				return err
			}

			eventSummary := telemetry.SummarizeRecentEvents(events, time.Now().UTC())

			critical := 0
			warning := 0

			missingCreds := 0
			for _, source := range cfg.Sources {
				if source.Enabled && source.CredentialRef == "" {
					missingCreds++
				}
			}
			if missingCreds > 0 {
				critical++
			}

			if len(runs) > 0 {
				warning++
			}
			if eventSummary.ErrorsLastHour > 0 {
				critical++
			}
			if cfg.Cache.ProviderTTLSeconds <= 0 || cfg.Cache.BranchTTLSeconds <= 0 {
				warning++
			}

			score := telemetry.HealthScore(critical, warning)

			cmd.Printf("health_score: %d\n", score)
			cmd.Printf("critical_findings: %d\n", critical)
			cmd.Printf("warning_findings: %d\n", warning)
			cmd.Printf("recent_errors_last_hour: %d\n", eventSummary.ErrorsLastHour)
			cmd.Printf("in_flight_runs: %d\n", len(runs))

			if missingCreds > 0 {
				cmd.Printf("finding: source_auth_missing count=%d\n", missingCreds)
			}
			if len(runs) > 0 {
				cmd.Printf("finding: lock_or_run_contention count=%d\n", len(runs))
			}
			if eventSummary.ErrorsLastHour > 0 {
				cmd.Printf("finding: failed_jobs_last_hour count=%d\n", eventSummary.ErrorsLastHour)
			}

			return nil
		},
	}
}
