package providers

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestParseRateLimitErrorUsesRetryAfterHeader(t *testing.T) {
	t.Parallel()

	resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}}
	resp.Header.Set("Retry-After", "7")

	err, ok := ParseRateLimitError("github", resp)
	if !ok {
		t.Fatal("expected rate limit parsing to match")
	}
	if err.RetryAfter != 7*time.Second {
		t.Fatalf("retry_after=%s want 7s", err.RetryAfter)
	}
}

func TestParseRateLimitErrorUsesRemainingAndResetHeaders(t *testing.T) {
	t.Parallel()

	reset := time.Now().UTC().Add(9 * time.Second)
	resp := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}
	resp.Header.Set("X-RateLimit-Remaining", "0")
	resp.Header.Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))

	err, ok := ParseRateLimitError("azuredevops", resp)
	if !ok {
		t.Fatal("expected header-based rate limit detection")
	}
	if err.RetryAfter <= 0 {
		t.Fatalf("expected positive retry_after, got %s", err.RetryAfter)
	}
}
