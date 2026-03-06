package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

// GitHubClient implements RepositoryDiscoveryClient for GitHub
type GitHubClient struct {
	source     config.SourceConfig
	token      string
	httpClient *http.Client
	baseURL    string
}

// NewGitHubClient creates a new GitHub API client
func NewGitHubClient(source config.SourceConfig, token string, httpClient *http.Client) *GitHubClient {
	baseURL := "https://api.github.com"
	if source.Host != "" && source.Host != "github.com" {
		// GitHub Enterprise
		baseURL = fmt.Sprintf("https://%s/api/v3", source.Host)
	}

	return &GitHubClient{
		source:     source,
		token:      token,
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

// githubRepo represents a repository from GitHub API
type githubRepo struct {
	ID            int64       `json:"id"`
	Name          string      `json:"name"`
	FullName      string      `json:"full_name"`
	Owner         githubOwner `json:"owner"`
	Private       bool        `json:"private"`
	CloneURL      string      `json:"clone_url"`
	DefaultBranch string      `json:"default_branch"`
	Archived      bool        `json:"archived"`
	Disabled      bool        `json:"disabled"`
	Fork          bool        `json:"fork"`
	Size          int64       `json:"size"` // in KB
	Visibility    string      `json:"visibility"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

type githubOwner struct {
	Login string `json:"login"`
	Type  string `json:"type"` // "User" or "Organization"
}

// ListRepositories retrieves all repositories accessible to the authenticated user
func (c *GitHubClient) ListRepositories(ctx context.Context, opts ListOptions) ([]RemoteRepository, error) {
	var allRepos []RemoteRepository
	page := 1
	perPage := 100

	for {
		// Build URL based on whether we're targeting an org or user repos
		var url string
		if c.source.Organization != "" {
			url = fmt.Sprintf("%s/orgs/%s/repos?page=%d&per_page=%d&type=all",
				c.baseURL, c.source.Organization, page, perPage)
		} else if c.source.Account != "" {
			url = fmt.Sprintf("%s/users/%s/repos?page=%d&per_page=%d&type=all",
				c.baseURL, c.source.Account, page, perPage)
		} else {
			// List all repos for the authenticated user
			url = fmt.Sprintf("%s/user/repos?page=%d&per_page=%d&affiliation=owner,collaborator,organization_member",
				c.baseURL, page, perPage)
		}

		repos, hasMore, err := c.fetchPage(ctx, url, opts)
		if err != nil {
			return nil, err
		}

		allRepos = append(allRepos, repos...)

		if !hasMore || len(repos) == 0 {
			break
		}

		page++
	}

	return allRepos, nil
}

// fetchPage fetches a single page of repositories
func (c *GitHubClient) fetchPage(ctx context.Context, url string, opts ListOptions) ([]RemoteRepository, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, false, &ClientError{
			Provider:  "github",
			Message:   fmt.Sprintf("request failed: %v", err),
			Transient: true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		isTransient := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, false, &ClientError{
			Provider:   "github",
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("API request failed (status %d): %s", resp.StatusCode, string(body)),
			Transient:  isTransient,
		}
	}

	var githubRepos []githubRepo
	if err := json.NewDecoder(resp.Body).Decode(&githubRepos); err != nil {
		return nil, false, fmt.Errorf("decode response: %w", err)
	}

	// Convert to RemoteRepository and apply filters
	var repos []RemoteRepository
	for _, gr := range githubRepos {
		// Apply filters
		if gr.Archived && !opts.IncludeArchived {
			continue
		}
		if gr.Fork && !opts.IncludeForks {
			continue
		}
		if opts.MaxSizeKB > 0 && gr.Size > opts.MaxSizeKB {
			continue
		}
		if len(opts.Visibility) > 0 {
			matched := false
			visibility := "public"
			if gr.Private {
				visibility = "private"
			}
			if gr.Visibility != "" {
				visibility = gr.Visibility
			}
			for _, v := range opts.Visibility {
				if v == visibility {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		repos = append(repos, RemoteRepository{
			Provider:      "github",
			SourceID:      c.source.ID,
			Owner:         gr.Owner.Login,
			Name:          gr.Name,
			FullName:      gr.FullName,
			CloneURL:      c.buildAuthenticatedCloneURL(gr.CloneURL),
			DefaultBranch: gr.DefaultBranch,
			IsArchived:    gr.Archived,
			IsDisabled:    gr.Disabled,
			IsFork:        gr.Fork,
			SizeKB:        gr.Size,
			Visibility:    c.getVisibility(gr),
			UpdatedAt:     gr.UpdatedAt,
		})
	}

	// Check for more pages via Link header
	hasMore := false
	linkHeader := resp.Header.Get("Link")
	if linkHeader != "" {
		hasMore = strings.Contains(linkHeader, `rel="next"`)
	}

	return repos, hasMore, nil
}

// GetRepositoryMetadata fetches detailed information for a specific repository
func (c *GitHubClient) GetRepositoryMetadata(ctx context.Context, owner, repo string) (*RemoteRepository, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &ClientError{
			Provider:  "github",
			Message:   fmt.Sprintf("request failed: %v", err),
			Transient: true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		isTransient := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, &ClientError{
			Provider:   "github",
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("API request failed (status %d): %s", resp.StatusCode, string(body)),
			Transient:  isTransient,
		}
	}

	var gr githubRepo
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &RemoteRepository{
		Provider:      "github",
		SourceID:      c.source.ID,
		Owner:         gr.Owner.Login,
		Name:          gr.Name,
		FullName:      gr.FullName,
		CloneURL:      c.buildAuthenticatedCloneURL(gr.CloneURL),
		DefaultBranch: gr.DefaultBranch,
		IsArchived:    gr.Archived,
		IsDisabled:    gr.Disabled,
		IsFork:        gr.Fork,
		SizeKB:        gr.Size,
		Visibility:    c.getVisibility(gr),
		UpdatedAt:     gr.UpdatedAt,
	}, nil
}

// buildAuthenticatedCloneURL constructs a clone URL with embedded token
func (c *GitHubClient) buildAuthenticatedCloneURL(cloneURL string) string {
	// Convert https://github.com/owner/repo.git to https://token@github.com/owner/repo.git
	if strings.HasPrefix(cloneURL, "https://") {
		return strings.Replace(cloneURL, "https://", "https://"+c.token+"@", 1)
	}
	return cloneURL
}

// getVisibility determines the visibility of a repository
func (c *GitHubClient) getVisibility(repo githubRepo) string {
	if repo.Visibility != "" {
		return repo.Visibility
	}
	if repo.Private {
		return "private"
	}
	return "public"
}

// ParseRateLimit extracts rate limit information from response headers
func (c *GitHubClient) ParseRateLimit(resp *http.Response) (limit, remaining int, resetTime time.Time, err error) {
	limitStr := resp.Header.Get("X-RateLimit-Limit")
	remainingStr := resp.Header.Get("X-RateLimit-Remaining")
	resetStr := resp.Header.Get("X-RateLimit-Reset")

	if limitStr != "" {
		limit, _ = strconv.Atoi(limitStr)
	}
	if remainingStr != "" {
		remaining, _ = strconv.Atoi(remainingStr)
	}
	if resetStr != "" {
		resetUnix, _ := strconv.ParseInt(resetStr, 10, 64)
		resetTime = time.Unix(resetUnix, 0)
	}

	return limit, remaining, resetTime, nil
}
