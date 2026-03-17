package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	coressh "github.com/basmulder03/git-project-sync/internal/core/ssh"
)

// AzureDevOpsClient implements RepositoryDiscoveryClient for Azure DevOps
type AzureDevOpsClient struct {
	source     config.SourceConfig
	token      string
	httpClient *http.Client
	baseURL    string
}

// NewAzureDevOpsClient creates a new Azure DevOps API client
func NewAzureDevOpsClient(source config.SourceConfig, token string, httpClient *http.Client) *AzureDevOpsClient {
	baseURL := "https://dev.azure.com"
	if source.Host != "" && source.Host != "dev.azure.com" {
		// Azure DevOps Server (on-premises)
		baseURL = "https://" + source.Host
	}

	return &AzureDevOpsClient{
		source:     source,
		token:      token,
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

// azureProject represents an Azure DevOps project
type azureProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// azureProjectsResponse represents the response from projects list API
type azureProjectsResponse struct {
	Count int            `json:"count"`
	Value []azureProject `json:"value"`
}

// azureRepo represents a repository from Azure DevOps API
type azureRepo struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	RemoteURL     string       `json:"remoteUrl"`
	DefaultBranch string       `json:"defaultBranch"`
	Size          int64        `json:"size"` // in bytes
	IsDisabled    bool         `json:"isDisabled"`
	IsFork        bool         `json:"isFork"`
	Project       azureProject `json:"project"`
}

// azureReposResponse represents the response from repos list API
type azureReposResponse struct {
	Count int         `json:"count"`
	Value []azureRepo `json:"value"`
}

// ListRepositories retrieves all repositories accessible to the authenticated user
func (c *AzureDevOpsClient) ListRepositories(ctx context.Context, opts ListOptions) ([]RemoteRepository, error) {
	// First, get all projects
	projects, err := c.listProjects(ctx)
	if err != nil {
		return nil, err
	}

	// Then, get all repos from each project
	var allRepos []RemoteRepository
	for _, project := range projects {
		repos, err := c.listReposInProject(ctx, project.Name, opts)
		if err != nil {
			// Log error but continue with other projects
			continue
		}
		allRepos = append(allRepos, repos...)
	}

	return allRepos, nil
}

// listProjects retrieves all projects in the organization/account
func (c *AzureDevOpsClient) listProjects(ctx context.Context) ([]azureProject, error) {
	// Azure DevOps API endpoint for listing projects
	url := fmt.Sprintf("%s/%s/_apis/projects?api-version=7.0", c.baseURL, c.source.Account)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Azure DevOps uses Basic auth with PAT as password
	req.SetBasicAuth("", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &ClientError{
			Provider:  "azuredevops",
			Message:   fmt.Sprintf("request failed: %v", err),
			Transient: true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		isTransient := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, &ClientError{
			Provider:   "azuredevops",
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("API request failed (status %d): %s", resp.StatusCode, string(body)),
			Transient:  isTransient,
		}
	}

	var projectsResp azureProjectsResponse
	if err := json.NewDecoder(resp.Body).Decode(&projectsResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return projectsResp.Value, nil
}

// listReposInProject retrieves all repositories in a specific project
func (c *AzureDevOpsClient) listReposInProject(ctx context.Context, projectName string, opts ListOptions) ([]RemoteRepository, error) {
	// Azure DevOps API endpoint for listing repos in a project
	url := fmt.Sprintf("%s/%s/%s/_apis/git/repositories?api-version=7.0",
		c.baseURL, c.source.Account, projectName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.SetBasicAuth("", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &ClientError{
			Provider:  "azuredevops",
			Message:   fmt.Sprintf("request failed: %v", err),
			Transient: true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		isTransient := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, &ClientError{
			Provider:   "azuredevops",
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("API request failed (status %d): %s", resp.StatusCode, string(body)),
			Transient:  isTransient,
		}
	}

	var reposResp azureReposResponse
	if err := json.NewDecoder(resp.Body).Decode(&reposResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Convert to RemoteRepository and apply filters
	var repos []RemoteRepository
	for _, ar := range reposResp.Value {
		// Apply filters
		if ar.IsDisabled {
			continue // Always skip disabled repos
		}
		if ar.IsFork && !opts.IncludeForks {
			continue
		}

		sizeKB := ar.Size / 1024
		if opts.MaxSizeKB > 0 && sizeKB > opts.MaxSizeKB {
			continue
		}

		repos = append(repos, RemoteRepository{
			Provider:      "azuredevops",
			SourceID:      c.source.ID,
			Owner:         c.source.Account + "/" + ar.Project.Name,
			Name:          ar.Name,
			FullName:      fmt.Sprintf("%s/%s/%s", c.source.Account, ar.Project.Name, ar.Name),
			CloneURL:      c.cleanCloneURL(ar.RemoteURL),
			SSHCloneURL:   c.buildSSHCloneURL(c.source.Account, ar.Project.Name, ar.Name),
			DefaultBranch: c.normalizeDefaultBranch(ar.DefaultBranch),
			IsArchived:    false, // Azure doesn't have archive concept
			IsDisabled:    ar.IsDisabled,
			IsFork:        ar.IsFork,
			SizeKB:        sizeKB,
			Visibility:    "private",  // Azure DevOps repos are private by default
			UpdatedAt:     time.Now(), // Azure API doesn't provide this in list endpoint
		})
	}

	return repos, nil
}

// GetRepositoryMetadata fetches detailed information for a specific repository
func (c *AzureDevOpsClient) GetRepositoryMetadata(ctx context.Context, owner, repo string) (*RemoteRepository, error) {
	// Parse owner as "account/project"
	parts := strings.Split(owner, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid owner format (expected account/project): %s", owner)
	}
	projectName := parts[1]

	url := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s?api-version=7.0",
		c.baseURL, c.source.Account, projectName, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.SetBasicAuth("", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &ClientError{
			Provider:  "azuredevops",
			Message:   fmt.Sprintf("request failed: %v", err),
			Transient: true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		isTransient := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, &ClientError{
			Provider:   "azuredevops",
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("API request failed (status %d): %s", resp.StatusCode, string(body)),
			Transient:  isTransient,
		}
	}

	var ar azureRepo
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	sizeKB := ar.Size / 1024

	return &RemoteRepository{
		Provider:      "azuredevops",
		SourceID:      c.source.ID,
		Owner:         c.source.Account + "/" + ar.Project.Name,
		Name:          ar.Name,
		FullName:      fmt.Sprintf("%s/%s/%s", c.source.Account, ar.Project.Name, ar.Name),
		CloneURL:      c.cleanCloneURL(ar.RemoteURL),
		SSHCloneURL:   c.buildSSHCloneURL(c.source.Account, ar.Project.Name, ar.Name),
		DefaultBranch: c.normalizeDefaultBranch(ar.DefaultBranch),
		IsArchived:    false,
		IsDisabled:    ar.IsDisabled,
		IsFork:        ar.IsFork,
		SizeKB:        sizeKB,
		Visibility:    "private",
		UpdatedAt:     time.Now(),
	}, nil
}

// buildSSHCloneURL constructs the SSH clone URL for an Azure DevOps repository.
func (c *AzureDevOpsClient) buildSSHCloneURL(org, project, repo string) string {
	alias := coressh.AliasForSource(c.source.ID)
	sshHost := c.source.SSH.SSHHost
	if sshHost == "" {
		if c.source.Host != "" && c.source.Host != "dev.azure.com" {
			// Azure DevOps Server: use the custom host for SSH too.
			sshHost = c.source.Host
		}
		// sshHost="" → coressh.CloneURLForAzureDevOps will use the default
	}
	return coressh.CloneURLForAzureDevOps(org, project, repo, alias)
}

// cleanCloneURL removes any username@ prefix from Azure DevOps URLs
// Azure DevOps API returns URLs like: https://org@dev.azure.com/org/project/_git/repo
// We need clean URLs like: https://dev.azure.com/org/project/_git/repo
// Git credential manager will handle authentication without embedded credentials
func (c *AzureDevOpsClient) cleanCloneURL(cloneURL string) string {
	// Remove any trailing slash
	cloneURL = strings.TrimSuffix(cloneURL, "/")

	// Remove username@ prefix if present (e.g., https://org@dev.azure.com/...)
	if strings.HasPrefix(cloneURL, "https://") {
		// Find the @ symbol that separates username from hostname
		if idx := strings.Index(cloneURL, "@"); idx > 8 { // 8 = len("https://")
			// Remove everything between "https://" and "@"
			cloneURL = "https://" + cloneURL[idx+1:]
		}
	}

	return cloneURL
}

// normalizeDefaultBranch converts Azure's branch format to standard format
func (c *AzureDevOpsClient) normalizeDefaultBranch(branch string) string {
	// Azure returns branches in format "refs/heads/main"
	// Convert to just "main"
	branch = strings.TrimPrefix(branch, "refs/heads/")
	if branch == "" {
		return "main" // default fallback
	}
	return branch
}
