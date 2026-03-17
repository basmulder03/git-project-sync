package clone

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/providers/api"
	coressh "github.com/basmulder03/git-project-sync/internal/core/ssh"
)

func TestEngine_ValidatePreClone(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root: tmpDir,
		},
	}
	engine := NewEngine(cfg)

	tests := []struct {
		name    string
		setup   func() string
		wantErr bool
	}{
		{
			name: "non-existent path is valid",
			setup: func() string {
				return filepath.Join(tmpDir, "new-repo")
			},
			wantErr: false,
		},
		{
			name: "existing path is invalid",
			setup: func() string {
				existingPath := filepath.Join(tmpDir, "existing-repo")
				_ = os.MkdirAll(existingPath, 0o755)
				return existingPath
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetPath := tt.setup()
			err := engine.validatePreClone(targetPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("validatePreClone() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEngine_VerifyClone(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root: tmpDir,
		},
	}
	engine := NewEngine(cfg)

	tests := []struct {
		name    string
		setup   func() string
		wantErr bool
	}{
		{
			name: "valid git repository",
			setup: func() string {
				repoPath := filepath.Join(tmpDir, "valid-repo")
				_ = os.MkdirAll(repoPath, 0o755)
				// Initialize a real git repository
				ctx := context.Background()
				cmd := exec.CommandContext(ctx, "git", "init", repoPath)
				_ = cmd.Run()
				return repoPath
			},
			wantErr: false,
		},
		{
			name: "missing .git directory",
			setup: func() string {
				repoPath := filepath.Join(tmpDir, "no-git")
				_ = os.MkdirAll(repoPath, 0o755)
				return repoPath
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := tt.setup()
			err := engine.verifyClone(repoPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("verifyClone() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEngine_CloneRepository_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root: tmpDir,
		},
	}
	engine := NewEngine(cfg)

	repo := api.RemoteRepository{
		Provider: "github",
		Owner:    "testuser",
		Name:     "testrepo",
		CloneURL: "https://github.com/testuser/testrepo.git",
	}

	ctx := context.Background()
	result := engine.CloneRepository(ctx, repo, true)

	if !result.Success {
		t.Errorf("dry run should succeed, got error: %v", result.Error)
	}

	if result.ReasonCode != "dry_run" {
		t.Errorf("expected reason code 'dry_run', got: %s", result.ReasonCode)
	}

	// Verify nothing was actually cloned
	if _, err := os.Stat(result.TargetPath); err == nil {
		t.Error("dry run should not create target directory")
	}
}

func TestEngine_CloneRepository_PathAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root: tmpDir,
		},
	}
	engine := NewEngine(cfg)

	// Create existing directory
	existingPath := filepath.Join(tmpDir, "github", "testuser", "testrepo")
	_ = os.MkdirAll(existingPath, 0o755)

	repo := api.RemoteRepository{
		Provider: "github",
		Owner:    "testuser",
		Name:     "testrepo",
		CloneURL: "https://github.com/testuser/testrepo.git",
	}

	ctx := context.Background()
	result := engine.CloneRepository(ctx, repo, false)

	if result.Success {
		t.Error("clone should fail when path already exists")
	}

	if result.ReasonCode != "validation_failed" {
		t.Errorf("expected reason code 'validation_failed', got: %s", result.ReasonCode)
	}
}

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantRetry bool
	}{
		{
			name:      "nil error",
			err:       nil,
			wantRetry: false,
		},
		{
			name:      "connection error",
			err:       &os.PathError{Op: "dial", Path: "github.com", Err: os.ErrDeadlineExceeded},
			wantRetry: true,
		},
		{
			name:      "context canceled",
			err:       context.Canceled,
			wantRetry: false,
		},
		{
			name:      "context deadline exceeded",
			err:       context.DeadlineExceeded,
			wantRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransientError(tt.err)
			if got != tt.wantRetry {
				t.Errorf("isTransientError() = %v, want %v", got, tt.wantRetry)
			}
		})
	}
}

