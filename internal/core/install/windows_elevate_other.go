//go:build !windows && !linux

package install

import "errors"

// TryElevate is a no-op on unsupported platforms. It always returns false and
// an error indicating that elevation is not applicable.
func TryElevate(args []string) (relaunched bool, err error) {
	return false, errors.New("elevation is not supported on this platform")
}
