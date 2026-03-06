package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalDevScriptsPresentAndWiredToDevConfig(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	linuxScript := filepath.Join(root, "scripts", "dev", "local.sh")
	windowsScript := filepath.Join(root, "scripts", "dev", "local.ps1")

	for _, script := range []string{linuxScript, windowsScript} {
		if _, err := os.Stat(script); err != nil {
			t.Fatalf("expected script %s: %v", script, err)
		}
	}

	if out, err := exec.Command("bash", "-n", linuxScript).CombinedOutput(); err != nil {
		t.Fatalf("bash syntax check failed for %s: %v (%s)", linuxScript, err, string(out))
	}

	linuxContent, err := os.ReadFile(linuxScript)
	if err != nil {
		t.Fatalf("read linux script: %v", err)
	}
	windowsContent, err := os.ReadFile(windowsScript)
	if err != nil {
		t.Fatalf("read windows script: %v", err)
	}

	for _, want := range []string{"config.dev.yaml", "state.dev.db", "go run ./cmd/syncctl --config", "go run ./cmd/syncd --config", "go run ./cmd/synctui --config"} {
		if !strings.Contains(string(linuxContent), want) {
			t.Fatalf("local.sh missing %q", want)
		}
		if !strings.Contains(string(windowsContent), want) {
			t.Fatalf("local.ps1 missing %q", want)
		}
	}
}
