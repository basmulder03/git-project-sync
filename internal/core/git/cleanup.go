package git

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type BranchCleanupDecision struct {
	Branch     string
	Deleted    bool
	ReasonCode string
	Reason     string
}

type CleanupResult struct {
	DeletedBranches []string
	Decisions       []BranchCleanupDecision
}

func (c *Client) CleanupMergedLocalBranches(ctx context.Context, repoPath, defaultBranch string) (CleanupResult, error) {
	branches, err := c.ListLocalBranches(ctx, repoPath)
	if err != nil {
		return CleanupResult{}, err
	}

	current, err := c.CurrentBranch(ctx, repoPath)
	if err != nil {
		return CleanupResult{}, err
	}

	result := CleanupResult{
		DeletedBranches: make([]string, 0),
		Decisions:       make([]BranchCleanupDecision, 0, len(branches)),
	}

	if current != defaultBranch {
		decision, err := c.cleanupBranch(ctx, repoPath, current, defaultBranch, true)
		if err != nil {
			return CleanupResult{}, err
		}
		result.Decisions = append(result.Decisions, decision)
		if decision.Deleted {
			result.DeletedBranches = append(result.DeletedBranches, current)
			current = defaultBranch
			branches, err = c.ListLocalBranches(ctx, repoPath)
			if err != nil {
				return CleanupResult{}, err
			}
		}
	}

	for _, branch := range branches {
		if branch == defaultBranch || branch == current {
			continue
		}

		decision, err := c.cleanupBranch(ctx, repoPath, branch, defaultBranch, false)
		if err != nil {
			return CleanupResult{}, err
		}

		result.Decisions = append(result.Decisions, decision)
		if decision.Deleted {
			result.DeletedBranches = append(result.DeletedBranches, branch)
		}
	}

	return result, nil
}

func (c *Client) cleanupBranch(ctx context.Context, repoPath, branch, defaultBranch string, isCurrent bool) (BranchCleanupDecision, error) {
	merged, err := c.IsAncestor(ctx, repoPath, branch, defaultBranch)
	if err != nil {
		return BranchCleanupDecision{}, err
	}
	if !merged {
		return BranchCleanupDecision{Branch: branch, ReasonCode: "cleanup_branch_not_merged", Reason: "branch is not merged into default branch"}, nil
	}

	unique, err := c.HasUniqueCommits(ctx, repoPath, branch, defaultBranch)
	if err != nil {
		return BranchCleanupDecision{}, err
	}
	if unique {
		return BranchCleanupDecision{Branch: branch, ReasonCode: "cleanup_unique_commits_present", Reason: "branch has unique commits not present on default branch"}, nil
	}

	if isCurrent {
		if err := c.CheckoutBranch(ctx, repoPath, defaultBranch); err != nil {
			return BranchCleanupDecision{}, err
		}
	}

	if err := c.DeleteBranch(ctx, repoPath, branch); err != nil {
		return BranchCleanupDecision{}, err
	}

	return BranchCleanupDecision{Branch: branch, Deleted: true}, nil
}

func (c *Client) ListLocalBranches(ctx context.Context, repoPath string) ([]string, error) {
	out, err := c.run(ctx, repoPath, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, fmt.Errorf("list local branches: %w", err)
	}

	branches := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		branch := strings.TrimSpace(line)
		if branch == "" {
			continue
		}
		branches = append(branches, branch)
	}

	sort.Strings(branches)
	return branches, nil
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
