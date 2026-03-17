package config

import (
	"context"
	"log/slog"
	"strings"

	coressh "github.com/basmulder03/git-project-sync/internal/core/ssh"
)

// migrateRemotesToSSH is a config migration that rewrites git remote origin
// URLs from HTTPS to SSH for all workspace repos whose source has SSH enabled.
//
// The migration is ONLY executed when:
//   - ssh.enabled is true (global or per-source), AND
//   - ssh.migration_opt_in == "accepted"
//
// When migration_opt_in is "" or "declined" this is a no-op.
// The CLI/setup flow is responsible for setting migration_opt_in to "accepted"
// after the user confirms.
func migrateRemotesToSSH(cfg *Config) error {
	if cfg.SSH.MigrationOptIn != "accepted" {
		// Not opted in; skip silently.
		return nil
	}

	if !cfg.SSH.Enabled {
		return nil
	}

	root := strings.TrimSpace(cfg.Workspace.Root)
	if root == "" {
		return nil
	}

	sshDir := cfg.SSHDir()

	// Build migration sources from the configured sources.
	var migrationSources []coressh.MigrationSource
	for _, src := range cfg.Sources {
		if !cfg.SSHEnabledForSource(src) {
			continue
		}
		// Only migrate sources that have a key already set up.
		if !coressh.KeyExists(coressh.PrivateKeyPathForSource(sshDir, src.ID)) {
			continue
		}

		ms := coressh.MigrationSource{
			SourceID: src.ID,
			Provider: src.Provider,
		}

		switch strings.ToLower(src.Provider) {
		case "github":
			host := src.Host
			if host == "" {
				host = "github.com"
			}
			ms.MatchHosts = []string{host}
		case "azuredevops", "azure":
			ms.MatchHosts = []string{"dev.azure.com"}
			ms.MatchAccount = src.Account
		}

		migrationSources = append(migrationSources, ms)
	}

	if len(migrationSources) == 0 {
		return nil
	}

	logger := slog.Default()
	results := coressh.MigrateWorkspaceToSSH(context.Background(), root, migrationSources, logger)

	var changed, skipped, failed int
	for _, r := range results {
		switch {
		case r.Error != nil:
			failed++
		case r.Changed:
			changed++
		default:
			skipped++
		}
	}

	if changed > 0 || failed > 0 {
		logger.Info("[migration ssh_remotes] done",
			"changed", changed,
			"skipped", skipped,
			"failed", failed,
		)
	}

	return nil
}
