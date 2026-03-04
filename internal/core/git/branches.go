package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func (c *Client) FetchAndPrune(ctx context.Context, repoPath, remote string) error {
	if remote == "" {
		remote = "origin"
	}
	_, err := c.run(ctx, repoPath, "fetch", remote, "--prune")
	if err != nil {
		return fmt.Errorf("fetch and prune: %w", err)
	}
	return nil
}

func (c *Client) ResolveDefaultBranchFromRemoteHEAD(ctx context.Context, repoPath, remote string) (string, error) {
	if remote == "" {
		remote = "origin"
	}

	out, err := c.run(ctx, repoPath, "symbolic-ref", "--short", "refs/remotes/"+remote+"/HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve remote HEAD: %w", err)
	}

	trimmed := strings.TrimSpace(out)
	prefix := remote + "/"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", fmt.Errorf("unexpected remote HEAD ref %q", trimmed)
	}

	branch := strings.TrimPrefix(trimmed, prefix)
	if branch == "" {
		return "", errors.New("resolved default branch is empty")
	}

	return branch, nil
}

func (c *Client) CurrentBranch(ctx context.Context, repoPath string) (string, error) {
	out, err := c.run(ctx, repoPath, "branch", "--show-current")
	if err != nil {
		return "", fmt.Errorf("current branch: %w", err)
	}

	branch := strings.TrimSpace(out)
	if branch == "" {
		return "", errors.New("repository is in detached HEAD state")
	}

	return branch, nil
}

func (c *Client) UpstreamBranch(ctx context.Context, repoPath string) (string, bool, error) {
	out, err := c.run(ctx, repoPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		if strings.Contains(err.Error(), "no upstream configured") {
			return "", false, nil
		}
		return "", false, fmt.Errorf("resolve upstream: %w", err)
	}

	branch := strings.TrimSpace(out)
	if branch == "" {
		return "", false, nil
	}

	return branch, true, nil
}

func (c *Client) AheadBehind(ctx context.Context, repoPath, localRef, upstreamRef string) (ahead, behind int, err error) {
	out, err := c.run(ctx, repoPath, "rev-list", "--left-right", "--count", localRef+"..."+upstreamRef)
	if err != nil {
		return 0, 0, fmt.Errorf("ahead/behind query: %w", err)
	}

	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected ahead/behind output %q", strings.TrimSpace(out))
	}

	ahead, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse ahead commits: %w", err)
	}
	behind, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse behind commits: %w", err)
	}

	return ahead, behind, nil
}

func (c *Client) FastForwardTo(ctx context.Context, repoPath, upstreamRef string) error {
	if _, err := c.run(ctx, repoPath, "merge", "--ff-only", upstreamRef); err != nil {
		return fmt.Errorf("fast-forward merge: %w", err)
	}
	return nil
}

func (c *Client) RemoteBranchExists(ctx context.Context, repoPath, remote, branch string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--exit-code", "--heads", remote, branch)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
			return false, nil
		}
		return false, fmt.Errorf("ls-remote for branch %q: %w", branch, err)
	}
	return true, nil
}
