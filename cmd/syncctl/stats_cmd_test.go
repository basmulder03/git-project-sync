package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func saveMinimalConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.Workspace.Root = t.TempDir()
	// Use a per-test DB path to prevent SQLITE_BUSY when parallel tests
	// share the same process and default DB path.
	cfg.State.DBPath = filepath.Join(dir, "state.db")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return configPath
}

func TestStatsExportCommand_Prometheus(t *testing.T) {
	t.Parallel()

	configPath := saveMinimalConfig(t)
	out, err := executeSyncctl("--config", configPath, "stats", "export", "--format", "prometheus")
	if err != nil {
		t.Fatalf("stats export --format prometheus failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "gps_events_total") {
		t.Errorf("expected 'gps_events_total' in Prometheus output, got: %s", out)
	}
	if !strings.Contains(out, "# HELP") {
		t.Errorf("expected '# HELP' in Prometheus output, got: %s", out)
	}
}

func TestStatsExportCommand_OpenMetrics(t *testing.T) {
	t.Parallel()

	configPath := saveMinimalConfig(t)
	out, err := executeSyncctl("--config", configPath, "stats", "export", "--format", "openmetrics")
	if err != nil {
		t.Fatalf("stats export --format openmetrics failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "# EOF") {
		t.Errorf("expected '# EOF' in OpenMetrics output, got: %s", out)
	}
}

func TestStatsExportCommand_JSON(t *testing.T) {
	t.Parallel()

	configPath := saveMinimalConfig(t)
	out, err := executeSyncctl("--config", configPath, "stats", "export", "--format", "json")
	if err != nil {
		t.Fatalf("stats export --format json failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "HealthScore") {
		t.Errorf("expected 'HealthScore' in JSON output, got: %s", out)
	}
}

func TestStatsExportCommand_DefaultIsPrometheus(t *testing.T) {
	t.Parallel()

	configPath := saveMinimalConfig(t)
	out, err := executeSyncctl("--config", configPath, "stats", "export")
	if err != nil {
		t.Fatalf("stats export (default) failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "gps_health_score") {
		t.Errorf("expected Prometheus output by default, got: %s", out)
	}
}

func TestStatsExportCommand_InvalidFormat(t *testing.T) {
	t.Parallel()

	configPath := saveMinimalConfig(t)
	_, err := executeSyncctl("--config", configPath, "stats", "export", "--format", "graphite")
	if err == nil {
		t.Error("expected error for invalid format")
	}
}
