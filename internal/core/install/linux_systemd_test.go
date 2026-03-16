package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxSystemdInstallUserMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	serviceDir := filepath.Join(root, "user-systemd")
	binaryPath := "/usr/local/bin/syncd"
	configPath := "/tmp/config.yaml"

	var calls []string
	installer := NewLinuxSystemdInstaller(binaryPath, configPath)
	installer.UserServiceDir = serviceDir
	installer.goos = "linux"
	installer.lookPath = func(string) (string, error) { return "/usr/bin/systemctl", nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.run = func(name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}

	if err := installer.Install(ModeUser); err != nil {
		t.Fatalf("install user mode: %v", err)
	}

	servicePath := filepath.Join(serviceDir, installer.unitName())
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("read written service file: %v", err)
	}
	if !strings.Contains(string(content), binaryPath) {
		t.Fatalf("service file missing binary path: %s", string(content))
	}

	// Calls: migration (disable timer, disable oneshot, daemon-reload) +
	// install (daemon-reload, enable --now) = 5 total.
	if len(calls) < 5 {
		t.Fatalf("expected at least 5 systemctl calls (migration+install), got %d: %+v", len(calls), calls)
	}

	// The last two calls must be daemon-reload then enable --now.
	enableCall := calls[len(calls)-1]
	if !strings.Contains(enableCall, "enable") || !strings.Contains(enableCall, "--now") {
		t.Fatalf("last call should be systemctl enable --now, got: %s", enableCall)
	}
	reloadCall := calls[len(calls)-2]
	if !strings.Contains(reloadCall, "daemon-reload") {
		t.Fatalf("second-to-last call should be systemctl daemon-reload, got: %s", reloadCall)
	}
}

func TestLinuxSystemdInstallSystemModeRequiresRoot(t *testing.T) {
	t.Parallel()

	installer := NewLinuxSystemdInstaller("/usr/local/bin/syncd", "/tmp/config.yaml")
	installer.goos = "linux"
	installer.lookPath = func(string) (string, error) { return "/usr/bin/systemctl", nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.geteuid = func() int { return 1000 }

	err := installer.Install(ModeSystem)
	if err == nil {
		t.Fatal("expected permission error for non-root system install")
	}
	var reasonErr *ReasonError
	if !errors.As(err, &reasonErr) {
		t.Fatalf("expected reason error, got %T", err)
	}
	if reasonErr.Code != ReasonInstallValidationFailed {
		t.Fatalf("unexpected code %q", reasonErr.Code)
	}
}

func TestLinuxSystemdUninstallIsIdempotent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	serviceDir := filepath.Join(root, "user-systemd")
	installer := NewLinuxSystemdInstaller("/usr/local/bin/syncd", "/tmp/config.yaml")
	installer.UserServiceDir = serviceDir
	installer.goos = "linux"
	installer.lookPath = func(string) (string, error) { return "/usr/bin/systemctl", nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.run = func(string, ...string) error { return nil }

	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("mkdir service dir: %v", err)
	}
	servicePath := filepath.Join(serviceDir, installer.unitName())
	if err := os.WriteFile(servicePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write service file: %v", err)
	}

	if err := installer.Uninstall(ModeUser); err != nil {
		t.Fatalf("uninstall first run: %v", err)
	}
	if err := installer.Uninstall(ModeUser); err != nil {
		t.Fatalf("uninstall second run should be idempotent: %v", err)
	}
}

func TestLinuxSystemdPreflightReportsMissingDependency(t *testing.T) {
	t.Parallel()

	installer := NewLinuxSystemdInstaller("/usr/local/bin/syncd", "/tmp/config.yaml")
	installer.goos = "linux"
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.lookPath = func(string) (string, error) { return "", os.ErrNotExist }

	findings := installer.Preflight(ModeUser)
	found := false
	for _, finding := range findings {
		if finding.Code == ReasonInstallDependencyMissing {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s finding, got %+v", ReasonInstallDependencyMissing, findings)
	}
}

func TestLinuxSystemdInstallMigratesLegacyTimer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	serviceDir := filepath.Join(root, "user-systemd")
	var calls []string
	installer := NewLinuxSystemdInstaller("/usr/local/bin/syncd", "/tmp/config.yaml")
	installer.UserServiceDir = serviceDir
	installer.goos = "linux"
	installer.lookPath = func(string) (string, error) { return "/usr/bin/systemctl", nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.run = func(name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}

	if err := installer.Install(ModeUser); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Verify that the timer disable call is present in the migration phase.
	timerDisabled := false
	for _, c := range calls {
		if strings.Contains(c, "disable") && strings.Contains(c, ".timer") {
			timerDisabled = true
			break
		}
	}
	if !timerDisabled {
		t.Fatalf("expected systemctl disable .timer migration call, calls: %+v", calls)
	}
}
