package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/maintenance"
)

func saveCfgWithMaintenanceWindows(t *testing.T, windows []config.MaintenanceWindow) string {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.Default()
	cfg.Workspace.Root = t.TempDir()
	cfg.Daemon.MaintenanceWindows = windows
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return configPath
}

func TestMaintenanceStatusCommand_NoWindows(t *testing.T) {
	t.Parallel()

	configPath := saveCfgWithMaintenanceWindows(t, nil)
	out, err := executeSyncctl("--config", configPath, "maintenance", "status")
	if err != nil {
		t.Fatalf("maintenance status failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "inactive") {
		t.Errorf("expected 'inactive' in output, got: %s", out)
	}
}

func TestMaintenanceStatusCommand_JSON_NoWindows(t *testing.T) {
	t.Parallel()

	configPath := saveCfgWithMaintenanceWindows(t, nil)
	out, err := executeSyncctl("--config", configPath, "maintenance", "status", "--format", "json")
	if err != nil {
		t.Fatalf("maintenance status --format json failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"active"`) {
		t.Errorf("expected JSON with 'active' key, got: %s", out)
	}
}

func TestMaintenanceListCommand_Empty(t *testing.T) {
	t.Parallel()

	configPath := saveCfgWithMaintenanceWindows(t, nil)
	out, err := executeSyncctl("--config", configPath, "maintenance", "list")
	if err != nil {
		t.Fatalf("maintenance list failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no maintenance windows") {
		t.Errorf("expected 'no maintenance windows' in output, got: %s", out)
	}
}

func TestMaintenanceListCommand_WithWindow(t *testing.T) {
	t.Parallel()

	windows := []config.MaintenanceWindow{
		{Name: "nightly", Days: []string{"monday", "tuesday"}, Start: "02:00", End: "04:00"},
	}
	configPath := saveCfgWithMaintenanceWindows(t, windows)
	out, err := executeSyncctl("--config", configPath, "maintenance", "list")
	if err != nil {
		t.Fatalf("maintenance list failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "nightly") {
		t.Errorf("expected window name 'nightly' in output, got: %s", out)
	}
	if !strings.Contains(out, "02:00") {
		t.Errorf("expected start time '02:00' in output, got: %s", out)
	}
}

func TestMaintenanceListCommand_JSON(t *testing.T) {
	t.Parallel()

	windows := []config.MaintenanceWindow{
		{Name: "deploy", Days: []string{"friday"}, Start: "22:00", End: "23:00"},
	}
	configPath := saveCfgWithMaintenanceWindows(t, windows)
	out, err := executeSyncctl("--config", configPath, "maintenance", "list", "--format", "json")
	if err != nil {
		t.Fatalf("maintenance list --format json failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"deploy"`) {
		t.Errorf("expected 'deploy' in JSON output, got: %s", out)
	}
}

func TestMaintenanceListCommand_InvalidFormat(t *testing.T) {
	t.Parallel()

	configPath := saveCfgWithMaintenanceWindows(t, nil)
	_, err := executeSyncctl("--config", configPath, "maintenance", "list", "--format", "xml")
	if err == nil {
		t.Error("expected error for invalid format")
	}
}

// Unit-level tests for the maintenance package helpers used by the scheduler.

func TestActiveWindow_SchedulerIntegration(t *testing.T) {
	t.Parallel()

	// Tuesday 14:30 UTC
	now := time.Date(2026, 1, 6, 14, 30, 0, 0, time.UTC)
	windows := []config.MaintenanceWindow{
		{Name: "mid-day", Days: []string{"tuesday"}, Start: "14:00", End: "16:00"},
	}
	mw, desc := maintenance.ActiveWindow(windows, now)
	if mw == nil {
		t.Fatal("expected maintenance window to be active")
	}
	if !strings.Contains(desc, "mid-day") {
		t.Errorf("description %q should contain window name", desc)
	}
}

func TestActiveWindow_OutsideWindow_SchedulerIntegration(t *testing.T) {
	t.Parallel()

	// Tuesday 17:00 — after the window
	now := time.Date(2026, 1, 6, 17, 0, 0, 0, time.UTC)
	windows := []config.MaintenanceWindow{
		{Name: "mid-day", Days: []string{"tuesday"}, Start: "14:00", End: "16:00"},
	}
	mw, _ := maintenance.ActiveWindow(windows, now)
	if mw != nil {
		t.Error("maintenance window should not be active outside its time range")
	}
}
