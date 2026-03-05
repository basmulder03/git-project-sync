package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/providers"
)

func newSourceCommand(configPath *string) *cobra.Command {
	sourceCmd := &cobra.Command{
		Use:   "source",
		Short: "Manage provider sources",
	}

	sourceCmd.AddCommand(
		newSourceAddCommand(configPath),
		newSourceRemoveCommand(configPath),
		newSourceListCommand(configPath),
		newSourceShowCommand(configPath),
	)

	return sourceCmd
}

func newSourceAddCommand(configPath *string) *cobra.Command {
	var account string
	var org string
	var host string
	var enabled bool

	cmd := &cobra.Command{
		Use:   "add [github|azure] <source-id>",
		Short: "Add a provider source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := strings.ToLower(strings.TrimSpace(args[0]))
			sourceID := strings.TrimSpace(args[1])

			if account == "" {
				return errors.New("required flag: --account")
			}

			if host == "" {
				host = defaultHost(provider)
			}

			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			registry, err := providers.NewSourceRegistry(cfg.Sources)
			if err != nil {
				return err
			}

			err = registry.Add(config.SourceConfig{
				ID:           sourceID,
				Provider:     provider,
				Account:      account,
				Organization: org,
				Host:         host,
				Enabled:      enabled,
			})
			if err != nil {
				return err
			}

			cfg.Sources = registry.List()
			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}

			cmd.Printf("added source %s (%s)\n", sourceID, provider)
			return nil
		},
	}

	cmd.Flags().StringVar(&account, "account", "", "Account name")
	cmd.Flags().StringVar(&org, "org", "", "Organization/team context")
	cmd.Flags().StringVar(&host, "host", "", "Provider host")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Enable source immediately")

	return cmd
}

func newSourceRemoveCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <source-id>",
		Short: "Remove a provider source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			registry, err := providers.NewSourceRegistry(cfg.Sources)
			if err != nil {
				return err
			}

			if err := registry.Remove(args[0]); err != nil {
				return err
			}

			cfg.Sources = registry.List()
			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}

			cmd.Printf("removed source %s\n", args[0])
			return nil
		},
	}
}

func newSourceListCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List provider sources",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			if len(cfg.Sources) == 0 {
				cmd.Println("no sources configured")
				return nil
			}

			for _, source := range cfg.Sources {
				context := source.Organization
				if context == "" {
					context = "personal"
				}
				cmd.Printf("%s\t%s\t%s\t%s\tenabled=%t\n", source.ID, source.Provider, source.Account, context, source.Enabled)
			}

			return nil
		},
	}
}

func newSourceShowCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show <source-id>",
		Short: "Show source details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			registry, err := providers.NewSourceRegistry(cfg.Sources)
			if err != nil {
				return err
			}

			source, ok := registry.Get(args[0])
			if !ok {
				return fmt.Errorf("source %q not found", args[0])
			}

			cmd.Printf("id: %s\n", source.ID)
			cmd.Printf("provider: %s\n", source.Provider)
			cmd.Printf("account: %s\n", source.Account)
			cmd.Printf("organization: %s\n", source.Organization)
			cmd.Printf("host: %s\n", source.Host)
			cmd.Printf("enabled: %t\n", source.Enabled)
			cmd.Printf("credential_ref: %s\n", source.CredentialRef)
			return nil
		},
	}
}

func defaultHost(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		return "github.com"
	case "azure", "azuredevops":
		return "dev.azure.com"
	default:
		return ""
	}
}
