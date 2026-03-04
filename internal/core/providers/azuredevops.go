package providers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/basmulder03/git-project-sync/internal/core/git"
)

type AzureDevOpsResolver struct {
	Git *git.Client
}

func NewAzureDevOpsResolver(client *git.Client) *AzureDevOpsResolver {
	return &AzureDevOpsResolver{Git: client}
}

func (r *AzureDevOpsResolver) ResolveDefaultBranch(ctx context.Context, repoPath, remote string) (string, error) {
	for _, candidate := range []string{"main", "master"} {
		exists, err := r.Git.RemoteBranchExists(ctx, repoPath, remote, candidate)
		if err != nil {
			return "", err
		}
		if exists {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not resolve Azure DevOps default branch for remote %q", remote)
}

func (r *AzureDevOpsResolver) ParseRateLimit(resp *http.Response) (*RateLimitError, bool) {
	return ParseRateLimitError("azuredevops", resp)
}
