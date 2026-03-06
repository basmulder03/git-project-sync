package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/install"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

type daemonController interface {
	Start(mode install.Mode) error
	Stop(mode install.Mode) error
	Restart(mode install.Mode) error
	Status(mode install.Mode) (string, error)
}

type osDaemonController struct{}

var daemonOps daemonController = osDaemonController{}

func newDaemonCommand(configPath *string) *cobra.Command {
	var userFlag bool
	var systemFlag bool

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Control the daemon",
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start daemon service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode, err := resolveInstallMode(userFlag, systemFlag)
			if err != nil {
				return err
			}
			if err := daemonOps.Start(mode); err != nil {
				return err
			}
			cmd.Printf("daemon start requested (%s)\n", mode)
			return nil
		},
	}

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop daemon service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode, err := resolveInstallMode(userFlag, systemFlag)
			if err != nil {
				return err
			}
			if err := daemonOps.Stop(mode); err != nil {
				return err
			}
			cmd.Printf("daemon stop requested (%s)\n", mode)
			return nil
		},
	}

	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart daemon service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode, err := resolveInstallMode(userFlag, systemFlag)
			if err != nil {
				return err
			}
			if err := daemonOps.Restart(mode); err != nil {
				return err
			}
			cmd.Printf("daemon restart requested (%s)\n", mode)
			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show daemon service status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode, err := resolveInstallMode(userFlag, systemFlag)
			if err != nil {
				return err
			}
			status, err := daemonOps.Status(mode)
			if err != nil {
				return err
			}
			cmd.Printf("mode: %s\n", mode)
			cmd.Printf("status: %s\n", status)
			return nil
		},
	}

	var limit int
	var follow bool
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Show daemon event logs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			api, closer, err := loadServiceAPI(*configPath)
			if err != nil {
				return err
			}
			defer closer()

			lastPrintedAt := time.Time{}
			for {
				events, err := api.ListEvents(limit)
				if err != nil {
					return err
				}

				for i := len(events) - 1; i >= 0; i-- {
					event := events[i]
					if !lastPrintedAt.IsZero() && (event.CreatedAt.Before(lastPrintedAt) || event.CreatedAt.Equal(lastPrintedAt)) {
						continue
					}
					cmd.Printf("%s\t%s\t%s\t%s\t%s\n", formatTime(event.CreatedAt), event.Level, event.ReasonCode, event.RepoPath, redactExportMessage(event.Message))
					if event.CreatedAt.After(lastPrintedAt) {
						lastPrintedAt = event.CreatedAt
					}
				}

				if !follow {
					return nil
				}

				select {
				case <-cmd.Context().Done():
					return nil
				case <-time.After(2 * time.Second):
				}
			}
		},
	}
	logsCmd.Flags().IntVar(&limit, "limit", 100, "Maximum number of log events")
	logsCmd.Flags().BoolVar(&follow, "follow", false, "Follow new log events")

	for _, sub := range []*cobra.Command{startCmd, stopCmd, restartCmd, statusCmd} {
		sub.Flags().BoolVar(&userFlag, "user", false, "Use user mode (default)")
		sub.Flags().BoolVar(&systemFlag, "system", false, "Use system mode")
	}

	cmd.AddCommand(startCmd, stopCmd, restartCmd, statusCmd, logsCmd)
	return cmd
}

func (osDaemonController) Start(mode install.Mode) error {
	switch runtime.GOOS {
	case "linux":
		_, err := runCommand("systemctl", append(systemctlModeArgs(mode), "start", "git-project-sync.service")...)
		return err
	case "windows":
		_, err := runCommand("schtasks", "/Run", "/TN", "GitProjectSync")
		return err
	default:
		return fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
}

func (osDaemonController) Stop(mode install.Mode) error {
	switch runtime.GOOS {
	case "linux":
		_, err := runCommand("systemctl", append(systemctlModeArgs(mode), "stop", "git-project-sync.service")...)
		return err
	case "windows":
		_, err := runCommand("schtasks", "/End", "/TN", "GitProjectSync")
		return err
	default:
		return fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
}

func (c osDaemonController) Restart(mode install.Mode) error {
	if err := c.Stop(mode); err != nil {
		return err
	}
	return c.Start(mode)
}

func (osDaemonController) Status(mode install.Mode) (string, error) {
	switch runtime.GOOS {
	case "linux":
		out, err := runCommand("systemctl", append(systemctlModeArgs(mode), "is-active", "git-project-sync.service")...)
		if err != nil {
			return "unknown", err
		}
		return strings.TrimSpace(out), nil
	case "windows":
		out, err := runCommand("schtasks", "/Query", "/TN", "GitProjectSync", "/FO", "LIST")
		if err != nil {
			return "unknown", err
		}
		for _, line := range strings.Split(out, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(strings.ToLower(trimmed), "status:") {
				return strings.TrimSpace(strings.TrimPrefix(trimmed, "Status:")), nil
			}
		}
		return "unknown", nil
	default:
		return "unknown", fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
}

func systemctlModeArgs(mode install.Mode) []string {
	if mode == install.ModeUser {
		return []string{"--user"}
	}
	return []string{}
}

var runCommand = func(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func telemetryEventRows(events []telemetry.Event) []string {
	rows := make([]string, 0, len(events))
	for _, event := range events {
		rows = append(rows, fmt.Sprintf("%s\t%s\t%s\t%s\t%s", formatTime(event.CreatedAt), event.Level, event.ReasonCode, event.RepoPath, redactExportMessage(event.Message)))
	}
	return rows
}
