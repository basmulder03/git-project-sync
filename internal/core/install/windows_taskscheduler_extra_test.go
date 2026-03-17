package install

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsTaskInstallSystemMode(t *testing.T) {
	t.Parallel()

	var calls []string
	installer := NewWindowsTaskSchedulerInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.isAdmin = func() bool { return true }
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\schtasks.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.run = func(name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}

	if err := installer.Install(ModeSystem); err != nil {
		t.Fatalf("install system mode: %v", err)
	}

	// System mode should include /RL HIGHEST /RU SYSTEM.
	createCall := calls[0]
	if !strings.Contains(createCall, "SYSTEM") {
		t.Fatalf("system mode should include SYSTEM user, got: %s", createCall)
	}
}

func TestWindowsTaskPreflightMissingBinaryPath(t *testing.T) {
	t.Parallel()

	installer := NewWindowsTaskSchedulerInstaller("", `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\schtasks.exe`, nil }

	findings := installer.Preflight(ModeUser)
	found := false
	for _, f := range findings {
		if f.Code == ReasonInstallMissingBinaryPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s finding, got: %+v", ReasonInstallMissingBinaryPath, findings)
	}
}

func TestWindowsTaskPreflightMissingConfigPath(t *testing.T) {
	t.Parallel()

	installer := NewWindowsTaskSchedulerInstaller(`C:\tools\syncd.exe`, "")
	installer.goos = "windows"
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\schtasks.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }

	findings := installer.Preflight(ModeUser)
	found := false
	for _, f := range findings {
		if f.Code == ReasonInstallMissingConfigPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s finding, got: %+v", ReasonInstallMissingConfigPath, findings)
	}
}

func TestWindowsTaskPreflightBinaryMissing(t *testing.T) {
	t.Parallel()

	installer := NewWindowsTaskSchedulerInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\schtasks.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }

	findings := installer.Preflight(ModeUser)
	found := false
	for _, f := range findings {
		if f.Code == ReasonInstallBinaryMissing {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s finding, got: %+v", ReasonInstallBinaryMissing, findings)
	}
}

func TestWindowsTaskPreflightMissingDependency(t *testing.T) {
	t.Parallel()

	installer := NewWindowsTaskSchedulerInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.lookPath = func(string) (string, error) { return "", os.ErrNotExist }

	findings := installer.Preflight(ModeUser)
	found := false
	for _, f := range findings {
		if f.Code == ReasonInstallDependencyMissing {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s finding, got: %+v", ReasonInstallDependencyMissing, findings)
	}
}

func TestWindowsTaskPreflightInvalidMode(t *testing.T) {
	t.Parallel()

	installer := NewWindowsTaskSchedulerInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\schtasks.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }

	findings := installer.Preflight(Mode("bogus"))
	found := false
	for _, f := range findings {
		if f.Code == ReasonInstallInvalidMode {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s finding, got: %+v", ReasonInstallInvalidMode, findings)
	}
}
