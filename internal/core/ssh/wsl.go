package ssh

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// IsWSL reports whether the current process is running inside Windows Subsystem
// for Linux (WSL 1 or WSL 2).
//
// Detection strategy (in order):
//  1. WSL_DISTRO_NAME environment variable — set by WSL for all processes.
//  2. WSL_INTEROP environment variable — set in WSL 2 for interop socket.
//  3. Presence of /proc/version containing "microsoft" or "WSL".
//
// Returns false on native Windows (runtime.GOOS == "windows") even if
// WSL_DISTRO_NAME is somehow set.
func IsWSL() bool {
	if runtime.GOOS == "windows" {
		return false
	}

	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	if os.Getenv("WSL_INTEROP") != "" {
		return true
	}

	// Fallback: read /proc/version.
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}

// WindowsHomeDir returns the Windows user home directory as a WSL path
// (e.g. "/mnt/c/Users/Alice").
//
// It first tries USERPROFILE via wslpath, then falls back to deriving the path
// from the Windows username via WSLENV/LOGNAME heuristics.
//
// Returns ("", false) if the path cannot be determined.
func WindowsHomeDir() (string, bool) {
	// Ask wslpath to convert %USERPROFILE% to a Linux path.
	// USERPROFILE is not available in WSL by default unless WSLENV includes it.
	// We call cmd.exe /c echo %USERPROFILE% instead.
	out, err := exec.Command("cmd.exe", "/c", "echo %USERPROFILE%").Output()
	if err == nil {
		winPath := strings.TrimSpace(string(out))
		if winPath != "" && winPath != "%USERPROFILE%" {
			if linuxPath, ok := WindowsPathToWSL(winPath); ok {
				return linuxPath, true
			}
		}
	}

	// Fallback: use wslpath on a known Windows env variable via PowerShell.
	out2, err2 := exec.Command("powershell.exe", "-NoProfile", "-Command",
		"[Environment]::GetFolderPath('UserProfile')").Output()
	if err2 == nil {
		winPath := strings.TrimSpace(string(out2))
		if winPath != "" {
			if linuxPath, ok := WindowsPathToWSL(winPath); ok {
				return linuxPath, true
			}
		}
	}

	return "", false
}

// WindowsPathToWSL converts a Windows absolute path (e.g. "C:\Users\Alice\foo")
// to the corresponding WSL mount path (e.g. "/mnt/c/Users/Alice/foo") using
// the wslpath utility.
//
// Returns ("", false) if wslpath is unavailable or the conversion fails.
func WindowsPathToWSL(winPath string) (string, bool) {
	out, err := exec.Command("wslpath", "-u", winPath).Output()
	if err != nil {
		// Manual fallback: handle simple drive-letter paths.
		return manualWindowsToWSL(winPath)
	}
	result := strings.TrimSpace(string(out))
	if result == "" {
		return "", false
	}
	return result, true
}

// WSLPathToWindows converts a WSL/Linux absolute path (e.g. "/mnt/c/Users/Alice")
// to the corresponding Windows path (e.g. "C:\Users\Alice") using wslpath.
//
// Returns ("", false) if wslpath is unavailable or the conversion fails.
func WSLPathToWindows(wslPath string) (string, bool) {
	out, err := exec.Command("wslpath", "-w", wslPath).Output()
	if err != nil {
		return manualWSLToWindows(wslPath)
	}
	result := strings.TrimSpace(string(out))
	if result == "" {
		return "", false
	}
	return result, true
}

// manualWindowsToWSL converts a Windows path to a WSL path without wslpath.
// Handles paths of the form "X:\..." → "/mnt/x/...".
func manualWindowsToWSL(winPath string) (string, bool) {
	if len(winPath) < 3 {
		return "", false
	}
	driveLetter := winPath[0]
	if winPath[1] != ':' {
		return "", false
	}
	rest := winPath[2:]
	rest = strings.ReplaceAll(rest, "\\", "/")
	rest = strings.TrimPrefix(rest, "/")
	return "/mnt/" + strings.ToLower(string(driveLetter)) + "/" + rest, true
}

// manualWSLToWindows converts a /mnt/<drive>/... path to Windows form.
func manualWSLToWindows(wslPath string) (string, bool) {
	if !strings.HasPrefix(wslPath, "/mnt/") {
		return "", false
	}
	rest := strings.TrimPrefix(wslPath, "/mnt/")
	if len(rest) < 1 {
		return "", false
	}
	drive := strings.ToUpper(string(rest[0]))
	tail := rest[1:]
	tail = strings.ReplaceAll(tail, "/", "\\")
	tail = strings.TrimPrefix(tail, "\\")
	if tail == "" {
		return drive + ":\\", true
	}
	return drive + ":\\" + tail, true
}

// WindowsSSHConfigPath returns the path to the Windows user SSH config file,
// expressed as a WSL path (e.g. "/mnt/c/Users/Alice/.ssh/config").
//
// Returns ("", false) if the Windows home directory cannot be determined.
func WindowsSSHConfigPath() (string, bool) {
	winHome, ok := WindowsHomeDir()
	if !ok {
		return "", false
	}
	return filepath.Join(winHome, ".ssh", "config"), true
}

// WindowsSSHDir returns the default SSH key directory on the Windows side,
// expressed as a WSL path (e.g. "/mnt/c/Users/Alice/AppData/Local/git-project-sync/ssh").
//
// Returns ("", false) if the Windows home directory cannot be determined.
func WindowsSSHDir() (string, bool) {
	winHome, ok := WindowsHomeDir()
	if !ok {
		return "", false
	}
	return filepath.Join(winHome, "AppData", "Local", "git-project-sync", "ssh"), true
}
