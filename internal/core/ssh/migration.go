package ssh

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MigrationResult describes the outcome of a single repository's SSH migration.
type MigrationResult struct {
	RepoPath   string
	OldURL     string
	NewURL     string
	Changed    bool
	Skipped    bool
	SkipReason string
	Error      error
}

// MigrateRepoToSSH rewrites the "origin" remote of a single repository from
// HTTPS to SSH.  It uses the SSH alias derived from sourceID so that git
// automatically picks the correct identity file.
//
// The function is safe to call on a repo that is already using SSH (no-op) and
// on repos whose HTTPS origin contains an embedded PAT token.
//
// It returns (changed, error).
func MigrateRepoToSSH(
	ctx context.Context,
	repoPath string,
	sourceID string,
	provider string,
	logger *slog.Logger,
) MigrationResult {
	result := MigrationResult{RepoPath: repoPath}

	// We need a git client to read / write the remote URL.
	// Use the raw exec approach to avoid importing the git package (cycle risk).
	currentURL, err := getRemoteURL(ctx, repoPath, "origin")
	if err != nil {
		result.Skipped = true
		result.SkipReason = "no origin remote"
		logger.Debug("ssh migration: skipped (no origin remote)", "repo", repoPath, "err", err)
		return result
	}

	result.OldURL = currentURL

	// If already SSH, nothing to do.
	if IsSSHURL(currentURL) {
		result.Skipped = true
		result.SkipReason = "already_ssh"
		logger.Debug("ssh migration: already SSH", "repo", repoPath, "url", currentURL)
		return result
	}

	// Convert to SSH.
	alias := AliasForSource(sourceID)
	newURL, ok := httpsToSSHWithAlias(currentURL, provider, alias)
	if !ok {
		result.Skipped = true
		result.SkipReason = "unrecognized_url"
		logger.Warn("ssh migration: cannot convert URL to SSH", "repo", repoPath, "url", currentURL)
		return result
	}

	result.NewURL = newURL

	if err := setRemoteURL(ctx, repoPath, "origin", newURL); err != nil {
		result.Error = fmt.Errorf("set remote URL: %w", err)
		logger.Error("ssh migration: failed to update remote", "repo", repoPath, "err", err)
		return result
	}

	result.Changed = true
	logger.Info("ssh migration: updated origin to SSH", "repo", repoPath, "old", currentURL, "new", newURL)
	return result
}

// MigrateWorkspaceToSSH migrates all git repositories under workspaceRoot to
// SSH remotes.  Repositories are matched to a source by checking whether their
// HTTPS origin URL belongs to the provider/account of the source.
//
// This function is called during the opt-in migration flow triggered on first
// startup after the SSH feature is introduced.
func MigrateWorkspaceToSSH(
	ctx context.Context,
	workspaceRoot string,
	sources []MigrationSource,
	logger *slog.Logger,
) []MigrationResult {
	if workspaceRoot == "" {
		return nil
	}

	if _, err := os.Stat(workspaceRoot); os.IsNotExist(err) {
		return nil
	}

	// Find all git repos.
	repoPaths, err := findGitReposForMigration(workspaceRoot)
	if err != nil {
		logger.Error("ssh migration: scan workspace", "err", err)
		return nil
	}

	var results []MigrationResult

	for _, repoPath := range repoPaths {
		select {
		case <-ctx.Done():
			logger.Warn("ssh migration: context cancelled", "remaining", len(repoPaths))
			return results
		default:
		}

		repoCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		currentURL, err := getRemoteURL(repoCtx, repoPath, "origin")
		cancel()

		if err != nil {
			results = append(results, MigrationResult{
				RepoPath:   repoPath,
				Skipped:    true,
				SkipReason: "no_origin",
			})
			continue
		}

		// Match to a source.
		src, ok := matchSourceForURL(currentURL, sources)
		if !ok {
			results = append(results, MigrationResult{
				RepoPath:   repoPath,
				OldURL:     currentURL,
				Skipped:    true,
				SkipReason: "unmatched_source",
			})
			continue
		}

		migrCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		result := MigrateRepoToSSH(migrCtx, repoPath, src.SourceID, src.Provider, logger)
		cancel()

		results = append(results, result)
	}

	return results
}

