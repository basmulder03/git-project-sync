package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	coressh "github.com/basmulder03/git-project-sync/internal/core/ssh"
)

// newAuthSSHCommand returns the "auth ssh" subcommand group.
func newAuthSSHCommand(configPath *string) *cobra.Command {
	sshCmd := &cobra.Command{
		Use:   "ssh",
		Short: "Manage SSH keys for git operations",
	}

	sshCmd.AddCommand(
		newAuthSSHKeygenCommand(configPath),
		newAuthSSHAddKeyCommand(configPath),
		newAuthSSHTestCommand(configPath),
		newAuthSSHShowCommand(configPath),
		newAuthSSHMigrateCommand(configPath),
	)

	return sshCmd
}

// newAuthSSHKeygenCommand generates an SSH key pair for a source.
func newAuthSSHKeygenCommand(configPath *string) *cobra.Command {
	var keyType string
	var force bool

	cmd := &cobra.Command{
		Use:   "keygen <source-id>",
		Short: "Generate an SSH key pair for a source",
		Long: `Generate a new Ed25519 SSH key pair for the given source.

The private key is stored in the application SSH directory and never leaves
the machine. The public key can be uploaded to the provider with "auth ssh add-key".

An existing key will not be overwritten unless --force is specified.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceID := args[0]
			cfg, source, err := loadSourceByID(*configPath, sourceID)
			if err != nil {
				return err
			}

			mgr := newSSHManager(&cfg)

			privPath := mgr.PrivateKeyPath(sourceID)

			if force && coressh.KeyExists(privPath) {
				cmd.Printf("removing existing key at %s\n", privPath)
				if _, err := mgr.RegenerateKey(sourceID, ""); err != nil {
					return fmt.Errorf("regenerate key: %w", err)
				}
				cmd.Printf("regenerated SSH key for source %s\n", sourceID)
				cmd.Printf("public key path: %s.pub\n", privPath)
				return nil
			}

			kt := coressh.KeyType(strings.ToLower(keyType))
			if kt == "" {
				kt = coressh.DefaultKeyType
			}

			comment := fmt.Sprintf("git-project-sync/%s", sourceID)
			kp, err := coressh.GenerateKeyPair(privPath, kt, comment)
			if err != nil {
				return fmt.Errorf("generate key: %w", err)
			}

			cmd.Printf("generated %s SSH key for source %s\n", kt, sourceID)
			cmd.Printf("private key: %s\n", kp.PrivateKeyPath)
			cmd.Printf("public key:  %s\n", kp.PublicKeyPath)
			cmd.Printf("\nNext step: upload the public key with:\n")
			cmd.Printf("  syncctl auth ssh add-key %s\n", source.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&keyType, "type", "ed25519", "Key type: ed25519 or ecdsa")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing key (removes old key first)")
	return cmd
}

// newAuthSSHAddKeyCommand uploads the public key to the provider.
func newAuthSSHAddKeyCommand(configPath *string) *cobra.Command {
	var title string
	var pat string
	var clientID string
	var forceFallback bool

	cmd := &cobra.Command{
		Use:   "add-key <source-id>",
		Short: "Upload public key to the provider",
		Long: `Upload the SSH public key for the given source to the provider.

For GitHub sources:  Uses the OAuth device flow (no PAT write:public_key scope
required). You will be prompted to visit a URL and enter a code in your browser.

For Azure DevOps sources: Uses the PAT token directly. The PAT must have the
"SSH Public Keys (Read and Write)" permission. Use --pat to supply the PAT, or
it will be read from the token store (the stored PAT must include SSH scope).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			sourceID := args[0]
			cfg, source, err := loadSourceByID(*configPath, sourceID)
			if err != nil {
				return err
			}

			mgr := newSSHManager(&cfg)

			if !mgr.HasKey(sourceID) {
				return fmt.Errorf("no SSH key found for source %s; run 'auth ssh keygen %s' first", sourceID, sourceID)
			}

			keyTitle := title
			if keyTitle == "" {
				hostname, _ := os.Hostname()
				keyTitle = fmt.Sprintf("git-project-sync/%s@%s", sourceID, hostname)
			}

			switch strings.ToLower(source.Provider) {
			case "github":
				oid := clientID
				if oid == "" {
					oid = cfg.SSH.GitHubOAuthClientID
				}
				cmd.Printf("Uploading SSH key to GitHub via device flow...\n")
				if err := mgr.GitHubUploadKey(ctx, sourceID, keyTitle, oid, func(userCode, uri string) {
					cmd.Printf("\nOpen this URL in your browser:\n  %s\n\nThen enter the code: %s\n\nWaiting for authorization", uri, userCode)
				}); err != nil {
					return fmt.Errorf("github ssh key upload: %w", err)
				}
				cmd.Printf("\nSSH key uploaded successfully to GitHub for source %s\n", sourceID)

			case "azuredevops", "azure":
				patValue := strings.TrimSpace(pat)
				if patValue == "" {
					patValue = strings.TrimSpace(os.Getenv("SYNCCTL_TOKEN"))
				}
				if patValue == "" {
					// Try the stored token.
					store, storeErr := newTokenStore(*configPath, forceFallback)
					if storeErr == nil {
						patValue, _ = store.GetToken(ctx, sourceID)
					}
				}
				if patValue == "" {
					return fmt.Errorf("PAT is required for Azure DevOps SSH key upload (use --pat or SYNCCTL_TOKEN)")
				}

				org := source.Organization
				if org == "" {
					org = source.Account
				}
				if org == "" {
					return fmt.Errorf("source %s has no organization/account configured", sourceID)
				}

				cmd.Printf("Uploading SSH key to Azure DevOps (org: %s)...\n", org)
				if err := mgr.AzureDevOpsUploadKey(ctx, sourceID, patValue, org, keyTitle); err != nil {
					return fmt.Errorf("azure devops ssh key upload: %w", err)
				}
				cmd.Printf("SSH key uploaded successfully to Azure DevOps for source %s\n", sourceID)

			default:
				return fmt.Errorf("provider %q does not support automated SSH key upload; add the public key manually", source.Provider)
			}

			// Mark key as uploaded in config.
			if err := markSSHKeyUploaded(*configPath, sourceID); err != nil {
				// Non-fatal — log but don't fail.
				cmd.Printf("warning: could not update config to mark key as uploaded: %v\n", err)
			}

			// Also ensure the SSH config entry exists.
			if err := mgr.EnsureSSHConfigEntry(sourceID, source.Provider, source.SSH.SSHHost); err != nil {
				cmd.Printf("warning: could not update ~/.ssh/config: %v\n", err)
			} else {
				cmd.Printf("SSH config entry written for alias %s\n", coressh.AliasForSource(sourceID))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Key title to use on the provider (default: git-project-sync/<source-id>@<hostname>)")
	cmd.Flags().StringVar(&pat, "pat", "", "Azure DevOps PAT with SSH Public Keys scope (or set SYNCCTL_TOKEN)")
	cmd.Flags().StringVar(&clientID, "client-id", "", "Override GitHub OAuth app client ID for device flow")
	cmd.Flags().BoolVar(&forceFallback, "force-fallback", false, "Use encrypted fallback token store")
	return cmd
}

// newAuthSSHTestCommand tests SSH connectivity for a source.
func newAuthSSHTestCommand(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <source-id>",
		Short: "Test SSH connectivity for a source",
		Long: `Test whether the SSH key for the given source can authenticate
with the provider's SSH endpoint.

For GitHub:     runs "ssh -T git@github.com" (or the alias)
For Azure DevOps: runs "ssh -T git@ssh.dev.azure.com"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			sourceID := args[0]
			cfg, source, err := loadSourceByID(*configPath, sourceID)
			if err != nil {
				return err
			}

			mgr := newSSHManager(&cfg)

			if !mgr.HasKey(sourceID) {
				return fmt.Errorf("no SSH key found for source %s; run 'auth ssh keygen %s' first", sourceID, sourceID)
			}

			privPath := mgr.PrivateKeyPath(sourceID)
			alias := coressh.AliasForSource(sourceID)
			hostname := coressh.DefaultHostname(source.Provider)
			if source.SSH.SSHHost != "" {
				hostname = source.SSH.SSHHost
			}

			// Build SSH test command using GIT_SSH_COMMAND pattern.
			sshArgs := []string{
				"ssh",
				"-i", privPath,
				"-o", "IdentitiesOnly=yes",
				"-o", "StrictHostKeyChecking=accept-new",
				"-o", "BatchMode=yes",
				"-T",
				"git@" + hostname,
			}

			cmd.Printf("Testing SSH connectivity for source %s (alias: %s)\n", sourceID, alias)
			cmd.Printf("Running: %s\n\n", strings.Join(sshArgs, " "))

			// SSH returns exit code 1 even on success (GitHub prints "Hi user!")
			// so we capture stderr and treat authentication success as acceptable.
			sshCmd := coressh.NewSSHTestCommand(ctx, sshArgs[1:])
			output, err := sshCmd.CombinedOutput()

			outStr := strings.TrimSpace(string(output))
			if outStr != "" {
				cmd.Printf("%s\n\n", outStr)
			}

			if err != nil {
				// Exit code 1 from GitHub/Azure DevOps with "successfully authenticated" is OK.
				if strings.Contains(outStr, "successfully authenticated") ||
					strings.Contains(outStr, "Hi ") {
					cmd.Printf("SSH authentication successful for source %s\n", sourceID)
					return nil
				}
				// Real failure.
				return fmt.Errorf("SSH test failed for source %s: %w", sourceID, err)
			}

			cmd.Printf("SSH connection successful for source %s\n", sourceID)
			return nil
		},
	}

	return cmd
}

// newAuthSSHShowCommand prints the public key for a source.
func newAuthSSHShowCommand(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <source-id>",
		Short: "Print the SSH public key for a source",
		Long: `Print the OpenSSH public key for the given source.

Use this to manually add the key to the provider's SSH key settings when
the automated upload (auth ssh add-key) is not available or preferred.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceID := args[0]
			cfg, _, err := loadSourceByID(*configPath, sourceID)
			if err != nil {
				return err
			}

			mgr := newSSHManager(&cfg)

			if !mgr.HasKey(sourceID) {
				return fmt.Errorf("no SSH key found for source %s; run 'auth ssh keygen %s' first", sourceID, sourceID)
			}

			pubKey, err := mgr.PublicKeyContent(sourceID)
			if err != nil {
				return err
			}

			cmd.Printf("%s\n", pubKey)
			return nil
		},
	}

	return cmd
}

