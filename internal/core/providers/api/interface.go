package api

import (
	"context"
	"time"
)

// RepositoryDiscoveryClient defines the interface for discovering repositories from provider APIs
type RepositoryDiscoveryClient interface {
	// ListRepositories returns all repositories accessible to the authenticated token
	ListRepositories(ctx context.Context, opts ListOptions) ([]RemoteRepository, error)

	// GetRepositoryMetadata fetches detailed info for a specific repository
	GetRepositoryMetadata(ctx context.Context, owner, repo string) (*RemoteRepository, error)
}

// RemoteRepository represents a repository discovered from a provider API
type RemoteRepository struct {
	Provider      string    // "github" or "azuredevops"
	SourceID      string    // config source ID
	Owner         string    // account/org name
	Name          string    // repository name
	FullName      string    // owner/name (for matching)
	CloneURL      string    // HTTPS clone URL (no embedded credentials)
	SSHCloneURL   string    // SSH clone URL using the per-source alias (preferred)
	DefaultBranch string    // main, master, etc.
	IsArchived    bool      // whether repo is archived
	IsDisabled    bool      // whether repo is disabled
	IsFork        bool      // whether repo is a fork
	SizeKB        int64     // repository size in KB
	Visibility    string    // public, private, internal
	UpdatedAt     time.Time // last update time
}

// PreferredCloneURL returns the SSH clone URL when available, otherwise
// the HTTPS clone URL.  Callers should use this method to respect the
// SSH-first preference without needing to know which is set.
func (r *RemoteRepository) PreferredCloneURL() string {
	if r.SSHCloneURL != "" {
		return r.SSHCloneURL
	}
	return r.CloneURL
}

// ListOptions provides filtering options for repository discovery
type ListOptions struct {
	IncludeArchived bool     // whether to include archived repos
	IncludeForks    bool     // whether to include forked repos
	Visibility      []string // filter by visibility (empty = all)
	MaxSizeKB       int64    // skip repos larger than this (0 = no limit)
}

// ClientError represents an error from the provider API
type ClientError struct {
	Provider   string
	StatusCode int
	Message    string
	Transient  bool
}

func (e *ClientError) Error() string {
	return e.Message
}

// IsTransient returns true if the error is transient and should be retried
func (e *ClientError) IsTransient() bool {
	return e.Transient
}
