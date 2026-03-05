package integration

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/state"
)

func TestAuditExportsJSONAndCSVAreAvailable(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	statePath := filepath.Join(t.TempDir(), "state.db")

	cfg := config.Default()
	cfg.State.DBPath = statePath
	cfg.Repos = []config.RepoConfig{{Path: "/repos/a", SourceID: "gh1", Enabled: true}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store, err := state.NewSQLiteStore(statePath)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	_ = store.AppendEvent(state.Event{TraceID: "trace-audit", RepoPath: "/repos/a", Level: "warn", ReasonCode: "repo_locked", Message: "token=abcd1234", CreatedAt: time.Now().UTC()})

	cmd := exec.Command("go", "run", "./cmd/syncctl", "--config", configPath, "events", "list", "--format", "json", "--source-id", "gh1")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("events export failed: %v (%s)", err, string(out))
	}
	if !strings.Contains(string(out), "\"trace_id\": \"trace-audit\"") || !strings.Contains(string(out), "[redacted]") {
		t.Fatalf("unexpected events export output: %s", string(out))
	}

	cmd = exec.Command("go", "run", "./cmd/syncctl", "--config", configPath, "stats", "show", "--format", "csv")
	cmd.Dir = root
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("stats csv export failed: %v (%s)", err, string(out))
	}
	if !strings.Contains(string(out), "metric,value") {
		t.Fatalf("stats csv output missing header: %s", string(out))
	}
}
