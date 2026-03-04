package git

import "github.com/basmulder03/git-project-sync/internal/core/telemetry"

type DirtyState struct {
	HasStagedChanges   bool
	HasUnstagedChanges bool
	HasUntrackedFiles  bool
	HasConflicts       bool
}

func (d DirtyState) IsDirty() bool {
	return d.HasStagedChanges || d.HasUnstagedChanges || d.HasUntrackedFiles || d.HasConflicts
}

func (d DirtyState) ReasonCode() string {
	if !d.IsDirty() {
		return ""
	}
	if d.HasConflicts {
		return telemetry.ReasonRepoConflicts
	}
	if d.HasStagedChanges {
		return telemetry.ReasonRepoStagedChanges
	}
	if d.HasUnstagedChanges {
		return telemetry.ReasonRepoUnstagedChanges
	}
	if d.HasUntrackedFiles {
		return telemetry.ReasonRepoUntrackedFiles
	}
	return telemetry.ReasonRepoDirty
}
