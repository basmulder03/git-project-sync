package clone

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/git"
	"github.com/basmulder03/git-project-sync/internal/core/providers/api"
	"github.com/basmulder03/git-project-sync/internal/core/workspace"
)

// Engine handles repository cloning operations
type Engine struct {
	Git    *git.Client
	Config config.Config
}

// NewEngine creates a new clone engine
func NewEngine(cfg config.Config) *Engine {
	return &Engine{
		Git:    git.NewClient(),
		Config: cfg,
	}
}

// CloneResult represents the outcome of a clone operation
type CloneResult struct {
	Repo       api.RemoteRepository
	TargetPath string
	Success    bool
	Error      error
	Duration   time.Duration
	ReasonCode string
}

// CloneRepository clones a single repository to the workspace
func (e *Engine) CloneRepository(ctx context.Context, repo api.RemoteRepository, dryRun bool) CloneResult {
	start := time.Now()

	result := CloneResult{
		Repo: repo,
	}

	// Calculate target path using workspace layout
	layout := workspace.NewLayout(e.Config.Workspace)
	targetPath := layout.RepoPath(repo.Provider, repo.Owner, repo.Name)
	result.TargetPath = targetPath

	// Pre-flight validations
	if err := e.validatePreClone(targetPath); err != nil {
		result.Error = err
		result.ReasonCode = "validation_failed"
		result.Duration = time.Since(start)
		return result
	}

	// Check disk space
	if err := e.checkDiskSpace(targetPath); err != nil {
		result.Error = err
		result.ReasonCode = "insufficient_disk_space"
		result.Duration = time.Since(start)
		return result
	}

	// Dry run mode - just simulate
	if dryRun {
		result.Success = true
		result.ReasonCode = "dry_run"
		result.Duration = time.Since(start)
		return result
	}

	// Create parent directories
	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		result.Error = fmt.Errorf("create parent directory: %w", err)
		result.ReasonCode = "mkdir_failed"
		result.Duration = time.Since(start)
		return result
	}

	// Perform the clone
	if err := e.cloneRepo(ctx, repo.CloneURL, targetPath); err != nil {
		// Cleanup on failure
		_ = os.RemoveAll(targetPath)
		result.Error = err
		result.ReasonCode = "clone_failed"
		result.Duration = time.Since(start)
		return result
	}

	// Verify clone was successful
	if err := e.verifyClone(targetPath); err != nil {
		// Cleanup invalid clone
		_ = os.RemoveAll(targetPath)
		result.Error = err
		result.ReasonCode = "verify_failed"
		result.Duration = time.Since(start)
		return result
	}

	result.Success = true
	result.ReasonCode = "clone_completed"
	result.Duration = time.Since(start)
	return result
}

// CloneRepositories clones multiple repositories sequentially
func (e *Engine) CloneRepositories(ctx context.Context, repos []api.RemoteRepository, dryRun bool) []CloneResult {
	results := make([]CloneResult, 0, len(repos))

	for _, repo := range repos {
		// Check context cancellation
		select {
		case <-ctx.Done():
			// Add cancelled results for remaining repos
			for i := len(results); i < len(repos); i++ {
				results = append(results, CloneResult{
					Repo:       repos[i],
					Success:    false,
					Error:      ctx.Err(),
					ReasonCode: "cancelled",
				})
			}
			return results
		default:
		}

		result := e.CloneRepository(ctx, repo, dryRun)
		results = append(results, result)

		// If clone failed permanently, continue with others
		// Transient errors should be retried by the caller
	}

	return results
}

// validatePreClone performs pre-clone validation checks
func (e *Engine) validatePreClone(targetPath string) error {
	// Check if path already exists
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("target path already exists: %s", targetPath)
	}

	// Check if parent directory is writable
	parentDir := filepath.Dir(targetPath)
	if info, err := os.Stat(parentDir); err != nil {
		// Parent doesn't exist yet - that's okay, we'll create it
		return nil
	} else if !info.IsDir() {
		return fmt.Errorf("parent path is not a directory: %s", parentDir)
	}

	return nil
}

// checkDiskSpace checks if there's sufficient disk space for cloning
func (e *Engine) checkDiskSpace(targetPath string) error {
	// Get the root directory for disk space check
	root := e.Config.Workspace.Root
	if root == "" {
		root = filepath.VolumeName(targetPath)
		if root == "" {
			root = "/"
		}
	}

	// Check available disk space
	// Note: This is a simplified check. In production, you'd want to use
	// platform-specific APIs (syscall.Statfs on Unix, GetDiskFreeSpaceEx on Windows)
	// For now, we'll just verify the root exists
	if _, err := os.Stat(root); err != nil {
		return fmt.Errorf("cannot access workspace root: %w", err)
	}

	// TODO: Implement actual disk space check using syscall
	// For now, we'll assume there's enough space

	return nil
}

// cloneRepo performs the actual git clone operation
func (e *Engine) cloneRepo(ctx context.Context, cloneURL, targetPath string) error {
	// Use git.Client's run method indirectly by creating a temporary wrapper
	// Since we need to clone to a new directory, we'll use exec directly here
	cmd := exec.CommandContext(ctx, "git", "clone", "--quiet", cloneURL, targetPath)

	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	return nil
}

// verifyClone verifies that the clone was successful
func (e *Engine) verifyClone(repoPath string) error {
	// Check if .git directory exists
	gitDir := filepath.Join(repoPath, ".git")
	if info, err := os.Stat(gitDir); err != nil {
		return fmt.Errorf("git directory not found: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("git path is not a directory: %s", gitDir)
	}

	// Verify we can read git config
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := e.Git.DirtyState(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("cannot read git state: %w", err)
	}

	return nil
}
