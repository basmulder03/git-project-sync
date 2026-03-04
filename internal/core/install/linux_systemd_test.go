package install

import (
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

	if len(calls) != 2 {
		t.Fatalf("expected two systemctl calls, got %d: %+v", len(calls), calls)
	}
	if !strings.Contains(calls[0], "--user daemon-reload") {
		t.Fatalf("unexpected first systemctl call: %s", calls[0])
	}
}

func TestLinuxSystemdInstallSystemModeRequiresRoot(t *testing.T) {
	t.Parallel()

	installer := NewLinuxSystemdInstaller("/usr/local/bin/syncd", "/tmp/config.yaml")
	installer.geteuid = func() int { return 1000 }

	if err := installer.Install(ModeSystem); err == nil {
		t.Fatal("expected permission error for non-root system install")
	}
}

func TestLinuxSystemdUninstallIsIdempotent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	serviceDir := filepath.Join(root, "user-systemd")
	installer := NewLinuxSystemdInstaller("/usr/local/bin/syncd", "/tmp/config.yaml")
	installer.UserServiceDir = serviceDir
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
