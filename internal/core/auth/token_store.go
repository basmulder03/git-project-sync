package auth

import (
	"context"
	"errors"
	"fmt"
)

var ErrTokenNotFound = errors.New("token not found")

type TokenStore interface {
	SetToken(ctx context.Context, sourceID, token string) error
	GetToken(ctx context.Context, sourceID string) (string, error)
	DeleteToken(ctx context.Context, sourceID string) error
}

type Options struct {
	ServiceName    string
	FallbackPath   string
	FallbackKeyEnv string
	ForceFallback  bool
}

func NewTokenStore(opts Options) (TokenStore, error) {
	service := opts.ServiceName
	if service == "" {
		service = "git-project-sync"
	}

	if !opts.ForceFallback {
		store, err := NewKeyringStore(service)
		if err == nil {
			return store, nil
		}

		if opts.FallbackPath == "" {
			return nil, fmt.Errorf("initialize keyring store: %w", err)
		}
	}

	if opts.FallbackPath == "" {
		return nil, errors.New("fallback path is required")
	}

	return NewFallbackStore(service, opts.FallbackPath, opts.FallbackKeyEnv)
}
