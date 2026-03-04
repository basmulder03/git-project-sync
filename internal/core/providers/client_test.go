package providers

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestClassifyErrorTransientCases(t *testing.T) {
	t.Parallel()

	cases := []error{
		context.DeadlineExceeded,
		&RateLimitError{Provider: "github", RetryAfter: time.Second},
		net.UnknownNetworkError("tcp"),
	}

	for _, err := range cases {
		class, _ := ClassifyError(err)
		if class != ErrorClassTransient {
			t.Fatalf("expected transient class for %T: %v", err, err)
		}
	}
}

func TestClassifyErrorPermanentDefault(t *testing.T) {
	t.Parallel()

	class, reason := ClassifyError(errors.New("invalid token"))
	if class != ErrorClassPermanent {
		t.Fatalf("class=%s want permanent", class)
	}
	if reason != "permanent_error" {
		t.Fatalf("reason=%q want permanent_error", reason)
	}
}
