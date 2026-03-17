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
	coressh "github.com/basmulder03/git-project-sync/internal/core/ssh"
	"github.com/basmulder03/git-project-sync/internal/core/workspace"
)

// Engine handles repository cloning operations
type Engine struct {
	Git    *git.Client
	Config config.Config

	// SSHManager, when non-nil, is used to derive per-source SSH credentials.
	// If nil, SSH key management is disabled and the clone falls back to HTTPS.
	SSHManager *coressh.Manager
}

// NewEngine creates a new clone engine.
func NewEngine(cfg config.Config) *Engine {
	return &Engine{
		Git:    git.NewClient(),
		Config: cfg,
	}
}

// NewEngineWithSSH creates a clone engine that prefers SSH transport.
func NewEngineWithSSH(cfg config.Config, sshManager *coressh.Manager) *Engine {
	return &Engine{
		Git:        git.NewClient(),
		Config:     cfg,
		SSHManager: sshManager,
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
	// UsedSSH reports whether SSH transport was used for this clone.
	UsedSSH bool
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

	// Determine clone URL and SSH env to use.
	cloneURL, sshEnvKey, sshEnvVal, usedSSH := e.resolveCloneParams(repo)
	result.UsedSSH = usedSSH

	// Perform the clone
	if err := e.cloneRepo(ctx, cloneURL, targetPath, sshEnvKey, sshEnvVal); err != nil {
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

// resolveCloneParams determines the URL and optional GIT_SSH_COMMAND env
// variable for a clone.  SSH is preferred when:
//  1. The global ssh.enabled flag is true (default), AND
//  2. A per-source private key file exists in the SSH manager.
//
// If both conditions hold, the SSH clone URL (with alias) is used together
// with GIT_SSH_COMMAND so no credential manager or ssh-agent is needed.
// Otherwise we fall back to HTTPS.
func (e *Engine) resolveCloneParams(repo api.RemoteRepository) (cloneURL, sshEnvKey, sshEnvVal string, usedSSH bool) {
	// Find the source config for this repo.
	src := e.findSource(repo.SourceID)

	sshEnabled := e.Config.SSHEnabledForSource(src)

	if sshEnabled && e.SSHManager != nil && e.SSHManager.HasKey(repo.SourceID) && repo.SSHCloneURL != "" {
		envKey, envVal := e.SSHManager.GitEnv(repo.SourceID)
		return repo.SSHCloneURL, envKey, envVal, true
	}

	// Fallback: clean HTTPS URL (no embedded token — use git credential manager
	// or the token via the PAT migration strip path).
	return repo.CloneURL, "", "", false
}

// findSource looks up the source config for a given source ID.
// Returns an empty SourceConfig if not found.
func (e *Engine) findSource(sourceID string) config.SourceConfig {
	for _, src := range e.Config.Sources {
		if src.ID == sourceID {
			return src
		}
	}
	return config.SourceConfig{}
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

// cloneRepo performs the actual git clone operation.
// sshEnvKey/sshEnvVal, when non-empty, are added to the subprocess environment
// so that git uses the correct SSH key without relying on ssh-agent.
func (e *Engine) cloneRepo(ctx context.Context, cloneURL, targetPath, sshEnvKey, sshEnvVal string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", "--quiet", cloneURL, targetPath)

	// Inherit the current environment.
	cmd.Env = os.Environ()

	// Override / add GIT_SSH_COMMAND when SSH key is specified.
	if sshEnvKey != "" && sshEnvVal != "" {
		cmd.Env = setEnv(cmd.Env, sshEnvKey, sshEnvVal)
	}

	// Prevent git from interactively prompting for credentials.
	cmd.Env = setEnv(cmd.Env, "GIT_TERMINAL_PROMPT", "0")

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

// setEnv sets or replaces an environment variable in the slice.
func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, v := range env {
		if strings.HasPrefix(v, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
