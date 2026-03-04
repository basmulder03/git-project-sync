package providers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/basmulder03/git-project-sync/internal/core/git"
)

type DefaultBranchResolver interface {
	ResolveDefaultBranch(ctx context.Context, repoPath, remote string) (string, error)
}

type GitHubResolver struct {
	Git *git.Client
}

func NewGitHubResolver(client *git.Client) *GitHubResolver {
	return &GitHubResolver{Git: client}
}

func (r *GitHubResolver) ResolveDefaultBranch(ctx context.Context, repoPath, remote string) (string, error) {
	for _, candidate := range []string{"main", "master"} {
		exists, err := r.Git.RemoteBranchExists(ctx, repoPath, remote, candidate)
		if err != nil {
			return "", err
		}
		if exists {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not resolve GitHub default branch for remote %q", remote)
}

func (r *GitHubResolver) ParseRateLimit(resp *http.Response) (*RateLimitError, bool) {
	return ParseRateLimitError("github", resp)
}
