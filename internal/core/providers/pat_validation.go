package providers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func ValidatePAT(ctx context.Context, source config.SourceConfig, token string) error {
	provider := normalizeProvider(source.Provider)
	switch provider {
	case "github":
		return validateGitHubPAT(ctx, source.Host, token)
	case "azuredevops":
		org := source.Organization
		if strings.TrimSpace(org) == "" {
			org = source.Account
		}
		return validateAzureDevOpsPAT(ctx, source.Host, org, token)
	default:
		return fmt.Errorf("unsupported provider %q", source.Provider)
	}
}

func validateGitHubPAT(ctx context.Context, host, token string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "github.com"
	}

	url := "https://api.github.com/user"
	if host != "github.com" {
		url = "https://" + host + "/api/v3/user"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("github PAT validation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if rateLimit, ok := ParseRateLimitError("github", resp); ok {
			return rateLimit
		}
		return fmt.Errorf("github PAT validation failed with status %d", resp.StatusCode)
	}

	return nil
}

func validateAzureDevOpsPAT(ctx context.Context, host, organization, token string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "dev.azure.com"
	}

	organization = strings.TrimSpace(organization)
	if organization == "" {
		return fmt.Errorf("azure devops organization/account context is required")
	}

	url := fmt.Sprintf("https://%s/%s/_apis/projects?api-version=7.1", host, organization)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	creds := base64.StdEncoding.EncodeToString([]byte(":" + token))
	req.Header.Set("Authorization", "Basic "+creds)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("azure devops PAT validation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if rateLimit, ok := ParseRateLimitError("azuredevops", resp); ok {
			return rateLimit
		}
		return fmt.Errorf("azure devops PAT validation failed with status %d", resp.StatusCode)
	}

	return nil
}
