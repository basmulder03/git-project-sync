package providers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type RateLimitError struct {
	Provider   string
	RetryAfter time.Duration
	Message    string
}

func (e *RateLimitError) Error() string {
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		msg = "provider rate limit exceeded"
	}
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%s (retry_after=%s)", msg, e.RetryAfter)
	}
	return msg
}

func NewRateLimitError(provider string, retryAfter time.Duration, message string) error {
	return &RateLimitError{Provider: provider, RetryAfter: retryAfter, Message: message}
}

func AsRateLimitError(err error) (*RateLimitError, bool) {
	if err == nil {
		return nil, false
	}

	typed, ok := err.(*RateLimitError)
	if !ok {
		return nil, false
	}
	return typed, true
}

func ParseRateLimitError(provider string, resp *http.Response) (*RateLimitError, bool) {
	if resp == nil {
		return nil, false
	}

	retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
	remaining := strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining"))
	resetAt := parseUnixSeconds(resp.Header.Get("X-RateLimit-Reset"))

	isRateLimited := resp.StatusCode == http.StatusTooManyRequests
	if !isRateLimited && remaining == "0" {
		isRateLimited = true
	}

	if !isRateLimited {
		return nil, false
	}

	if retryAfter <= 0 && !resetAt.IsZero() {
		candidate := time.Until(resetAt)
		if candidate > 0 {
			retryAfter = candidate
		}
	}

	if retryAfter <= 0 {
		retryAfter = 30 * time.Second
	}

	return &RateLimitError{
		Provider:   provider,
		RetryAfter: retryAfter,
		Message:    fmt.Sprintf("%s API rate limited (status=%d)", provider, resp.StatusCode),
	}, true
}

func parseRetryAfter(value string) time.Duration {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}

	if secs, err := strconv.Atoi(trimmed); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}

	if when, err := http.ParseTime(trimmed); err == nil {
		delta := time.Until(when)
		if delta > 0 {
			return delta
		}
	}

	return 0
}

func parseUnixSeconds(value string) time.Time {
	secs, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || secs <= 0 {
		return time.Time{}
	}
	return time.Unix(secs, 0).UTC()
}
