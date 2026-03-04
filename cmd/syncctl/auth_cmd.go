package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/auth"
	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/providers"
)

func newAuthCommand(configPath *string) *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage credentials",
	}

	authCmd.AddCommand(
		newAuthLoginCommand(configPath),
		newAuthTestCommand(configPath),
		newAuthLogoutCommand(configPath),
	)

	return authCmd
}

func newAuthLoginCommand(configPath *string) *cobra.Command {
	var token string
	var forceFallback bool

	cmd := &cobra.Command{
		Use:   "login <source-id>",
		Short: "Store and validate PAT for source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tokenValue := strings.TrimSpace(token)
			if tokenValue == "" {
				tokenValue = strings.TrimSpace(os.Getenv("SYNCCTL_TOKEN"))
			}
			if tokenValue == "" {
				return fmt.Errorf("token is required (use --token or SYNCCTL_TOKEN)")
			}

			cfg, source, err := loadSourceByID(*configPath, args[0])
			if err != nil {
				return err
			}

			if err := providers.ValidatePAT(context.Background(), source, tokenValue); err != nil {
				return err
			}

			store, err := newTokenStore(*configPath, forceFallback)
			if err != nil {
				return err
			}

			if err := store.SetToken(context.Background(), source.ID, tokenValue); err != nil {
				return err
			}

			if err := setCredentialRef(&cfg, source.ID, forceFallback); err != nil {
				return err
			}
			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}

			cmd.Printf("login successful for source %s\n", source.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&token, "token", "", "PAT token (or set SYNCCTL_TOKEN)")
	cmd.Flags().BoolVar(&forceFallback, "force-fallback", false, "Use encrypted fallback token store instead of OS keyring")
	return cmd
}

func newAuthTestCommand(configPath *string) *cobra.Command {
	var forceFallback bool

	cmd := &cobra.Command{
		Use:   "test <source-id>",
		Short: "Validate current PAT for source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, source, err := loadSourceByID(*configPath, args[0])
			if err != nil {
				return err
			}

			store, err := newTokenStore(*configPath, forceFallback)
			if err != nil {
				return err
			}

			tokenValue, err := store.GetToken(context.Background(), source.ID)
			if err != nil {
				return err
			}

			if err := providers.ValidatePAT(context.Background(), source, tokenValue); err != nil {
				return err
			}

			cmd.Printf("token is valid for source %s\n", source.ID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&forceFallback, "force-fallback", false, "Use encrypted fallback token store instead of OS keyring")
	return cmd
}

func newAuthLogoutCommand(configPath *string) *cobra.Command {
	var forceFallback bool

	cmd := &cobra.Command{
		Use:   "logout <source-id>",
		Short: "Delete PAT for source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, source, err := loadSourceByID(*configPath, args[0])
			if err != nil {
				return err
			}

			store, err := newTokenStore(*configPath, forceFallback)
			if err != nil {
				return err
			}

			if err := store.DeleteToken(context.Background(), source.ID); err != nil {
				return err
			}

			cmd.Printf("logged out source %s\n", source.ID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&forceFallback, "force-fallback", false, "Use encrypted fallback token store instead of OS keyring")
	return cmd
}

func newTokenStore(configPath string, forceFallback bool) (auth.TokenStore, error) {
	secretsPath := filepath.Join(filepath.Dir(configPath), "secrets", "tokens.enc")
	return auth.NewTokenStore(auth.Options{
		ServiceName:    "git-project-sync",
		FallbackPath:   secretsPath,
		FallbackKeyEnv: "GIT_PROJECT_SYNC_FALLBACK_KEY",
		ForceFallback:  forceFallback,
	})
}

func setCredentialRef(cfg *config.Config, sourceID string, fallback bool) error {
	for i, source := range cfg.Sources {
		if source.ID != sourceID {
			continue
		}

		if fallback {
			cfg.Sources[i].CredentialRef = "fallback://git-project-sync/sources/" + sourceID
		} else {
			cfg.Sources[i].CredentialRef = "keyring://git-project-sync/sources/" + sourceID
		}
		return nil
	}

	return fmt.Errorf("source %q not found", sourceID)
}
