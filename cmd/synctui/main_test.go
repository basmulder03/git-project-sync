package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	"github.com/basmulder03/git-project-sync/internal/core/state"
)

func TestRunCommandRepoList(t *testing.T) {
	t.Parallel()

	exec := testActionExecutor(t)
	out, err := exec.runCommand(context.Background(), "repo list")
	if err != nil {
		t.Fatalf("runCommand repo list failed: %v", err)
	}
	if !strings.Contains(out, "repos configured") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunCommandStateCheck(t *testing.T) {
	t.Parallel()

	exec := testActionExecutor(t)
	out, err := exec.runCommand(context.Background(), "state check")
	if err != nil {
		t.Fatalf("runCommand state check failed: %v", err)
	}
	if !strings.Contains(out, "state integrity ok") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunCommandUnsupported(t *testing.T) {
	t.Parallel()

	exec := testActionExecutor(t)
	if _, err := exec.runCommand(context.Background(), "source add"); err == nil {
		t.Fatal("expected unsupported command error")
	}
}

func TestRunCommandDoctor(t *testing.T) {
	t.Parallel()

	exec := testActionExecutor(t)
	out, err := exec.runCommand(context.Background(), "doctor")
	if err != nil {
		t.Fatalf("runCommand doctor failed: %v", err)
	}
	if !strings.Contains(out, "doctor:") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunCommandDiscover(t *testing.T) {
	t.Parallel()

	exec := testActionExecutor(t)
	out, err := exec.runCommand(context.Background(), "discover")
	if err != nil {
		t.Fatalf("runCommand discover failed: %v", err)
	}
	if !strings.Contains(out, "discover:") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunCommandMaintenanceStatus(t *testing.T) {
	t.Parallel()

	exec := testActionExecutor(t)
	out, err := exec.runCommand(context.Background(), "maintenance status")
	if err != nil {
		t.Fatalf("runCommand maintenance status failed: %v", err)
	}
	if !strings.Contains(out, "maintenance status:") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func testActionExecutor(t *testing.T) *actionExecutor {
	t.Helper()

	cfg := config.Default()
	cfg.State.DBPath = filepath.Join(t.TempDir(), "state.db")
	cfg.Repos = append(cfg.Repos, config.RepoConfig{Path: "/tmp/repo-1", Enabled: true})

	store, err := state.NewSQLiteStore(cfg.State.DBPath)
	if err != nil {
		t.Fatalf("open state db: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	return &actionExecutor{
		cfg: cfg,
		api: daemon.NewServiceAPI(store),
	}
}
