package workspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/providers/api"
)

type DiscoveryResult struct {
	Repos        []config.RepoConfig
	Skipped      []string
	RemoteRepos  []api.RemoteRepository // All repos from provider APIs
	ReposToClone []api.RemoteRepository // Repos that pass governance and don't exist locally
}

// SkippedRemoteRepo represents a remote repository that was skipped during discovery
type SkippedRemoteRepo struct {
	Repo       api.RemoteRepository
	ReasonCode string
	Message    string
}

func ResolveRunRepos(cfg config.Config) (DiscoveryResult, error) {
	result := DiscoveryResult{Repos: append([]config.RepoConfig(nil), cfg.Repos...)}

	workspaceRoot := strings.TrimSpace(cfg.Workspace.Root)
	if workspaceRoot == "" {
		return result, nil
	}

	paths, err := discoverGitRepos(workspaceRoot)
	if err != nil {
		return DiscoveryResult{}, err
	}

	configuredByPath := map[string]struct{}{}
	for _, repo := range cfg.Repos {
		configuredByPath[filepath.Clean(repo.Path)] = struct{}{}
	}

	for _, repoPath := range paths {
		if _, exists := configuredByPath[filepath.Clean(repoPath)]; exists {
			continue
		}

		sourceID, ok := inferSourceID(cfg, repoPath)
		if !ok {
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s (source not resolved)", repoPath))
			continue
		}

		result.Repos = append(result.Repos, config.RepoConfig{
			Path:                       repoPath,
			SourceID:                   sourceID,
			Remote:                     "origin",
			Enabled:                    true,
			Provider:                   "auto",
			CleanupMergedLocalBranches: true,
			SkipIfDirty:                true,
		})
	}

	return result, nil
}

func discoverGitRepos(root string) ([]string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	root = filepath.Clean(root)

	reposByPath := map[string]struct{}{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if isIgnorableWalkError(walkErr) {
				return nil
			}
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() != ".git" {
			return nil
		}

		repoPath := filepath.Clean(filepath.Dir(path))
		reposByPath[repoPath] = struct{}{}
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(reposByPath))
	for repoPath := range reposByPath {
		out = append(out, repoPath)
	}
	sort.Strings(out)
	return out, nil
}

func isIgnorableWalkError(err error) bool {
	return errors.Is(err, fs.ErrPermission) || errors.Is(err, os.ErrPermission)
}

func inferSourceID(cfg config.Config, repoPath string) (string, bool) {
	enabled := make([]config.SourceConfig, 0, len(cfg.Sources))
	for _, source := range cfg.Sources {
		if source.Enabled {
			enabled = append(enabled, source)
		}
	}

	if len(enabled) == 1 {
		return enabled[0].ID, true
	}

	workspaceRoot := strings.TrimSpace(cfg.Workspace.Root)
	if workspaceRoot == "" {
		return "", false
	}

	rel, err := filepath.Rel(filepath.Clean(workspaceRoot), filepath.Clean(repoPath))
	if err != nil {
		return "", false
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}

	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 3 {
		return "", false
	}

	repoProvider := normalizeProvider(parts[0])
	repoAccount := sanitizeSegment(parts[1])

	matches := make([]string, 0)
	for _, source := range enabled {
		sourceProvider := normalizeProvider(source.Provider)
		sourceAccount := source.Account
		if strings.TrimSpace(source.Organization) != "" {
			sourceAccount = source.Organization
		}
		sourceAccount = sanitizeSegment(sourceAccount)
		if repoProvider == sourceProvider && repoAccount == sourceAccount {
			matches = append(matches, source.ID)
		}
	}

	if len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

// DiscoverRemoteRepos queries provider APIs to discover all accessible repositories
func DiscoverRemoteRepos(ctx context.Context, cfg config.Config, clientFactory *api.ClientFactory, getToken func(sourceID string) (string, error)) ([]api.RemoteRepository, error) {
	var allRepos []api.RemoteRepository

	for _, source := range cfg.Sources {
		if !source.Enabled {
			continue
		}

		// Get token for this source
		token, err := getToken(source.ID)
		if err != nil {
			// Skip sources without valid tokens
			continue
		}

		// Create API client for this source
		client, err := clientFactory.CreateClient(source, token)
		if err != nil {
			continue
		}

		// Build list options from config
		opts := buildListOptions(cfg, source.ID)

		// Query provider API for repos
		repos, err := client.ListRepositories(ctx, opts)
		if err != nil {
			// Log error but continue with other sources
			continue
		}

		allRepos = append(allRepos, repos...)
	}

	return allRepos, nil
}

// buildListOptions constructs API list options from config governance settings
func buildListOptions(cfg config.Config, sourceID string) api.ListOptions {
	policy := cfg.Governance.DefaultPolicy

	// Check for source-specific override
	if sourcePolicy, ok := cfg.Governance.SourcePolicies[sourceID]; ok {
		// Apply source-specific overrides
		if sourcePolicy.AutoCloneEnabled != nil && !*sourcePolicy.AutoCloneEnabled {
			// If auto-clone is disabled for this source, return restrictive options
			return api.ListOptions{
				IncludeArchived: false,
				IncludeForks:    false,
				MaxSizeKB:       0,
			}
		}
		if sourcePolicy.AutoCloneMaxSizeMB > 0 {
			policy.AutoCloneMaxSizeMB = sourcePolicy.AutoCloneMaxSizeMB
		}
		if sourcePolicy.AutoCloneIncludeArchived {
			policy.AutoCloneIncludeArchived = sourcePolicy.AutoCloneIncludeArchived
		}
	}

	// Build options from policy
	maxSizeKB := int64(0)
	if policy.AutoCloneMaxSizeMB > 0 {
		maxSizeKB = int64(policy.AutoCloneMaxSizeMB) * 1024
	}

	return api.ListOptions{
		IncludeArchived: policy.AutoCloneIncludeArchived,
		IncludeForks:    false, // Default to false for forks
		MaxSizeKB:       maxSizeKB,
	}
}

// IdentifyReposToClone compares remote repos with local repos to identify missing ones
func IdentifyReposToClone(cfg config.Config, remoteRepos []api.RemoteRepository, localRepos []config.RepoConfig) []api.RemoteRepository {
	// Build map of existing local repo paths
	localPaths := make(map[string]struct{})
	for _, repo := range localRepos {
		localPaths[filepath.Clean(repo.Path)] = struct{}{}
	}

	layout := NewLayout(cfg.Workspace)
	var reposToClone []api.RemoteRepository

	for _, remote := range remoteRepos {
		// Calculate where this repo would be cloned
		targetPath := layout.RepoPath(remote.Provider, remote.Owner, remote.Name)

		// Check if it already exists locally
		if _, exists := localPaths[filepath.Clean(targetPath)]; exists {
			continue
		}

		// Check if the directory already exists (even if not in config)
		if _, err := os.Stat(targetPath); err == nil {
			continue
		}

		reposToClone = append(reposToClone, remote)
	}

	return reposToClone
}
