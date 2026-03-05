//go:build windows

package install

import (
	"os/exec"
	"strings"
)

func defaultIsAdmin() bool {
	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		"([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(string(output)), "True")
}
