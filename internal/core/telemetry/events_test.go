package telemetry

import "testing"

func TestReasonConstantsHaveValues(t *testing.T) {
	t.Parallel()

	if ReasonSyncCompleted == "" || ReasonRepoLocked == "" || ReasonSyncRetry == "" || ReasonSyncFailed == "" {
		t.Fatal("telemetry reason constants must not be empty")
	}
}
