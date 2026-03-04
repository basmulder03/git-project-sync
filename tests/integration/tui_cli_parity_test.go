package integration

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/ui/tui"
)

func TestTUIActionsHaveCLIParity(t *testing.T) {
	t.Parallel()

	status := tui.DashboardStatus{Events: []tui.EventRow{{TraceID: "trace-1"}}}
	actions := map[string]string{
		"s": "sync all",
		"c": "cache refresh all",
		"t": "trace show trace-1",
	}

	for key, cli := range actions {
		if _, ok := tui.KeyToAction(key, status); !ok {
			t.Fatalf("expected TUI key %q to map to action with CLI equivalent %q", key, cli)
		}
	}
}

func TestCLIParityCommandsAreRoutable(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(configPath, config.Default()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	for _, args := range [][]string{
		{"--config", configPath, "sync", "all", "--dry-run"},
		{"--config", configPath, "cache", "refresh", "all"},
		{"--config", configPath, "trace", "show", "trace-1"},
	} {
		cmd := exec.Command("go", append([]string{"run", "./cmd/syncctl"}, args...)...)
		cmd.Dir = root
		out := &bytes.Buffer{}
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Run(); err != nil {
			t.Fatalf("command failed args=%v err=%v output=%s", args, err, out.String())
		}
	}

}
