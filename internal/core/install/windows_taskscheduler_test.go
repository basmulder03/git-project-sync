package install

import (
	"strings"
	"testing"
)

func TestWindowsTaskInstallUserMode(t *testing.T) {
	t.Parallel()

	var calls []string
	installer := NewWindowsTaskSchedulerInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.run = func(name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}

	if err := installer.Install(ModeUser); err != nil {
		t.Fatalf("install user mode: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected create+query calls, got %d", len(calls))
	}
	if !strings.Contains(calls[0], "/Create") {
		t.Fatalf("first call should create task: %s", calls[0])
	}
}

func TestWindowsTaskInstallSystemModeRequiresAdmin(t *testing.T) {
	t.Parallel()

	installer := NewWindowsTaskSchedulerInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.isAdmin = func() bool { return false }

	if err := installer.Install(ModeSystem); err == nil {
		t.Fatal("expected permission error for non-admin system install")
	}
}

func TestWindowsTaskUninstallIdempotent(t *testing.T) {
	t.Parallel()

	installer := NewWindowsTaskSchedulerInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.run = func(string, ...string) error { return nil }

	if err := installer.Uninstall(ModeUser); err != nil {
		t.Fatalf("uninstall user mode failed: %v", err)
	}
	if err := installer.Uninstall(ModeUser); err != nil {
		t.Fatalf("second uninstall should also succeed: %v", err)
	}
}
