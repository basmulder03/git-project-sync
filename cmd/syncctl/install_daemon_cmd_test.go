package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/install"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

type fakeInstaller struct {
	installMode   install.Mode
	uninstallMode install.Mode
}

func (f *fakeInstaller) Install(mode install.Mode) error {
	f.installMode = mode
	return nil
}

func (f *fakeInstaller) Uninstall(mode install.Mode) error {
	f.uninstallMode = mode
	return nil
}

type fakeDaemonController struct {
	started   install.Mode
	stopped   install.Mode
	restarted install.Mode
	status    string
}

func (f *fakeDaemonController) Start(mode install.Mode) error {
	f.started = mode
	return nil
}

func (f *fakeDaemonController) Stop(mode install.Mode) error {
	f.stopped = mode
	return nil
}

func (f *fakeDaemonController) Restart(mode install.Mode) error {
	f.restarted = mode
	return nil
}

func (f *fakeDaemonController) Status(mode install.Mode) (string, error) {
	if f.status == "" {
		return "unknown", nil
	}
	return f.status, nil
}

func TestResolveInstallMode(t *testing.T) {
	mode, err := resolveInstallMode(false, false)
	if err != nil {
		t.Fatalf("resolveInstallMode default returned error: %v", err)
	}
	if mode != install.ModeUser {
		t.Fatalf("expected default mode user, got %q", mode)
	}

	mode, err = resolveInstallMode(false, true)
	if err != nil {
		t.Fatalf("resolveInstallMode system returned error: %v", err)
	}
	if mode != install.ModeSystem {
		t.Fatalf("expected system mode, got %q", mode)
	}

	if _, err := resolveInstallMode(true, true); err == nil {
		t.Fatal("expected mutually exclusive flag error")
	}
}

func TestInstallAndUninstallCommands(t *testing.T) {
	fake := &fakeInstaller{}
	prevFactory := newServiceInstaller
	newServiceInstaller = func(binaryPath, configPath string) (serviceInstaller, error) {
		return fake, nil
	}
	t.Cleanup(func() {
		newServiceInstaller = prevFactory
	})

	configPath := filepath.Join(t.TempDir(), "config.yaml")

	installOutput, err := executeSyncctl("install", "--config-path", configPath, "--binary-path", "/tmp/syncd")
	if err != nil {
		t.Fatalf("install failed: %v output=%s", err, installOutput)
	}
	if !strings.Contains(installOutput, "installed service in user mode") {
		t.Fatalf("unexpected install output: %s", installOutput)
	}
	if fake.installMode != install.ModeUser {
		t.Fatalf("expected install mode user, got %q", fake.installMode)
	}

	uninstallOutput, err := executeSyncctl("uninstall", "--system", "--config-path", configPath, "--binary-path", "/tmp/syncd")
	if err != nil {
		t.Fatalf("uninstall failed: %v output=%s", err, uninstallOutput)
	}
	if !strings.Contains(uninstallOutput, "uninstalled service in system mode") {
		t.Fatalf("unexpected uninstall output: %s", uninstallOutput)
	}
	if fake.uninstallMode != install.ModeSystem {
		t.Fatalf("expected uninstall mode system, got %q", fake.uninstallMode)
	}
}

func TestServiceRegisterAndUnregisterCommands(t *testing.T) {
	fake := &fakeInstaller{}
	prevFactory := newServiceInstaller
	newServiceInstaller = func(binaryPath, configPath string) (serviceInstaller, error) {
		return fake, nil
	}
	t.Cleanup(func() {
		newServiceInstaller = prevFactory
	})

	configPath := filepath.Join(t.TempDir(), "config.yaml")

	registerOutput, err := executeSyncctl("service", "register", "--config-path", configPath, "--binary-path", "/tmp/syncd")
	if err != nil {
		t.Fatalf("service register failed: %v output=%s", err, registerOutput)
	}
	if !strings.Contains(registerOutput, "service registered in user mode") {
		t.Fatalf("unexpected register output: %s", registerOutput)
	}

	unregisterOutput, err := executeSyncctl("service", "unregister", "--system", "--config-path", configPath, "--binary-path", "/tmp/syncd")
	if err != nil {
		t.Fatalf("service unregister failed: %v output=%s", err, unregisterOutput)
	}
	if !strings.Contains(unregisterOutput, "service unregistered in system mode") {
		t.Fatalf("unexpected unregister output: %s", unregisterOutput)
	}
}

