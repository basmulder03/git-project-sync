package install

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestWindowsServiceInstallUserMode(t *testing.T) {
	t.Parallel()

	var calls [][]string
	installer := NewWindowsServiceInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.isAdmin = func() bool { return true }
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\sc.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.run = func(name string, args ...string) (string, error) {
		calls = append(calls, append([]string{name}, args...))
		// Simulate no legacy task scheduler job – schtasks /Query fails.
		if name == "schtasks" {
			return "", errors.New("task not found")
		}
		return "", nil
	}

	if err := installer.Install(ModeUser); err != nil {
		t.Fatalf("install user mode: %v", err)
	}

	// Expect: schtasks /Query (migration probe), then sc.exe create, description, start, query.
	if len(calls) < 3 {
		t.Fatalf("expected at least migration+create+start+query calls, got %d: %+v", len(calls), calls)
	}

	// Find the sc.exe create call (may not be the first due to migration probe).
	createIdx := -1
	for i, c := range calls {
		if strings.Join(c, " ") != "" && strings.Contains(strings.Join(c, " "), "create") && c[0] == "sc.exe" {
			createIdx = i
			break
		}
	}
	if createIdx == -1 {
		t.Fatalf("no sc.exe create call found in: %+v", calls)
	}
	createCall := strings.Join(calls[createIdx], " ")
	if !strings.Contains(createCall, "GitProjectSync") {
		t.Fatalf("sc.exe create call should reference service name, got: %s", createCall)
	}
}

func TestWindowsServiceInstallSystemMode(t *testing.T) {
	t.Parallel()

	var calls [][]string
	installer := NewWindowsServiceInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.isAdmin = func() bool { return true }
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\sc.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.run = func(name string, args ...string) (string, error) {
		calls = append(calls, append([]string{name}, args...))
		// Simulate no legacy task scheduler job.
		if name == "schtasks" {
			return "", errors.New("task not found")
		}
		return "", nil
	}

	if err := installer.Install(ModeSystem); err != nil {
		t.Fatalf("install system mode: %v", err)
	}

	// Find the sc.exe create call and verify it contains LocalService.
	createFound := false
	for _, c := range calls {
		joined := strings.Join(c, " ")
		if c[0] == "sc.exe" && strings.Contains(joined, "create") {
			if !strings.Contains(joined, "LocalService") {
				t.Fatalf("system mode sc.exe create should specify LocalService, got: %s", joined)
			}
			createFound = true
			break
		}
	}
	if !createFound {
		t.Fatalf("no sc.exe create call found in: %+v", calls)
	}
}

func TestWindowsServiceInstallRequiresAdmin(t *testing.T) {
	t.Parallel()

	installer := NewWindowsServiceInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.isAdmin = func() bool { return false }
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\sc.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }

	err := installer.Install(ModeUser)
	if err == nil {
		t.Fatal("expected error for non-admin install")
	}
	var reasonErr *ReasonError
	if !errors.As(err, &reasonErr) {
		t.Fatalf("expected ReasonError, got %T", err)
	}
	if reasonErr.Code != ReasonInstallValidationFailed {
		t.Fatalf("unexpected code %q", reasonErr.Code)
	}
}

func TestWindowsServiceUninstallIdempotent(t *testing.T) {
	t.Parallel()

	installer := NewWindowsServiceInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.isAdmin = func() bool { return true }
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\sc.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.run = func(string, ...string) (string, error) { return "", nil }

	if err := installer.Uninstall(ModeUser); err != nil {
		t.Fatalf("first uninstall: %v", err)
	}
	if err := installer.Uninstall(ModeUser); err != nil {
		t.Fatalf("second uninstall should be idempotent: %v", err)
	}
}

func TestWindowsServicePreflightUnsupportedOS(t *testing.T) {
	t.Parallel()

	installer := NewWindowsServiceInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "linux"
	installer.isAdmin = func() bool { return false }
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\sc.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }

	findings := installer.Preflight(ModeUser)
	found := false
	for _, f := range findings {
		if f.Code == ReasonInstallUnsupportedEnvironment {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s finding, got %+v", ReasonInstallUnsupportedEnvironment, findings)
	}
}

func TestWindowsServicePreflightMissingScExe(t *testing.T) {
	t.Parallel()

	installer := NewWindowsServiceInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.isAdmin = func() bool { return true }
	installer.lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }

	findings := installer.Preflight(ModeUser)
	found := false
	for _, f := range findings {
		if f.Code == ReasonInstallDependencyMissing {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s finding, got %+v", ReasonInstallDependencyMissing, findings)
	}
}

func TestWindowsServiceInstallMigratesLegacyTask(t *testing.T) {
	t.Parallel()

	var calls [][]string
	installer := NewWindowsServiceInstaller(`C:\tools\syncd.exe`, `C:\cfg\config.yaml`)
	installer.goos = "windows"
	installer.isAdmin = func() bool { return true }
	installer.lookPath = func(string) (string, error) { return `C:\Windows\System32\sc.exe`, nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.run = func(name string, args ...string) (string, error) {
		calls = append(calls, append([]string{name}, args...))
		// Simulate a legacy task scheduler job present (schtasks /Query succeeds).
		// schtasks /Delete should also succeed.
		return "", nil
	}

	if err := installer.Install(ModeUser); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Verify that schtasks /Delete was called (migration happened).
	deleteFound := false
	for _, c := range calls {
		if c[0] == "schtasks" && len(c) >= 2 && c[1] == "/Delete" {
			deleteFound = true
			break
		}
	}
	if !deleteFound {
		t.Fatalf("expected schtasks /Delete migration call, calls: %+v", calls)
	}
}
