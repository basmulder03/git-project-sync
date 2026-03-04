package providers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

type ErrorClass string

const (
	ErrorClassTransient ErrorClass = "transient"
	ErrorClassPermanent ErrorClass = "permanent"
)

func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{Timeout: timeout, Transport: transport}
}

func ClassifyError(err error) (ErrorClass, string) {
	if err == nil {
		return ErrorClassPermanent, ""
	}

	if _, ok := AsRateLimitError(err); ok {
		return ErrorClassTransient, "provider_rate_limited"
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrorClassTransient, "timeout"
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return ErrorClassTransient, "network_error"
	}

	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "connection reset") || strings.Contains(lower, "temporarily unavailable") || strings.Contains(lower, "timeout") {
		return ErrorClassTransient, "network_error"
	}

	return ErrorClassPermanent, "permanent_error"
}

func IsTransientError(err error) bool {
	class, _ := ClassifyError(err)
	return class == ErrorClassTransient
}

func WrapHTTPStatusError(provider string, statusCode int) error {
	if statusCode >= 500 && statusCode <= 599 {
		return fmt.Errorf("%s provider temporary server error: status=%d", provider, statusCode)
	}
	return fmt.Errorf("%s provider request failed: status=%d", provider, statusCode)
}