// newAuthSSHMigrateCommand triggers the opt-in SSH remote migration.
func newAuthSSHMigrateCommand(configPath *string) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate existing repos from HTTPS to SSH remotes",
		Long: `Rewrite the git remote URLs of all managed repositories from HTTPS to SSH.

This is a one-time migration. After running this command, git fetch/pull/push
operations will use SSH (with per-source identity files) instead of HTTPS
with embedded PAT tokens.

Safety rules:
  - Only repos with a known, clean HTTPS origin are touched.
  - Repos with dirty working trees are NOT modified (remotes are path data).
  - The migration is idempotent: repos already using SSH are skipped.

If you decline, you can always run this command again later.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			if !yes {
				cmd.Printf("This will rewrite HTTPS remote URLs to SSH for all managed repositories.\n")
				cmd.Printf("Workspace: %s\n\n", cfg.Workspace.Root)
				cmd.Printf("Proceed? [y/N] ")

				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "y" && answer != "yes" {
					cfg.SSH.MigrationOptIn = "declined"
					_ = config.Save(*configPath, cfg)
					cmd.Printf("Migration declined. You can run this command again any time.\n")
					return nil
				}
			}

			cfg.SSH.MigrationOptIn = "accepted"
			if err := config.Save(*configPath, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			// Build migration sources from configured sources.
			var migSources []coressh.MigrationSource
			for _, src := range cfg.Sources {
				if !src.Enabled {
					continue
				}
				ms := coressh.MigrationSource{
					SourceID:     src.ID,
					Provider:     src.Provider,
					MatchAccount: src.Organization,
				}
				switch strings.ToLower(src.Provider) {
				case "github":
					ms.MatchHosts = []string{"github.com"}
				case "azuredevops", "azure":
					ms.MatchHosts = []string{"dev.azure.com"}
					if src.Organization != "" {
						ms.MatchAccount = src.Organization
					} else {
						ms.MatchAccount = src.Account
					}
				}
				migSources = append(migSources, ms)
			}

			if cfg.Workspace.Root == "" {
				return fmt.Errorf("workspace.root is not configured; cannot scan for repositories")
			}

			cmd.Printf("Scanning %s for git repositories...\n\n", cfg.Workspace.Root)

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
			ctx := context.Background()

			results := coressh.MigrateWorkspaceToSSH(ctx, cfg.Workspace.Root, migSources, logger)

			var changed, skipped, failed int
			for _, r := range results {
				switch {
				case r.Error != nil:
					failed++
					cmd.Printf("  FAIL    %s\n    error: %v\n", r.RepoPath, r.Error)
				case r.Changed:
					changed++
					cmd.Printf("  UPDATED %s\n    %s\n    → %s\n", r.RepoPath, r.OldURL, r.NewURL)
				case r.Skipped:
					skipped++
					cmd.Printf("  SKIP    %s (%s)\n", r.RepoPath, r.SkipReason)
				}
			}

			cmd.Printf("\nMigration complete: %d updated, %d skipped, %d failed\n", changed, skipped, failed)
			if failed > 0 {
				return fmt.Errorf("%d repositories failed to migrate", failed)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

// --- helpers ---

// newSSHManager builds an ssh.Manager from config, with WSL ↔ Windows interop
// when running inside WSL and wsl.sync_to_windows is enabled (default: true).
//
// WSL interop logic:
//  1. Detect if running in WSL (IsWSL()).
//  2. If yes, and cfg.WSLSyncToWindows() is true:
//     a. Resolve the Windows SSH config path (/mnt/c/Users/<user>/.ssh/config).
//     b. Resolve the Windows SSH key dir (/mnt/c/Users/<user>/AppData/Local/.../ssh).
//     c. If cfg.WSLUseWindowsKeyDir() is true, use the Windows key dir as SSHDir
//     so keys are stored there (accessible from both WSL and native Windows).
//     d. Build a Manager with the Windows paths for dual-config mirroring.
func newSSHManager(cfg *config.Config) *coressh.Manager {
	sshDir := cfg.SSHDir()
	sshConfigPath := cfg.SSHConfigPath()

	if !coressh.IsWSL() || !cfg.WSLSyncToWindows() {
		return coressh.NewManager(sshDir, sshConfigPath, nil)
	}

	// --- WSL interop path ---

	// Resolve Windows SSH config path.
	winSSHConfigPath := cfg.WSLWindowsSSHConfigPath()
	if winSSHConfigPath == "" {
		if p, ok := coressh.WindowsSSHConfigPath(); ok {
			winSSHConfigPath = p
		}
	}

	// Resolve Windows key dir.
	winKeyDir := cfg.WSLWindowsKeyDir()
	if winKeyDir == "" {
		if d, ok := coressh.WindowsSSHDir(); ok {
			winKeyDir = d
		}
	}

	// When UseWindowsKeyDir is enabled and we have a valid Windows key dir,
	// store keys there so both WSL and native Windows git can read them.
	if cfg.WSLUseWindowsKeyDir() && winKeyDir != "" {
		sshDir = winKeyDir
	}

	return coressh.NewManagerWSL(sshDir, sshConfigPath, winKeyDir, winSSHConfigPath, nil)
}

// markSSHKeyUploaded sets SourceSSHConfig.KeyUploaded = true for sourceID and saves config.
func markSSHKeyUploaded(configPath, sourceID string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	for i, src := range cfg.Sources {
		if src.ID == sourceID {
			cfg.Sources[i].SSH.KeyUploaded = true
			break
		}
	}
	return config.Save(configPath, cfg)
}
