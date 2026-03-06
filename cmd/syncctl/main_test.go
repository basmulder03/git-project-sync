package main

import (
	"errors"
	"testing"
)

func TestClassifyExitCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", err: nil, want: 0},
		{name: "unknown command", err: errors.New("unknown command \"oops\""), want: 2},
		{name: "required argument", err: errors.New("required argument"), want: 2},
		{name: "runtime", err: errors.New("open sqlite db: permission denied"), want: 1},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyExitCode(tc.err); got != tc.want {
				t.Fatalf("classifyExitCode() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestFormatCLIError(t *testing.T) {
	t.Parallel()

	if got := formatCLIError(errors.New("problem")); got != "error: problem" {
		t.Fatalf("formatCLIError() = %q", got)
	}
}
