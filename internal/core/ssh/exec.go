package ssh

import (
	"context"
	"os/exec"
)

// newGitCommand creates an exec.Cmd for a git invocation in the given directory.
// We avoid importing the internal/core/git package to prevent import cycles.
func newGitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	return cmd
}

// NewSSHTestCommand creates an exec.Cmd for testing SSH connectivity.
// args should be everything after the "ssh" binary name, e.g.:
//
//	["-i", "/path/to/key", "-o", "IdentitiesOnly=yes", "-T", "git@github.com"]
func NewSSHTestCommand(ctx context.Context, args []string) *exec.Cmd {
	return exec.CommandContext(ctx, "ssh", args...)
}
