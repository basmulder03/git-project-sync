package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/install"
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
