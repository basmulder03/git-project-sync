package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func TestGitHubClient_ListRepositories(t *testing.T) {
	tests := []struct {
		name         string
		handler      http.HandlerFunc
		opts         ListOptions
		wantReposCnt int
		wantErr      bool
	}{
		{
			name: "successful list with no filters",
			handler: func(w http.ResponseWriter, r *http.Request) {
				// When Account is set, GitHub uses /users/:account/repos
				if r.URL.Path != "/users/testuser/repos" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`[
					{
						"id": 1,
						"name": "repo1",
						"full_name": "owner/repo1",
						"owner": {"login": "owner", "type": "User"},
						"private": false,
						"clone_url": "https://github.com/owner/repo1.git",
						"default_branch": "main",
						"archived": false,
						"disabled": false,
						"fork": false,
						"size": 1024,
						"visibility": "public",
						"updated_at": "2023-01-01T00:00:00Z"
					},
					{
						"id": 2,
						"name": "repo2",
						"full_name": "owner/repo2",
						"owner": {"login": "owner", "type": "User"},
						"private": true,
						"clone_url": "https://github.com/owner/repo2.git",
						"default_branch": "master",
						"archived": false,
						"disabled": false,
						"fork": false,
						"size": 2048,
						"visibility": "private",
						"updated_at": "2023-01-02T00:00:00Z"
					}
				]`))
			},
			opts:         ListOptions{},
			wantReposCnt: 2,
			wantErr:      false,
		},
		{
			name: "filter out archived repos",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`[
					{
						"id": 1,
						"name": "repo1",
						"full_name": "owner/repo1",
						"owner": {"login": "owner", "type": "User"},
						"private": false,
						"clone_url": "https://github.com/owner/repo1.git",
						"default_branch": "main",
						"archived": false,
						"disabled": false,
						"fork": false,
						"size": 1024,
						"visibility": "public",
						"updated_at": "2023-01-01T00:00:00Z"
					},
					{
						"id": 2,
						"name": "repo2",
						"full_name": "owner/repo2",
						"owner": {"login": "owner", "type": "User"},
						"private": true,
						"clone_url": "https://github.com/owner/repo2.git",
						"default_branch": "master",
						"archived": true,
						"disabled": false,
						"fork": false,
						"size": 2048,
						"visibility": "private",
						"updated_at": "2023-01-02T00:00:00Z"
					}
				]`))
			},
			opts:         ListOptions{IncludeArchived: false},
			wantReposCnt: 1,
			wantErr:      false,
		},
		{
			name: "filter by size",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`[
					{
						"id": 1,
						"name": "small-repo",
						"full_name": "owner/small-repo",
						"owner": {"login": "owner", "type": "User"},
						"private": false,
						"clone_url": "https://github.com/owner/small-repo.git",
						"default_branch": "main",
						"archived": false,
						"disabled": false,
						"fork": false,
						"size": 500,
						"visibility": "public",
						"updated_at": "2023-01-01T00:00:00Z"
					},
					{
						"id": 2,
						"name": "large-repo",
						"full_name": "owner/large-repo",
						"owner": {"login": "owner", "type": "User"},
						"private": false,
						"clone_url": "https://github.com/owner/large-repo.git",
						"default_branch": "main",
						"archived": false,
						"disabled": false,
						"fork": false,
						"size": 5000,
						"visibility": "public",
						"updated_at": "2023-01-02T00:00:00Z"
					}
				]`))
			},
			opts:         ListOptions{MaxSizeKB: 1000},
			wantReposCnt: 1,
			wantErr:      false,
		},
		{
			name: "rate limit error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", "1672531200")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"message": "API rate limit exceeded"}`))
			},
			opts:         ListOptions{},
			wantReposCnt: 0,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.handler))
			defer server.Close()

			source := config.SourceConfig{
				ID:       "test-github",
				Provider: "github",
				Account:  "testuser",
				Host:     "github.com",
				Enabled:  true,
			}

			httpClient := &http.Client{Timeout: 5 * time.Second}
			client := &GitHubClient{
				source:     source,
				token:      "test-token",
				httpClient: httpClient,
				baseURL:    server.URL,
			}

			ctx := context.Background()
			repos, err := client.ListRepositories(ctx, tt.opts)

			if (err != nil) != tt.wantErr {
				t.Errorf("ListRepositories() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(repos) != tt.wantReposCnt {
				t.Errorf("ListRepositories() got %d repos, want %d", len(repos), tt.wantReposCnt)
			}

			// Verify fields are populated correctly
			if !tt.wantErr && len(repos) > 0 {
				repo := repos[0]
				if repo.Provider != "github" {
					t.Errorf("Provider = %s, want github", repo.Provider)
				}
				if repo.SourceID != source.ID {
					t.Errorf("SourceID = %s, want %s", repo.SourceID, source.ID)
				}
				if repo.CloneURL == "" {
					t.Error("CloneURL is empty")
				}
			}
		})
	}
}

func TestGitHubClient_GetRepositoryMetadata(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": 1,
			"name": "repo1",
			"full_name": "owner/repo1",
			"owner": {"login": "owner", "type": "User"},
			"private": false,
			"clone_url": "https://github.com/owner/repo1.git",
			"default_branch": "main",
			"archived": false,
			"disabled": false,
			"fork": false,
			"size": 1024,
			"visibility": "public",
			"updated_at": "2023-01-01T00:00:00Z"
		}`))
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	source := config.SourceConfig{
		ID:       "test-github",
		Provider: "github",
		Account:  "testuser",
		Host:     "github.com",
		Enabled:  true,
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}
	client := &GitHubClient{
		source:     source,
		token:      "test-token",
		httpClient: httpClient,
		baseURL:    server.URL,
	}

	ctx := context.Background()
	repo, err := client.GetRepositoryMetadata(ctx, "owner", "repo1")

	if err != nil {
		t.Fatalf("GetRepositoryMetadata() error = %v", err)
	}

	if repo.Name != "repo1" {
		t.Errorf("Name = %s, want repo1", repo.Name)
	}
	if repo.FullName != "owner/repo1" {
		t.Errorf("FullName = %s, want owner/repo1", repo.FullName)
	}
	if repo.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %s, want main", repo.DefaultBranch)
	}
}

func TestClientFactory_CreateClient(t *testing.T) {
	factory := NewClientFactory(10 * time.Second)

	tests := []struct {
		name     string
		provider string
		wantErr  bool
	}{
		{
			name:     "github provider",
			provider: "github",
			wantErr:  false,
		},
		{
			name:     "azuredevops provider",
			provider: "azuredevops",
			wantErr:  false,
		},
		{
			name:     "azure alias",
			provider: "azure",
			wantErr:  false,
		},
		{
			name:     "unsupported provider",
			provider: "gitlab",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := config.SourceConfig{
				ID:       "test-source",
				Provider: tt.provider,
				Account:  "testaccount",
				Enabled:  true,
			}

			client, err := factory.CreateClient(source, "test-token")

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && client == nil {
				t.Error("CreateClient() returned nil client without error")
			}
		})
	}
}
