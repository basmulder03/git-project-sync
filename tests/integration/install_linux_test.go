package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLinuxInstallScriptsContainServiceRegistrationFlow(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" {
		t.Skip("linux-specific integration test")
	}

	root := repoRoot(t)
	installScript := filepath.Join(root, "scripts", "install.sh")
	uninstallScript := filepath.Join(root, "scripts", "uninstall.sh")

	for _, script := range []string{installScript, uninstallScript} {
		if _, err := os.Stat(script); err != nil {
			t.Fatalf("expected script %s: %v", script, err)
		}
		cmd := exec.Command("bash", "-n", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bash syntax check failed for %s: %v (%s)", script, err, string(out))
		}
	}

	installContent, err := os.ReadFile(installScript)
	if err != nil {
		t.Fatalf("read install script: %v", err)
	}
	uninstallContent, err := os.ReadFile(uninstallScript)
	if err != nil {
		t.Fatalf("read uninstall script: %v", err)
	}

	for _, want := range []string{"systemctl", "enable --now"} {
		if !strings.Contains(string(installContent), want) {
			t.Fatalf("install script missing %q", want)
		}
	}
	for _, want := range []string{"systemctl", "disable --now"} {
		if !strings.Contains(string(uninstallContent), want) {
			t.Fatalf("uninstall script missing %q", want)
		}
	}
}
