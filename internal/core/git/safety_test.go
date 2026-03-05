package git

import (
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

func TestDirtyStateIsDirty(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state DirtyState
		want  bool
	}{
		{name: "clean", state: DirtyState{}, want: false},
		{name: "staged", state: DirtyState{HasStagedChanges: true}, want: true},
		{name: "unstaged", state: DirtyState{HasUnstagedChanges: true}, want: true},
		{name: "untracked", state: DirtyState{HasUntrackedFiles: true}, want: true},
		{name: "conflicts", state: DirtyState{HasConflicts: true}, want: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.state.IsDirty(); got != tc.want {
				t.Fatalf("IsDirty() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDirtyStateReasonCodePrecedence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state DirtyState
		want  string
	}{
		{name: "clean", state: DirtyState{}, want: ""},
		{name: "conflicts first", state: DirtyState{HasConflicts: true, HasStagedChanges: true, HasUnstagedChanges: true, HasUntrackedFiles: true}, want: telemetry.ReasonRepoConflicts},
		{name: "staged before unstaged", state: DirtyState{HasStagedChanges: true, HasUnstagedChanges: true, HasUntrackedFiles: true}, want: telemetry.ReasonRepoStagedChanges},
		{name: "unstaged before untracked", state: DirtyState{HasUnstagedChanges: true, HasUntrackedFiles: true}, want: telemetry.ReasonRepoUnstagedChanges},
		{name: "untracked", state: DirtyState{HasUntrackedFiles: true}, want: telemetry.ReasonRepoUntrackedFiles},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.state.ReasonCode(); got != tc.want {
				t.Fatalf("ReasonCode() = %q, want %q", got, tc.want)
			}
		})
	}
}