// MigrationSource describes a configured source for matching during migration.
type MigrationSource struct {
	SourceID string
	Provider string
	// MatchHosts is the set of HTTPS hostnames that belong to this source
	// (e.g. ["github.com"] or ["dev.azure.com"]).
	MatchHosts []string
	// MatchAccount is used for providers that embed the org/account in the URL.
	MatchAccount string
}

// matchSourceForURL returns the first source whose MatchHosts contains the
// hostname from the given HTTPS URL.
func matchSourceForURL(rawURL string, sources []MigrationSource) (MigrationSource, bool) {
	// Strip embedded credentials.
	url := rawURL
	if strings.HasPrefix(url, "https://") {
		rest := url[len("https://"):]
		if idx := strings.Index(rest, "@"); idx >= 0 {
			url = "https://" + rest[idx+1:]
		}
	}

	for _, src := range sources {
		for _, host := range src.MatchHosts {
			if strings.Contains(url, host) {
				if src.MatchAccount != "" {
					// For providers like Azure DevOps where the account is in the path.
					if !strings.Contains(url, "/"+src.MatchAccount+"/") &&
						!strings.HasPrefix(url, "https://"+host+"/"+src.MatchAccount) {
						continue
					}
				}
				return src, true
			}
		}
	}
	return MigrationSource{}, false
}

// httpsToSSHWithAlias converts an HTTPS clone URL to SSH using the provided alias.
func httpsToSSHWithAlias(httpsURL, provider, alias string) (string, bool) {
	// Strip embedded credentials.
	url := httpsURL
	if strings.HasPrefix(url, "https://") {
		rest := url[len("https://"):]
		if idx := strings.Index(rest, "@"); idx >= 0 {
			afterAt := rest[idx+1:]
			hostPart := strings.SplitN(afterAt, "/", 2)[0]
			if strings.Contains(hostPart, ".") {
				url = "https://" + afterAt
			}
		}
	}

	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		path := strings.TrimPrefix(url, "https://github.com/")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 2 {
			return CloneURLForGitHub(parts[0], parts[1], "github.com", alias), true
		}

	case "azuredevops", "azure":
		path := strings.TrimPrefix(url, "https://dev.azure.com/")
		parts := strings.Split(path, "/")
		// Expected: org / project / _git / repo
		if len(parts) >= 4 && parts[2] == "_git" {
			return CloneURLForAzureDevOps(parts[0], parts[1], parts[3], alias), true
		}
	}

	// Generic fallback: try the converter.
	converted, ok := HTTPSToSSH(url)
	if ok {
		// Replace the generic host with the alias if possible.
		if alias != "" {
			// git@github.com: → git@<alias>:
			converted = strings.Replace(converted, "@github.com:", "@"+alias+":", 1)
			// ssh://git@ssh.dev.azure.com/ → ssh://git@<alias>/
			converted = strings.Replace(converted, "@ssh.dev.azure.com/", "@"+alias+"/", 1)
		}
		return converted, true
	}

	return "", false
}

// --- lightweight git helpers (no import cycle) ---

func getRemoteURL(ctx context.Context, repoPath, remote string) (string, error) {
	out, err := runGit(ctx, repoPath, "remote", "get-url", remote)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func setRemoteURL(ctx context.Context, repoPath, remote, newURL string) error {
	_, err := runGit(ctx, repoPath, "remote", "set-url", remote, newURL)
	return err
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	// Import exec directly to avoid importing the git package and creating a cycle.
	// This is intentional: the ssh package must not depend on the git package.
	cmd := newGitCommand(ctx, dir, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func findGitReposForMigration(root string) ([]string, error) {
	var repos []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable paths
		}

		if !d.IsDir() {
			return nil
		}

		// Skip hidden directories.
		if strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}

		// Check whether this directory is a git work tree.
		gitDir := filepath.Join(path, ".git")
		if info, statErr := os.Stat(gitDir); statErr == nil && info.IsDir() {
			repos = append(repos, path)
			return filepath.SkipDir // don't recurse into submodules
		}

		return nil
	})

	return repos, err
}
