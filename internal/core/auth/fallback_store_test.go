package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestFallbackStoreRoundTrip(t *testing.T) {
	t.Setenv("TOKEN_STORE_TEST_KEY", "super-secret-test-key")

	store, err := NewFallbackStore("git-project-sync", filepath.Join(t.TempDir(), "tokens.enc"), "TOKEN_STORE_TEST_KEY")
	if err != nil {
		t.Fatalf("new fallback store: %v", err)
	}

	ctx := context.Background()

	if err := store.SetToken(ctx, "gh-personal", "pat-123"); err != nil {
		t.Fatalf("set token: %v", err)
	}

	got, err := store.GetToken(ctx, "gh-personal")
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	if got != "pat-123" {
		t.Fatalf("token = %q, want pat-123", got)
	}

	if err := store.DeleteToken(ctx, "gh-personal"); err != nil {
		t.Fatalf("delete token: %v", err)
	}

	if _, err := store.GetToken(ctx, "gh-personal"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestFallbackStoreRequiresKey(t *testing.T) {
	store, err := NewFallbackStore("git-project-sync", filepath.Join(t.TempDir(), "tokens.enc"), "MISSING_KEY")
	if err != nil {
		t.Fatalf("new fallback store: %v", err)
	}

	err = store.SetToken(context.Background(), "gh-personal", "pat-123")
	if err == nil {
		t.Fatal("expected missing key env error")
	}
}

func TestNewTokenStoreUsesFallbackWhenForced(t *testing.T) {
	t.Setenv("TOKEN_STORE_TEST_KEY", "forced-fallback-key")

	store, err := NewTokenStore(Options{
		ServiceName:    "git-project-sync",
		FallbackPath:   filepath.Join(t.TempDir(), "tokens.enc"),
		FallbackKeyEnv: "TOKEN_STORE_TEST_KEY",
		ForceFallback:  true,
	})
	if err != nil {
		t.Fatalf("new token store: %v", err)
	}

	if err := store.SetToken(context.Background(), "az-work", "token-value"); err != nil {
		t.Fatalf("set token through fallback-backed store: %v", err)
	}
}
