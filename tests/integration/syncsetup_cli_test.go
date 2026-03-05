package integration

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestSyncsetupAppVersionFlag(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("syncsetup supports linux/windows only")
	}

	root := repoRoot(t)
	cmd := exec.Command("go", "run", "./cmd/syncsetup", "--app-version")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("syncsetup --app-version failed: %v (%s)", err, string(out))
	}
	if !strings.Contains(string(out), "syncsetup") {
		t.Fatalf("expected syncsetup version output, got: %s", string(out))
	}
}
