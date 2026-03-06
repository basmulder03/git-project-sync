package api

import (
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func TestAzureDevOpsClient_cleanCloneURL(t *testing.T) {
	client := &AzureDevOpsClient{
		token: "test-token",
		source: config.SourceConfig{
			Account: "test-org",
		},
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "URL with org prefix",
			input:    "https://rr-wfm@dev.azure.com/rr-wfm/Platform/_git/DeVries",
			expected: "https://dev.azure.com/rr-wfm/Platform/_git/DeVries",
		},
		{
			name:     "URL already clean",
			input:    "https://dev.azure.com/org/project/_git/repo",
			expected: "https://dev.azure.com/org/project/_git/repo",
		},
		{
			name:     "URL with trailing slash",
			input:    "https://org@dev.azure.com/org/project/_git/repo/",
			expected: "https://dev.azure.com/org/project/_git/repo",
		},
		{
			name:     "URL with different org name",
			input:    "https://myorg@dev.azure.com/myorg/MyProject/_git/MyRepo",
			expected: "https://dev.azure.com/myorg/MyProject/_git/MyRepo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.cleanCloneURL(tt.input)
			if result != tt.expected {
				t.Errorf("cleanCloneURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
