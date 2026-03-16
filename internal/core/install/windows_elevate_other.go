//go:build !windows

package install

import "errors"

// TryElevate is a no-op on non-Windows platforms. It always returns false and
// an error indicating that elevation is not applicable.
func TryElevate(args []string) (relaunched bool, err error) {
	return false, errors.New("elevation is only supported on Windows")
}
