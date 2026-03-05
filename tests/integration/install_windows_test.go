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
	bootstrapScript := filepath.Join(root, "scripts", "bootstrap", "install.ps1")
	installScript := filepath.Join(root, "scripts", "install.ps1")
	uninstallScript := filepath.Join(root, "scripts", "uninstall.ps1")

	bootstrapContent, err := os.ReadFile(bootstrapScript)
	if err != nil {
		t.Fatalf("read bootstrap.ps1: %v", err)
	}
	installContent, err := os.ReadFile(installScript)
	if err != nil {
		t.Fatalf("read install.ps1: %v", err)
	}
	uninstallContent, err := os.ReadFile(uninstallScript)
	if err != nil {
		t.Fatalf("read uninstall.ps1: %v", err)
	}

	for _, want := range []string{"Invoke-WebRequest", "releases/latest/download", "raw.githubusercontent.com", "scripts/install.ps1", "Next steps:", "docs/getting-started/first-run-onboarding.md", "$syncctlPath sync all --dry-run"} {
		if !strings.Contains(string(bootstrapContent), want) {
			t.Fatalf("bootstrap.ps1 missing %q", want)
		}
	}

	for _, want := range []string{"schtasks", "/Create", "/Query", "cmd /c", "Add-ToPath", "SetEnvironmentVariable"} {
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
