package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/install"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

var evaluateInstallPreflight = defaultInstallPreflight

func newDoctorCommand(configPath *string) *cobra.Command {
	installMode := string(install.ModeUser)

	cmd := &cobra.Command{
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

			installFindings := evaluateInstallPreflight(install.Mode(strings.ToLower(strings.TrimSpace(installMode))))
			for _, finding := range installFindings {
				if finding.Severity == "critical" {
					critical++
				} else {
					warning++
				}
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
			for _, finding := range installFindings {
				cmd.Printf("finding: install_preflight reason_code=%s severity=%s\n", finding.Code, finding.Severity)
				cmd.Printf("  detail: %s\n", finding.Message)
				if strings.TrimSpace(finding.Hint) != "" {
					cmd.Printf("  hint: %s\n", finding.Hint)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&installMode, "install-mode", string(install.ModeUser), "Install diagnostics mode: user or system")

	return cmd
}

func defaultInstallPreflight(mode install.Mode) []install.Finding {
	binaryPath, configPath, ok := defaultInstallPaths(mode)
	if !ok {
		return []install.Finding{{
			Severity: "critical",
			Code:     install.ReasonInstallUnsupportedEnvironment,
			Message:  "install diagnostics are unsupported on this operating system",
			Hint:     "run diagnostics on Linux or Windows",
		}}
	}

	switch runtime.GOOS {
	case "linux":
		return install.NewLinuxSystemdInstaller(binaryPath, configPath).Preflight(mode)
	case "windows":
		return install.NewWindowsTaskSchedulerInstaller(binaryPath, configPath).Preflight(mode)
	default:
		return []install.Finding{{
			Severity: "critical",
			Code:     install.ReasonInstallUnsupportedEnvironment,
			Message:  "install diagnostics are unsupported on this operating system",
			Hint:     "run diagnostics on Linux or Windows",
		}}
	}
}

func defaultInstallPaths(mode install.Mode) (string, string, bool) {
	if mode != install.ModeUser && mode != install.ModeSystem {
		return "", "", false
	}
	switch runtime.GOOS {
	case "linux":
		if mode == install.ModeSystem {
			return "/usr/local/bin/syncd", "/etc/git-project-sync/config.yaml", true
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", false
		}
		return filepath.Join(home, ".local", "bin", "syncd"), filepath.Join(home, ".config", "git-project-sync", "config.yaml"), true
	case "windows":
		if mode == install.ModeSystem {
			return filepath.Join(os.Getenv("ProgramFiles"), "git-project-sync", "syncd.exe"), filepath.Join(os.Getenv("ProgramData"), "git-project-sync", "config.yaml"), true
		}
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "git-project-sync", "bin", "syncd.exe"), filepath.Join(os.Getenv("APPDATA"), "git-project-sync", "config.yaml"), true
	default:
		return "", "", false
	}
}
