package workspace

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

type DiscoveryResult struct {
	Repos   []config.RepoConfig
	Skipped []string
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
