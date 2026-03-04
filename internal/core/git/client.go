package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) DirtyState(ctx context.Context, repoPath string) (DirtyState, error) {
	output, err := c.run(ctx, repoPath, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return DirtyState{}, err
	}

	state := DirtyState{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		if len(line) < 2 {
			continue
		}

		x := line[0]
		y := line[1]

		if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
			state.HasConflicts = true
		}

		if x != ' ' && x != '?' {
			state.HasStagedChanges = true
		}

		if y != ' ' && !(x == '?' && y == '?') {
			state.HasUnstagedChanges = true
		}

		if x == '?' && y == '?' {
			state.HasUntrackedFiles = true
		}
	}

	return state, nil
}

func (c *Client) run(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}
