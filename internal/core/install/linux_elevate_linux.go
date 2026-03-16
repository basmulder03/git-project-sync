//go:build linux

package install

import (
	"fmt"
	"os"
	"os/exec"
)

// TryElevate attempts to re-launch the current process with root privileges
// on Linux. It returns (true, nil) when it successfully re-launched an
// elevated child and the caller should exit. It returns (false, nil) when the
// current process is already root (no action taken).
//
// Elevation order:
//  1. sudo  – available on virtually all Linux distributions.
//  2. pkexec – PolicyKit; available on desktop distributions when sudo is absent.
//
// On Linux, elevation is only required for system-mode installs (root needed
// to write to /etc/systemd/system). User-mode installs never need elevation.
func TryElevate(args []string) (relaunched bool, err error) {
	if os.Geteuid() == 0 {
		// Already root – nothing to do.
		return false, nil
	}

	self, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("resolve current executable: %w", err)
	}

	// 1. Try sudo.
	if sudo, lookErr := exec.LookPath("sudo"); lookErr == nil {
		cmdArgs := append([]string{self}, args...)
		cmd := exec.Command(sudo, cmdArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if runErr := cmd.Run(); runErr != nil {
			return false, fmt.Errorf("sudo elevation failed: %w", runErr)
		}
		return true, nil
	}

	// 2. Try pkexec (PolicyKit – graphical auth prompt on desktop distros).
	if pkexec, lookErr := exec.LookPath("pkexec"); lookErr == nil {
		cmdArgs := append([]string{self}, args...)
		cmd := exec.Command(pkexec, cmdArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if runErr := cmd.Run(); runErr != nil {
			return false, fmt.Errorf("pkexec elevation failed: %w", runErr)
		}
		return true, nil
	}

	return false, fmt.Errorf("cannot elevate privileges: neither sudo nor pkexec found in PATH; rerun as root")
}
