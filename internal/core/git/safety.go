package git

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
		return "repo_conflicts"
	}
	if d.HasStagedChanges {
		return "repo_staged_changes"
	}
	if d.HasUnstagedChanges {
		return "repo_unstaged_changes"
	}
	if d.HasUntrackedFiles {
		return "repo_untracked_files"
	}
	return "repo_dirty"
}
