package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

// ClientFactory creates provider-specific API clients
type ClientFactory struct {
	httpClient *http.Client
}

// NewClientFactory creates a new client factory with the given HTTP client
func NewClientFactory(timeout time.Duration) *ClientFactory {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return &ClientFactory{
		httpClient: httpClient,
	}
}

// CreateClient creates a provider-specific API client for the given source
func (f *ClientFactory) CreateClient(source config.SourceConfig, token string) (RepositoryDiscoveryClient, error) {
	provider := strings.ToLower(source.Provider)

	switch provider {
	case "github":
		return NewGitHubClient(source, token, f.httpClient), nil
	case "azuredevops", "azure":
		return NewAzureDevOpsClient(source, token, f.httpClient), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", source.Provider)
	}
}

// GetHTTPClient returns the underlying HTTP client
func (f *ClientFactory) GetHTTPClient() *http.Client {
	return f.httpClient
}
