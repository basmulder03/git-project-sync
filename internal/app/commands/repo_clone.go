package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	coressh "github.com/basmulder03/git-project-sync/internal/core/ssh"
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

// BuildCloneURL builds an HTTPS clone URL for the given source and repo slug.
// Prefer BuildSSHCloneURL when SSH is enabled.
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

// BuildSSHCloneURL builds an SSH clone URL for the given source and repo slug.
// The URL uses the per-source SSH config alias so that git automatically
// selects the correct identity file.
//
// For GitHub:   git@gps-<sourceID>:owner/repo.git
// For Azure:    ssh://git@gps-<sourceID>/v3/org/project/repo
func BuildSSHCloneURL(source config.SourceConfig, repoSlug string) string {
	alias := coressh.AliasForSource(source.ID)
	provider := strings.ToLower(strings.TrimSpace(source.Provider))

	switch provider {
	case "azure", "azuredevops":
		org := strings.TrimSpace(source.Organization)
		if org == "" {
			org = strings.TrimSpace(source.Account)
		}
		// repoSlug is expected to be "project/repo" or just "repo".
		parts := strings.SplitN(repoSlug, "/", 2)
		if len(parts) == 2 {
			return coressh.CloneURLForAzureDevOps(org, parts[0], parts[1], alias)
		}
		return coressh.CloneURLForAzureDevOps(org, org, repoSlug, alias)

	default: // github
		owner := strings.TrimSpace(source.Organization)
		if owner == "" {
			owner = strings.TrimSpace(source.Account)
		}
		hostname := strings.TrimSpace(source.Host)
		if hostname == "" {
			hostname = "github.com"
		}
		if strings.Contains(repoSlug, "/") {
			parts := strings.SplitN(repoSlug, "/", 2)
			return coressh.CloneURLForGitHub(parts[0], parts[1], hostname, alias)
		}
		return coressh.CloneURLForGitHub(owner, repoSlug, hostname, alias)
	}
}

// PreferredCloneURL returns the SSH clone URL when SSH is enabled for the
// source, otherwise the HTTPS clone URL.
func PreferredCloneURL(cfg config.Config, source config.SourceConfig, repoSlug string, sshKeyExists bool) string {
	if cfg.SSHEnabledForSource(source) && sshKeyExists {
		return BuildSSHCloneURL(source, repoSlug)
	}
	return BuildCloneURL(source, repoSlug)
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
