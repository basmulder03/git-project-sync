package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type CleanupResult struct {
	DeletedBranch string
	Skipped       bool
	ReasonCode    string
	Reason        string
}

func (c *Client) CleanupCheckedOutStaleBranch(ctx context.Context, repoPath, defaultBranch string) (CleanupResult, error) {
	current, err := c.CurrentBranch(ctx, repoPath)
	if err != nil {
		return CleanupResult{}, err
	}

	if current == defaultBranch {
		return CleanupResult{Skipped: true, ReasonCode: "cleanup_not_applicable", Reason: "default branch is already checked out"}, nil
	}

	merged, err := c.IsAncestor(ctx, repoPath, current, defaultBranch)
	if err != nil {
		return CleanupResult{}, err
	}
	if !merged {
		return CleanupResult{Skipped: true, ReasonCode: "cleanup_branch_not_merged", Reason: "checked-out branch is not merged into default branch"}, nil
	}

	unique, err := c.HasUniqueCommits(ctx, repoPath, current, defaultBranch)
	if err != nil {
		return CleanupResult{}, err
	}
	if unique {
		return CleanupResult{Skipped: true, ReasonCode: "cleanup_unique_commits_present", Reason: "branch has unique commits not present on default branch"}, nil
	}

	if err := c.CheckoutBranch(ctx, repoPath, defaultBranch); err != nil {
		return CleanupResult{}, err
	}

	if err := c.DeleteBranch(ctx, repoPath, current); err != nil {
		return CleanupResult{}, err
	}

	return CleanupResult{DeletedBranch: current}, nil
}

func (c *Client) IsAncestor(ctx context.Context, repoPath, ancestor, descendant string) (bool, error) {
	err := c.runNoOutput(ctx, repoPath, "merge-base", "--is-ancestor", ancestor, descendant)
	if err == nil {
		return true, nil
	}

	if exitCode(err) == 1 {
		return false, nil
	}

	return false, fmt.Errorf("merge-base is-ancestor check failed: %w", err)
}

func (c *Client) HasUniqueCommits(ctx context.Context, repoPath, branch, base string) (bool, error) {
	out, err := c.run(ctx, repoPath, "rev-list", "--count", base+".."+branch)
	if err != nil {
		return false, fmt.Errorf("count unique commits: %w", err)
	}

	count, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return false, fmt.Errorf("parse unique commit count: %w", err)
	}

	return count > 0, nil
}

func (c *Client) CheckoutBranch(ctx context.Context, repoPath, branch string) error {
	if _, err := c.run(ctx, repoPath, "checkout", branch); err != nil {
		return fmt.Errorf("checkout branch %q: %w", branch, err)
	}
	return nil
}

func (c *Client) DeleteBranch(ctx context.Context, repoPath, branch string) error {
	if _, err := c.run(ctx, repoPath, "branch", "-d", branch); err != nil {
		return fmt.Errorf("delete local branch %q: %w", branch, err)
	}
	return nil
}