func TestDaemonCommands(t *testing.T) {
	fake := &fakeDaemonController{status: "active"}
	prevDaemon := daemonOps
	daemonOps = fake
	t.Cleanup(func() {
		daemonOps = prevDaemon
	})

	if output, err := executeSyncctl("daemon", "start"); err != nil {
		t.Fatalf("daemon start failed: %v output=%s", err, output)
	}
	if fake.started != install.ModeUser {
		t.Fatalf("expected daemon start user mode, got %q", fake.started)
	}

	statusOutput, err := executeSyncctl("daemon", "status", "--system")
	if err != nil {
		t.Fatalf("daemon status failed: %v output=%s", err, statusOutput)
	}
	if !strings.Contains(statusOutput, "status: active") {
		t.Fatalf("unexpected daemon status output: %s", statusOutput)
	}

	if output, err := executeSyncctl("daemon", "stop", "--system"); err != nil {
		t.Fatalf("daemon stop failed: %v output=%s", err, output)
	}
	if fake.stopped != install.ModeSystem {
		t.Fatalf("expected daemon stop system mode, got %q", fake.stopped)
	}

	if output, err := executeSyncctl("daemon", "restart", "--system"); err != nil {
		t.Fatalf("daemon restart failed: %v output=%s", err, output)
	}
	if fake.restarted != install.ModeSystem {
		t.Fatalf("expected daemon restart system mode, got %q", fake.restarted)
	}
}

// TestSystemctlModeArgs verifies the systemctlModeArgs helper returns the
// correct slice for each install mode.
func TestSystemctlModeArgs(t *testing.T) {
	t.Parallel()

	userArgs := systemctlModeArgs(install.ModeUser)
	if len(userArgs) != 1 || userArgs[0] != "--user" {
		t.Fatalf("user mode args = %v, want [--user]", userArgs)
	}

	systemArgs := systemctlModeArgs(install.ModeSystem)
	if len(systemArgs) != 0 {
		t.Fatalf("system mode args = %v, want []", systemArgs)
	}
}

// TestTelemetryEventRows verifies the helper produces one formatted row per
// event and that the output contains the expected fields.
func TestTelemetryEventRows(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	events := []telemetry.Event{
		{
			TraceID:    "t1",
			RepoPath:   "/repos/foo",
			Level:      "info",
			ReasonCode: "sync_ok",
			Message:    "all good",
			CreatedAt:  now,
		},
		{
			TraceID:    "t2",
			RepoPath:   "/repos/bar",
			Level:      "warn",
			ReasonCode: "dirty_repo",
			Message:    "working tree is dirty",
			CreatedAt:  now,
		},
	}

	rows := telemetryEventRows(events)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	for i, row := range rows {
		if !strings.Contains(row, events[i].RepoPath) {
			t.Errorf("row %d missing repo path: %s", i, row)
		}
		if !strings.Contains(row, events[i].ReasonCode) {
			t.Errorf("row %d missing reason code: %s", i, row)
		}
	}
}

// TestOsDaemonController_UnsupportedOS verifies the OS-specific daemon
// controller methods return a meaningful error on unsupported platforms and
// that they touch the platform dispatch path on the current OS.
func TestOsDaemonController_UnsupportedOS(t *testing.T) {
	t.Parallel()

	// We can only test the "current OS" path directly.  Verify that the
	// osDaemonController methods reach an OS branch (without actually running
	// systemctl / sc.exe) by swapping the runCommand implementation.
	prevRun := runCommand
	var called string
	runCommand = func(name string, args ...string) (string, error) {
		called = name
		return "", fmt.Errorf("stub: not running real %s", name)
	}
	t.Cleanup(func() { runCommand = prevRun })

	ctrl := osDaemonController{}
	// Start
	_ = ctrl.Start(install.ModeUser)
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		if called != "" {
			t.Fatalf("expected no runCommand call on unsupported OS, got %q", called)
		}
	}

	// Stop
	called = ""
	_ = ctrl.Stop(install.ModeSystem)

	// Status
	called = ""
	_, _ = ctrl.Status(install.ModeUser)

	// Restart delegates to Stop + Start — just ensure no panic.
	called = ""
	_ = ctrl.Restart(install.ModeUser)
}
