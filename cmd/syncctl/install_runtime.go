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

// tryElevateForInstall attempts to re-launch the current process with
// Administrator privileges on Windows. If relaunched is true the caller must
// exit immediately (the elevated child has already done the work). On
// non-Windows platforms this is a no-op that always returns false.
func tryElevateForInstall() (relaunched bool, err error) {
	if runtime.GOOS != "windows" {
		return false, nil
	}
	return install.TryElevate(os.Args[1:])
}
