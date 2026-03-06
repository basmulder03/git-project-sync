package clone

import (
	"context"
	"errors"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/providers/api"
)

// RetryConfig configures retry behavior for clone operations
type RetryConfig struct {
	MaxAttempts        int
	BaseBackoffSeconds int
}

// CloneWithRetry attempts to clone a repository with retry logic
func (e *Engine) CloneWithRetry(ctx context.Context, repo api.RemoteRepository, retryConfig RetryConfig, dryRun bool) CloneResult {
	var lastResult CloneResult

	for attempt := 1; attempt <= retryConfig.MaxAttempts; attempt++ {
		result := e.CloneRepository(ctx, repo, dryRun)

		if result.Success {
			return result
		}

		lastResult = result

		// Don't retry on validation errors or if it's the last attempt
		if !isTransientError(result.Error) || attempt == retryConfig.MaxAttempts {
			break
		}

		// Calculate backoff duration
		backoff := time.Duration(retryConfig.BaseBackoffSeconds) * time.Second * time.Duration(1<<(attempt-1))

		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			lastResult.Error = ctx.Err()
			lastResult.ReasonCode = "cancelled"
			return lastResult
		case <-time.After(backoff):
			// Continue to next attempt
		}
	}

	// All attempts failed
	lastResult.ReasonCode = "retry_exhausted"
	return lastResult
}

// isTransientError determines if an error is transient and should be retried
func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Network-related errors are transient
	transientPatterns := []string{
		"connection",
		"timeout",
		"temporary",
		"network",
		"unavailable",
		"refused",
		"reset",
	}

	for _, pattern := range transientPatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	// Check if it's a context error (not transient in retry sense)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(hasPrefix(s, substr) || hasSuffix(s, substr) || hasInfix(s, substr)))
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func hasInfix(s, infix string) bool {
	for i := 0; i <= len(s)-len(infix); i++ {
		if s[i:i+len(infix)] == infix {
			return true
		}
	}
	return false
}
