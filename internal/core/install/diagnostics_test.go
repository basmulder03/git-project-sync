//go:build !windows

package install

import (
	"errors"
	"testing"
)

func TestReasonErrorError(t *testing.T) {
	t.Parallel()

	e := &ReasonError{Code: "code", Message: "msg", Err: errors.New("inner")}
	s := e.Error()
	if s == "" {
		t.Fatal("expected non-empty error string")
	}

	// Without wrapped error.
	e2 := &ReasonError{Code: "code", Message: "msg"}
	s2 := e2.Error()
	if s2 == "" {
		t.Fatal("expected non-empty error string without wrapped err")
	}

	// Nil receiver.
	var nilErr *ReasonError
	if nilErr.Error() != "" {
		t.Fatal("expected empty string for nil ReasonError")
	}
}

func TestReasonErrorUnwrap(t *testing.T) {
	t.Parallel()

	inner := errors.New("inner")
	e := &ReasonError{Code: "code", Message: "msg", Err: inner}
	if !errors.Is(e, inner) {
		t.Fatal("Unwrap should expose inner error")
	}

	// Nil receiver.
	var nilErr *ReasonError
	if nilErr.Unwrap() != nil {
		t.Fatal("nil ReasonError.Unwrap() should return nil")
	}
}

func TestFirstCriticalFindingNoCritical(t *testing.T) {
	t.Parallel()

	findings := []Finding{
		{Severity: "warning", Code: "w1", Message: "some warning"},
		{Severity: "info", Code: "i1", Message: "some info"},
	}
	if firstCriticalFinding(findings) != nil {
		t.Fatal("expected nil when no critical findings present")
	}
}

func TestDefaultIsAdminReturnsFalseOnNonWindows(t *testing.T) {
	t.Parallel()

	// On Linux (the CI host) defaultIsAdmin always returns false.
	if defaultIsAdmin() {
		t.Fatal("defaultIsAdmin should return false on non-Windows")
	}
}
