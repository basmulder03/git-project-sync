//go:build !windows

package main

import "context"

// maybeRunAsService is a no-op on non-Windows platforms – the daemon is managed
// by systemd (or run manually) and has no service dispatcher to integrate with.
func maybeRunAsService(_ func(ctx context.Context) int) (bool, int) {
	return false, 0
}