func TestEngine_CloneRepositories_Sequential(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root: tmpDir,
		},
	}
	engine := NewEngine(cfg)

	repos := []api.RemoteRepository{
		{
			Provider: "github",
			Owner:    "testuser",
			Name:     "repo1",
			CloneURL: "https://github.com/testuser/repo1.git",
		},
		{
			Provider: "github",
			Owner:    "testuser",
			Name:     "repo2",
			CloneURL: "https://github.com/testuser/repo2.git",
		},
	}

	ctx := context.Background()
	results := engine.CloneRepositories(ctx, repos, true)

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	for i, result := range results {
		if !result.Success {
			t.Errorf("result %d should succeed in dry run mode", i)
		}
	}
}

func TestEngine_CloneRepositories_CancellationHandling(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root: tmpDir,
		},
	}
	engine := NewEngine(cfg)

	repos := []api.RemoteRepository{
		{Provider: "github", Owner: "test", Name: "repo1"},
		{Provider: "github", Owner: "test", Name: "repo2"},
		{Provider: "github", Owner: "test", Name: "repo3"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	results := engine.CloneRepositories(ctx, repos, false)

	// Should get results for all repos (cancelled)
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	for _, result := range results {
		if result.Success {
			t.Error("cancelled operations should not succeed")
		}
		if result.ReasonCode != "cancelled" {
			t.Errorf("expected reason code 'cancelled', got: %s", result.ReasonCode)
		}
	}
}

func TestRetryConfig_Backoff(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root: tmpDir,
		},
	}
	engine := NewEngine(cfg)

	// Create a repo that will fail (invalid clone URL)
	repo := api.RemoteRepository{
		Provider: "github",
		Owner:    "invalid",
		Name:     "repo",
		CloneURL: "https://invalid.example.com/repo.git",
	}

	retryConfig := RetryConfig{
		MaxAttempts:        3,
		BaseBackoffSeconds: 1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := engine.CloneWithRetry(ctx, repo, retryConfig, false)
	elapsed := time.Since(start)

	// Should fail quickly due to context timeout
	if result.Success {
		t.Error("clone with invalid URL should fail")
	}

	// Should not take too long due to context timeout
	// Allow up to 500ms for Windows/CI environments with scheduling overhead
	if elapsed > 500*time.Millisecond {
		t.Errorf("retry took too long: %v (expected < 500ms)", elapsed)
	}
}

// --- SSH preference tests ---

func TestResolveCloneParams_SSHPreferred(t *testing.T) {
	sshDir := t.TempDir()

	// Generate a real SSH key so HasKey returns true.
	privPath := coressh.PrivateKeyPathForSource(sshDir, "github-acme")
	if _, err := coressh.GenerateKeyPair(privPath, coressh.KeyTypeEd25519, "test"); err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	mgr := coressh.NewManager(sshDir, "/tmp/ssh_config_unused", nil)

	cfg := config.Config{
		SSH: config.SSHConfig{Enabled: true},
		Sources: []config.SourceConfig{
			{ID: "github-acme", Provider: "github", Enabled: true},
		},
	}

	engine := NewEngineWithSSH(cfg, mgr)

	repo := api.RemoteRepository{
		SourceID:    "github-acme",
		Provider:    "github",
		Owner:       "acme",
		Name:        "myrepo",
		CloneURL:    "https://github.com/acme/myrepo.git",
		SSHCloneURL: "git@gps-github-acme:acme/myrepo.git",
	}

	cloneURL, envKey, _, usedSSH := engine.resolveCloneParams(repo)

	if !usedSSH {
		t.Error("expected SSH to be used when key exists and SSH is enabled")
	}
	if cloneURL != repo.SSHCloneURL {
		t.Errorf("expected SSH clone URL %q, got %q", repo.SSHCloneURL, cloneURL)
	}
	if envKey != "GIT_SSH_COMMAND" {
		t.Errorf("expected GIT_SSH_COMMAND env key, got %q", envKey)
	}
}

