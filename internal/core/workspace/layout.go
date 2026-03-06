package workspace

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

type LayoutResolver struct {
	root string
}

func NewLayoutResolver(root string) (*LayoutResolver, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("workspace root is required")
	}

	return &LayoutResolver{root: filepath.Clean(root)}, nil
}

func (r *LayoutResolver) Root() string {
	return r.root
}

func (r *LayoutResolver) ExpectedRepoPath(source config.SourceConfig, repo config.RepoConfig) (string, error) {
	repoName := sanitizeSegment(filepath.Base(filepath.Clean(repo.Path)))
	if repoName == "" || repoName == "." {
		return "", errors.New("repo path must include a repository name")
	}

	provider := normalizeProvider(source.Provider)
	accountOrOrg := source.Account
	if strings.TrimSpace(source.Organization) != "" {
		accountOrOrg = source.Organization
	}

	return filepath.Join(
		r.root,
		sanitizeSegment(provider),
		sanitizeSegment(accountOrOrg),
		repoName,
	), nil
}

func normalizeProvider(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "azure" {
		return "azuredevops"
	}
	return v
}

func sanitizeSegment(value string) string {
	v := strings.TrimSpace(strings.ToLower(value))
	v = strings.ReplaceAll(v, "\\", "-")
	v = strings.ReplaceAll(v, "/", "-")
	v = strings.ReplaceAll(v, " ", "-")
	v = strings.Trim(v, ".-")
	return v
}

// Layout provides path resolution for repositories
type Layout struct {
	root string
}

// NewLayout creates a new layout resolver from workspace config
func NewLayout(workspace config.WorkspaceConfig) *Layout {
	return &Layout{
		root: filepath.Clean(workspace.Root),
	}
}

// RepoPath calculates the target path for a repository based on workspace layout
func (l *Layout) RepoPath(provider, owner, name string) string {
	provider = normalizeProvider(provider)
	name = sanitizeSegment(name)

	// For Azure DevOps, owner is in format "account/project" and should create nested directories
	if provider == "azuredevops" && strings.Contains(owner, "/") {
		parts := strings.Split(owner, "/")
		// Sanitize each part separately to preserve directory structure
		for i := range parts {
			parts[i] = sanitizeSegment(parts[i])
		}
		// Build path: root/azuredevops/account/project/repo
		pathParts := append([]string{l.root, provider}, parts...)
		pathParts = append(pathParts, name)
		return filepath.Join(pathParts...)
	}

	// For other providers (GitHub), use flat structure with sanitized owner
	owner = sanitizeSegment(owner)
	return filepath.Join(l.root, provider, owner, name)
}
