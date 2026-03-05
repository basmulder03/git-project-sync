package install

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestWindowsTaskInstallUserMode(t *testing.T) {
	t.Parallel()

	var calls []string
	installer := NewWindowsTaskSchedulerInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\schtasks.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
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
	installer.goos = "windows"
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\schtasks.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.isAdmin = func() bool { return false }

	err := installer.Install(ModeSystem)
	if err == nil {
		t.Fatal("expected permission error for non-admin system install")
	}
	var reasonErr *ReasonError
	if !errors.As(err, &reasonErr) {
		t.Fatalf("expected reason error, got %T", err)
	}
	if reasonErr.Code != ReasonInstallValidationFailed {
		t.Fatalf("unexpected code %q", reasonErr.Code)
	}
}

func TestWindowsTaskUninstallIdempotent(t *testing.T) {
	t.Parallel()

	installer := NewWindowsTaskSchedulerInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\schtasks.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.run = func(string, ...string) error { return nil }

	if err := installer.Uninstall(ModeUser); err != nil {
		t.Fatalf("uninstall user mode failed: %v", err)
	}
	if err := installer.Uninstall(ModeUser); err != nil {
		t.Fatalf("second uninstall should also succeed: %v", err)
	}
}

func TestWindowsTaskPreflightReportsUnsupportedEnvironment(t *testing.T) {
	t.Parallel()

	installer := NewWindowsTaskSchedulerInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "linux"
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\schtasks.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }

	findings := installer.Preflight(ModeUser)
	found := false
	for _, finding := range findings {
		if finding.Code == ReasonInstallUnsupportedEnvironment {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s finding, got %+v", ReasonInstallUnsupportedEnvironment, findings)
	}
}