func TestResolveCloneParams_FallsBackToHTTPS_WhenSSHDisabled(t *testing.T) {
	sshDir := t.TempDir()

	// Generate a key — but SSH is globally disabled.
	privPath := coressh.PrivateKeyPathForSource(sshDir, "github-acme")
	if _, err := coressh.GenerateKeyPair(privPath, coressh.KeyTypeEd25519, "test"); err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	mgr := coressh.NewManager(sshDir, "/tmp/ssh_config_unused", nil)

	cfg := config.Config{
		SSH: config.SSHConfig{Enabled: false},
		Sources: []config.SourceConfig{
			{ID: "github-acme", Provider: "github", Enabled: true},
		},
	}

	engine := NewEngineWithSSH(cfg, mgr)

	repo := api.RemoteRepository{
		SourceID:    "github-acme",
		Provider:    "github",
		CloneURL:    "https://github.com/acme/myrepo.git",
		SSHCloneURL: "git@gps-github-acme:acme/myrepo.git",
	}

	_, _, _, usedSSH := engine.resolveCloneParams(repo)
	if usedSSH {
		t.Error("expected HTTPS fallback when SSH is globally disabled")
	}
}

func TestResolveCloneParams_FallsBackToHTTPS_WhenNoKey(t *testing.T) {
	sshDir := t.TempDir()
	// No key generated — manager has empty SSH dir.
	mgr := coressh.NewManager(sshDir, "/tmp/ssh_config_unused", nil)

	cfg := config.Config{
		SSH: config.SSHConfig{Enabled: true},
		Sources: []config.SourceConfig{
			{ID: "github-acme", Provider: "github", Enabled: true},
		},
	}

	engine := NewEngineWithSSH(cfg, mgr)

	repo := api.RemoteRepository{
		SourceID:    "github-acme",
		Provider:    "github",
		CloneURL:    "https://github.com/acme/myrepo.git",
		SSHCloneURL: "git@gps-github-acme:acme/myrepo.git",
	}

	cloneURL, _, _, usedSSH := engine.resolveCloneParams(repo)
	if usedSSH {
		t.Error("expected HTTPS fallback when no SSH key exists")
	}
	if cloneURL != repo.CloneURL {
		t.Errorf("expected HTTPS clone URL, got %q", cloneURL)
	}
}

func TestResolveCloneParams_FallsBackToHTTPS_WhenNoSSHCloneURL(t *testing.T) {
	sshDir := t.TempDir()

	privPath := coressh.PrivateKeyPathForSource(sshDir, "github-acme")
	if _, err := coressh.GenerateKeyPair(privPath, coressh.KeyTypeEd25519, "test"); err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	mgr := coressh.NewManager(sshDir, "/tmp/ssh_config_unused", nil)

	cfg := config.Config{
		SSH: config.SSHConfig{Enabled: true},
		Sources: []config.SourceConfig{
			{ID: "github-acme", Provider: "github", Enabled: true},
		},
	}

	engine := NewEngineWithSSH(cfg, mgr)

	// SSHCloneURL is empty — e.g. for an older repo entry without it.
	repo := api.RemoteRepository{
		SourceID:    "github-acme",
		Provider:    "github",
		CloneURL:    "https://github.com/acme/myrepo.git",
		SSHCloneURL: "",
	}

	_, _, _, usedSSH := engine.resolveCloneParams(repo)
	if usedSSH {
		t.Error("expected HTTPS fallback when SSHCloneURL is empty")
	}
}

func TestSetEnv_AddsNew(t *testing.T) {
	env := []string{"FOO=bar"}
	result := setEnv(env, "GIT_SSH_COMMAND", "ssh -i /key")
	found := false
	for _, v := range result {
		if v == "GIT_SSH_COMMAND=ssh -i /key" {
			found = true
		}
	}
	if !found {
		t.Errorf("setEnv did not add new variable: %v", result)
	}
}

func TestSetEnv_Replaces(t *testing.T) {
	env := []string{"GIT_SSH_COMMAND=old-value", "OTHER=x"}
	result := setEnv(env, "GIT_SSH_COMMAND", "new-value")
	for _, v := range result {
		if strings.HasPrefix(v, "GIT_SSH_COMMAND=") && v != "GIT_SSH_COMMAND=new-value" {
			t.Errorf("setEnv did not replace value: got %q", v)
		}
	}
	count := 0
	for _, v := range result {
		if strings.HasPrefix(v, "GIT_SSH_COMMAND=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one GIT_SSH_COMMAND, got %d", count)
	}
}
