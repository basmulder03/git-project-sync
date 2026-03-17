//go:build !windows

package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxSystemdInstallSystemModeAsRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	systemDir := filepath.Join(root, "systemd-system")
	binaryPath := "/usr/local/bin/syncd"
	configPath := "/tmp/config.yaml"

	installer := NewLinuxSystemdInstaller(binaryPath, configPath)
	installer.SystemServiceDir = systemDir
	installer.goos = "linux"
	installer.lookPath = func(string) (string, error) { return "/usr/bin/systemctl", nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.geteuid = func() int { return 0 }
	installer.run = func(string, ...string) error { return nil }

	if err := installer.Install(ModeSystem); err != nil {
		t.Fatalf("install system mode as root: %v", err)
	}

	servicePath := filepath.Join(systemDir, installer.unitName())
	if _, err := os.Stat(servicePath); err != nil {
		t.Fatalf("service file not written: %v", err)
	}
}

func TestLinuxSystemdUninstallSystemMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	systemDir := filepath.Join(root, "systemd-system")
	if err := os.MkdirAll(systemDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	installer := NewLinuxSystemdInstaller("/usr/local/bin/syncd", "/tmp/config.yaml")
	installer.SystemServiceDir = systemDir
	installer.goos = "linux"
	installer.lookPath = func(string) (string, error) { return "/usr/bin/systemctl", nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }
	installer.geteuid = func() int { return 0 }
	installer.run = func(string, ...string) error { return nil }

	// Write a service file to remove.
	servicePath := filepath.Join(systemDir, installer.unitName())
	if err := os.WriteFile(servicePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write service file: %v", err)
	}

	if err := installer.Uninstall(ModeSystem); err != nil {
		t.Fatalf("uninstall system mode: %v", err)
	}
}

func TestLinuxSystemdServicePathHomedir(t *testing.T) {
	t.Parallel()

	installer := NewLinuxSystemdInstaller("/usr/local/bin/syncd", "/tmp/config.yaml")
	// No UserServiceDir set – should fall through to homedir.
	installer.goos = "linux"
	installer.homedir = func() (string, error) { return "/home/testuser", nil }

	path, flags, err := installer.servicePath(ModeUser)
	if err != nil {
		t.Fatalf("servicePath: %v", err)
	}
	if !strings.Contains(path, "/home/testuser") {
		t.Fatalf("expected path under home, got: %s", path)
	}
	if len(flags) == 0 || flags[0] != "--user" {
		t.Fatalf("expected --user flag, got: %v", flags)
	}
}

func TestLinuxSystemdServicePathHomedirError(t *testing.T) {
	t.Parallel()

	installer := NewLinuxSystemdInstaller("/usr/local/bin/syncd", "/tmp/config.yaml")
	installer.goos = "linux"
	installer.homedir = func() (string, error) { return "", errors.New("no home") }

	_, _, err := installer.servicePath(ModeUser)
	if err == nil {
		t.Fatal("expected error when homedir fails")
	}
}

func TestLinuxSystemdUnitNameEmpty(t *testing.T) {
	t.Parallel()

	installer := NewLinuxSystemdInstaller("/bin/syncd", "/cfg")
	installer.ServiceName = "  " // whitespace only → should fall back to default
	name := installer.unitName()
	if name != "git-project-sync.service" {
		t.Fatalf("expected default unit name, got: %s", name)
	}
}

func TestLinuxSystemdPreflightInvalidMode(t *testing.T) {
	t.Parallel()

	installer := NewLinuxSystemdInstaller("/usr/local/bin/syncd", "/tmp/config.yaml")
	installer.goos = "linux"
	installer.lookPath = func(string) (string, error) { return "/usr/bin/systemctl", nil }
	installer.stat = func(string) (os.FileInfo, error) { return nil, nil }

	findings := installer.Preflight(Mode("invalid"))
	found := false
	for _, f := range findings {
		if f.Code == ReasonInstallInvalidMode {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s finding for invalid mode, got: %+v", ReasonInstallInvalidMode, findings)
	}
}

func TestLinuxSystemdPreflightMissingBinaryPath(t *testing.T) {
	t.Parallel()

	installer := NewLinuxSystemdInstaller("", "/tmp/config.yaml")
	installer.goos = "linux"
	installer.lookPath = func(string) (string, error) { return "/usr/bin/systemctl", nil }

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

func TestLinuxSystemdPreflightMissingConfigPath(t *testing.T) {
	t.Parallel()

	installer := NewLinuxSystemdInstaller("/usr/local/bin/syncd", "")
	installer.goos = "linux"
	installer.lookPath = func(string) (string, error) { return "/usr/bin/systemctl", nil }
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

func TestLinuxSystemdPreflightBinaryMissing(t *testing.T) {
	t.Parallel()

	installer := NewLinuxSystemdInstaller("/usr/local/bin/syncd", "/tmp/config.yaml")
	installer.goos = "linux"
	installer.lookPath = func(string) (string, error) { return "/usr/bin/systemctl", nil }
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
