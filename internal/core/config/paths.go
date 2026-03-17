package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// DefaultDataDir returns the platform-specific default directory for application data
// Windows: %LOCALAPPDATA%\git-project-sync
// Linux/macOS: ~/.local/share/git-project-sync
func DefaultDataDir() string {
	if runtime.GOOS == "windows" {
		// Use LOCALAPPDATA for machine-specific data (not roaming)
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "git-project-sync")
		}
		if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
			return filepath.Join(userProfile, "AppData", "Local", "git-project-sync")
		}
	}

	// XDG Base Directory Specification for Linux
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "git-project-sync")
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "git-project-sync")
	}

	if runtime.GOOS == "darwin" {
		// macOS: ~/Library/Application Support
		return filepath.Join(home, "Library", "Application Support", "git-project-sync")
	}

	// Linux default: ~/.local/share
	return filepath.Join(home, ".local", "share", "git-project-sync")
}

// DefaultDBPath returns the default absolute path for the state database
func DefaultDBPath() string {
	return filepath.Join(DefaultDataDir(), "state", "sync.db")
}

// DefaultSSHDir returns the default directory where managed SSH keys are stored.
// Windows: %LOCALAPPDATA%\git-project-sync\ssh
// Linux/macOS: ~/.local/share/git-project-sync/ssh
func DefaultSSHDir() string {
	return filepath.Join(DefaultDataDir(), "ssh")
}

// DefaultSSHConfigPath returns the path to the user SSH config file.
// Windows: %USERPROFILE%\.ssh\config
// Linux/macOS: ~/.ssh/config
func DefaultSSHConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".ssh", "config")
}

// SSHDir returns the effective SSH key directory: the value from SSHConfig.KeyDir
// if set, otherwise the platform default.
func (c *Config) SSHDir() string {
	if c.SSH.KeyDir != "" {
		return c.SSH.KeyDir
	}
	return DefaultSSHDir()
}

// SSHConfigPath returns the effective SSH config file path.
func (c *Config) SSHConfigPath() string {
	if c.SSH.SSHConfigPath != "" {
		return c.SSH.SSHConfigPath
	}
	return DefaultSSHConfigPath()
}

// SSHEnabledForSource returns whether SSH is enabled for a specific source,
// falling back to the global SSH.Enabled flag.
func (c *Config) SSHEnabledForSource(src SourceConfig) bool {
	if src.SSH.Enabled != nil {
		return *src.SSH.Enabled
	}
	return c.SSH.Enabled
}

// WSLSyncToWindows reports whether the WSL→Windows SSH config sync is enabled.
// Defaults to true (opt-out) when running in WSL so that the Windows git client
// gets the same Host blocks without manual configuration.
func (c *Config) WSLSyncToWindows() bool {
	if c.SSH.WSL.SyncToWindows != nil {
		return *c.SSH.WSL.SyncToWindows
	}
	// Default: enabled (opt-out model).
	return true
}

// WSLUseWindowsKeyDir reports whether SSH private keys should be stored in the
// Windows key directory (accessible from both WSL and native Windows).
// Defaults to true when running in WSL.
func (c *Config) WSLUseWindowsKeyDir() bool {
	if c.SSH.WSL.UseWindowsKeyDir != nil {
		return *c.SSH.WSL.UseWindowsKeyDir
	}
	return true
}

// WSLWindowsKeyDir returns the effective Windows SSH key directory expressed as
// a WSL path (e.g. "/mnt/c/Users/Alice/AppData/Local/git-project-sync/ssh").
// Returns "" if it cannot be determined (wslpath unavailable, not in WSL, etc.)
func (c *Config) WSLWindowsKeyDir() string {
	if c.SSH.WSL.WindowsKeyDir != "" {
		return c.SSH.WSL.WindowsKeyDir
	}
	// Dynamic detection is done by the ssh package to avoid import cycles.
	// Callers that need this should use ssh.WindowsSSHDir() directly.
	return ""
}

// WSLWindowsSSHConfigPath returns the effective Windows SSH config path expressed
// as a WSL path (e.g. "/mnt/c/Users/Alice/.ssh/config").
// Returns "" if it cannot be determined.
func (c *Config) WSLWindowsSSHConfigPath() string {
	if c.SSH.WSL.WindowsSSHConfigPath != "" {
		return c.SSH.WSL.WindowsSSHConfigPath
	}
	// Dynamic detection is done by the ssh package.
	return ""
}
