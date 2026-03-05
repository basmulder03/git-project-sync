//go:build !windows

package install

func defaultIsAdmin() bool {
	return false
}
