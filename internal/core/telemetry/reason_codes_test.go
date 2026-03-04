package telemetry

import "testing"

func TestEnsureReasonCode(t *testing.T) {
	t.Parallel()

	if got := EnsureReasonCode(""); got != ReasonUnknown {
		t.Fatalf("empty reason -> %q, want %q", got, ReasonUnknown)
	}
	if got := EnsureReasonCode("  repo_dirty  "); got != "repo_dirty" {
		t.Fatalf("trimmed reason -> %q, want repo_dirty", got)
	}
}
