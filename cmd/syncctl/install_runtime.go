package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/install"
)

type serviceInstaller interface {
	Install(mode install.Mode) error
	Uninstall(mode install.Mode) error
}

type serviceInstallerFactory func(binaryPath, configPath string) (serviceInstaller, error)

var newServiceInstaller serviceInstallerFactory = defaultServiceInstaller

func defaultServiceInstaller(binaryPath, configPath string) (serviceInstaller, error) {
	switch runtime.GOOS {
	case "linux":
		return install.NewLinuxSystemdInstaller(binaryPath, configPath), nil
	case "windows":
		return install.NewWindowsServiceInstaller(binaryPath, configPath), nil
	default:
		return nil, fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
}

func resolveInstallMode(userFlag, systemFlag bool) (install.Mode, error) {
	if userFlag && systemFlag {
		return "", fmt.Errorf("flags --user and --system are mutually exclusive")
	}
	if systemFlag {
		return install.ModeSystem, nil
	}
	return install.ModeUser, nil
}

func resolveInstallPaths(mode install.Mode, binaryPath, configPath string) (string, string, error) {
	defaultBinaryPath, defaultConfigPath, ok := defaultInstallPaths(mode)
	if !ok {
		return "", "", fmt.Errorf("unsupported mode %q", mode)
	}

	binaryPath = strings.TrimSpace(binaryPath)
	configPath = strings.TrimSpace(configPath)
	if binaryPath == "" {
		binaryPath = defaultBinaryPath
	}
	if configPath == "" {
		configPath = defaultConfigPath
	}

	return binaryPath, configPath, nil
}

func ensureConfigExists(configPath string) error {
	if strings.TrimSpace(configPath) == "" {
		return fmt.Errorf("config path must not be empty")
	}

	if _, err := os.Stat(configPath); err == nil {
		_, loadErr := config.Load(configPath)
		return loadErr
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := config.Save(configPath, config.Default()); err != nil {
		return err
	}
	return nil
}

// tryElevateForInstall attempts to re-launch the current process with elevated
// privileges when the current operation requires them.
//
//   - Windows: always needs Administrator (sc.exe create requires it regardless
//     of user/system mode), so elevation is attempted unconditionally.
//   - Linux: only system-mode installs need root. Elevation is attempted when
//     --system is present in os.Args.
//
// If relaunched is true, the caller must exit immediately — the elevated child
// has already performed the work.
func tryElevateForInstall() (relaunched bool, err error) {
	switch runtime.GOOS {
	case "windows":
		return install.TryElevate(os.Args[1:])
	case "linux":
		// Only elevate when --system is explicitly requested.
		for _, arg := range os.Args[1:] {
			if arg == "--system" {
				return install.TryElevate(os.Args[1:])
			}
		}
		return false, nil
	default:
		return false, nil
	}
}
