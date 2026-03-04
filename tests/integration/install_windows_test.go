package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWindowsInstallScriptsContainTaskRegistrationFlow(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("windows-specific integration test")
	}

	root := repoRoot(t)
	installScript := filepath.Join(root, "scripts", "install.ps1")
	uninstallScript := filepath.Join(root, "scripts", "uninstall.ps1")

	installContent, err := os.ReadFile(installScript)
	if err != nil {
		t.Fatalf("read install.ps1: %v", err)
	}
	uninstallContent, err := os.ReadFile(uninstallScript)
	if err != nil {
		t.Fatalf("read uninstall.ps1: %v", err)
	}

	for _, want := range []string{"schtasks", "/Create", "/Query"} {
		if !strings.Contains(string(installContent), want) {
			t.Fatalf("install.ps1 missing %q", want)
		}
	}
	for _, want := range []string{"schtasks", "/Delete"} {
		if !strings.Contains(string(uninstallContent), want) {
			t.Fatalf("uninstall.ps1 missing %q", want)
		}
	}
}
