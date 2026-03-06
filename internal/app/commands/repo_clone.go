package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/workspace"
)

func ResolveCloneDestination(cfg config.Config, source config.SourceConfig, repoName, into string) (string, error) {
	if strings.TrimSpace(into) == "" {
		return filepath.Clean(repoName), nil
	}
	if strings.TrimSpace(into) != "managed" {
		return "", fmt.Errorf("invalid --into value %q (supported: managed)", into)
	}

	resolver, err := workspace.NewLayoutResolver(cfg.Workspace.Root)
	if err != nil {
		return "", err
	}

	expectedPath, err := resolver.ExpectedRepoPath(source, config.RepoConfig{Path: repoName, SourceID: source.ID})
	if err != nil {
		return "", err
	}
	return expectedPath, nil
}

func BuildCloneURL(source config.SourceConfig, repoSlug string) string {
	host := strings.TrimSpace(source.Host)
	if host == "" {
		host = defaultSourceHost(source.Provider)
	}
	provider := strings.ToLower(strings.TrimSpace(source.Provider))
	if provider == "azure" || provider == "azuredevops" {
		org := strings.TrimSpace(source.Organization)
		if org == "" {
			org = strings.TrimSpace(source.Account)
		}
		return fmt.Sprintf("https://%s/%s/%s/_git/%s", host, source.Account, org, repoSlug)
	}

	owner := strings.TrimSpace(source.Organization)
	if owner == "" {
		owner = strings.TrimSpace(source.Account)
	}
	if strings.Contains(repoSlug, "/") {
		return fmt.Sprintf("https://%s/%s.git", host, repoSlug)
	}
	return fmt.Sprintf("https://%s/%s/%s.git", host, owner, repoSlug)
}

func defaultSourceHost(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		return "github.com"
	case "azure", "azuredevops":
		return "dev.azure.com"
	default:
		return ""
	}
}
