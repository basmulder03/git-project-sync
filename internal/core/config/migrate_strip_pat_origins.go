package config

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/git"
)

// migrateStripPATFromOrigins walks every git repository found under the
// configured workspace root, checks whether the "origin" remote URL has a
// PAT token embedded in it (e.g. https://<token>@github.com/…), and
// rewrites the URL to the clean, credential-free form.
//
// The migration is safe to run more than once: repos whose origins are
// already clean are left untouched.  Repos with no "origin" remote or that
// cannot be read are skipped with a log warning rather than aborting the
// whole migration.
func migrateStripPATFromOrigins(cfg *Config) error {
	root := strings.TrimSpace(cfg.Workspace.Root)
	if root == "" {
		// No workspace root configured; nothing to migrate.
		return nil
	}

	if _, err := os.Stat(root); os.IsNotExist(err) {
		// Workspace does not exist on disk yet; nothing to migrate.
		return nil
	}

	gitClient := git.NewClient()

	repoPaths, err := findGitRepos(root)
	if err != nil {
		return fmt.Errorf("scan workspace for git repos: %w", err)
	}

	var fixedCount, skippedCount int

	for _, repoPath := range repoPaths {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		fixed, err := stripPATFromOrigin(ctx, gitClient, repoPath)
		cancel()

		if err != nil {
			log.Printf("[migration strip_pat_origins] WARN  skipping %s: %v", repoPath, err)
			skippedCount++
			continue
		}
		if fixed {
			log.Printf("[migration strip_pat_origins] INFO  cleaned origin URL in %s", repoPath)
			fixedCount++
		}
	}

	if fixedCount > 0 || skippedCount > 0 {
		log.Printf("[migration strip_pat_origins] done: fixed=%d skipped=%d total=%d",
			fixedCount, skippedCount, len(repoPaths))
	}

	return nil
}

// stripPATFromOrigin reads the "origin" URL for a single repo and rewrites
// it if it contains an embedded credential.  Returns true when the URL was
// actually changed.
func stripPATFromOrigin(ctx context.Context, gitClient *git.Client, repoPath string) (bool, error) {
	currentURL, err := gitClient.GetRemoteURL(ctx, repoPath, "origin")
	if err != nil {
		// Remote "origin" may not exist – treat as a skip, not an error.
		return false, fmt.Errorf("get remote URL: %w", err)
	}

	cleanURL, changed := removeEmbeddedCredential(currentURL)
	if !changed {
		return false, nil
	}

	if err := gitClient.SetRemoteURL(ctx, repoPath, "origin", cleanURL); err != nil {
		return false, fmt.Errorf("set remote URL: %w", err)
	}

	return true, nil
}

// removeEmbeddedCredential removes the userinfo (PAT) portion from an
// https:// URL.
//
//	"https://ghp_TOKEN@github.com/owner/repo.git" → "https://github.com/owner/repo.git", true
//	"https://github.com/owner/repo.git"           → unchanged, false
func removeEmbeddedCredential(rawURL string) (string, bool) {
	if !strings.HasPrefix(rawURL, "https://") {
		return rawURL, false
	}

	// The userinfo is the segment between "https://" and the first "@".
	rest := rawURL[len("https://"):]
	atIdx := strings.Index(rest, "@")
	if atIdx < 0 {
		// No "@" — no embedded credential.
		return rawURL, false
	}

	// Sanity-check: the host part (after "@") must contain a dot (e.g.
	// "github.com"), so we don't accidentally strip a legitimate "org@"
	// prefix that is part of the hostname rather than a credential.
	hostPart := rest[atIdx+1:]
	if !strings.Contains(strings.SplitN(hostPart, "/", 2)[0], ".") {
		return rawURL, false
	}

	return "https://" + hostPart, true
}

// findGitRepos walks the given directory tree and returns the paths of all
// directories that contain a ".git" subdirectory (i.e. are git work trees).
// It does NOT descend into the ".git" directories themselves.
func findGitRepos(root string) ([]string, error) {
	var repos []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Log and continue rather than aborting the whole walk.
			log.Printf("[migration strip_pat_origins] WARN  cannot access %s: %v", path, err)
			return nil
		}

		if !d.IsDir() {
			return nil
		}

		// Skip hidden directories (including ".git" itself).
		if strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}

		// Check whether this directory is a git work tree.
		gitDir := filepath.Join(path, ".git")
		if info, statErr := os.Stat(gitDir); statErr == nil && info.IsDir() {
			repos = append(repos, path)
			// Don't descend into a repo's subdirectories — git sub-modules
			// are handled by their own top-level entry (if they live inside
			// the workspace).
			return filepath.SkipDir
		}

		return nil
	})

	return repos, err
}
