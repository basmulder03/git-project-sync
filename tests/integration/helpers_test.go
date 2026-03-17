package integration

import (
	"path/filepath"
	"testing"
	"time"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	current, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve cwd: %v", err)
	}
	return filepath.Clean(filepath.Join(current, "..", ".."))
}

// waitForCondition polls cond every pollInterval until it returns true or the
// deadline is exceeded. It fails the test with msg if the condition is never
// satisfied. Use this instead of time.Sleep when a goroutine-driven side effect
// must be observed — it avoids fixed-delay flakiness on slow CI runners.
func waitForCondition(t *testing.T, timeout, pollInterval time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(pollInterval)
	}
	t.Fatalf("condition not met within %s: %s", timeout, msg)
}

// assertEventually is an alias for waitForCondition with sensible defaults
// (5 s timeout, 10 ms poll). Use for lightweight observable state changes.
func assertEventually(t *testing.T, msg string, cond func() bool) {
	t.Helper()
	waitForCondition(t, 5*time.Second, 10*time.Millisecond, msg, cond)
}
