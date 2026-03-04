package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Mode string

const (
	ModeUser   Mode = "user"
	ModeSystem Mode = "system"
)

type LinuxSystemdInstaller struct {
	ServiceName      string
	BinaryPath       string
	ConfigPath       string
	UserServiceDir   string
	SystemServiceDir string
	run              func(name string, args ...string) error
	geteuid          func() int
	homedir          func() (string, error)
}

func NewLinuxSystemdInstaller(binaryPath, configPath string) *LinuxSystemdInstaller {
	return &LinuxSystemdInstaller{
		ServiceName:      "git-project-sync",
		BinaryPath:       binaryPath,
		ConfigPath:       configPath,
		UserServiceDir:   "",
		SystemServiceDir: "/etc/systemd/system",
		run: func(name string, args ...string) error {
			cmd := exec.Command(name, args...)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s %s failed: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
			}
			return nil
		},
		geteuid: os.Geteuid,
		homedir: os.UserHomeDir,
	}
}

func (i *LinuxSystemdInstaller) Install(mode Mode) error {
	if err := i.validate(mode); err != nil {
		return err
	}

	servicePath, userFlag, err := i.servicePath(mode)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(servicePath), 0o755); err != nil {
		return fmt.Errorf("create service directory: %w", err)
	}

	if err := os.WriteFile(servicePath, []byte(i.serviceUnit()), 0o644); err != nil {
		return fmt.Errorf("write service unit: %w", err)
	}

	if err := i.run("systemctl", append(userFlag, "daemon-reload")...); err != nil {
		return err
	}
	if err := i.run("systemctl", append(userFlag, "enable", "--now", i.unitName())...); err != nil {
		return err
	}

	return nil
}

func (i *LinuxSystemdInstaller) Uninstall(mode Mode) error {
	if err := i.validate(mode); err != nil {
		return err
	}

	servicePath, userFlag, err := i.servicePath(mode)
	if err != nil {
		return err
	}

	_ = i.run("systemctl", append(userFlag, "disable", "--now", i.unitName())...)
	_ = i.run("systemctl", append(userFlag, "daemon-reload")...)

	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove service unit: %w", err)
	}

	return nil
}

func (i *LinuxSystemdInstaller) validate(mode Mode) error {
	if mode != ModeUser && mode != ModeSystem {
		return fmt.Errorf("invalid install mode %q", mode)
	}
	if i.BinaryPath == "" {
		return fmt.Errorf("binary path is required")
	}
	if i.ConfigPath == "" {
		return fmt.Errorf("config path is required")
	}
	if mode == ModeSystem && i.geteuid() != 0 {
		return fmt.Errorf("system mode requires root privileges")
	}
	return nil
}

func (i *LinuxSystemdInstaller) servicePath(mode Mode) (string, []string, error) {
	unit := i.unitName()
	if mode == ModeSystem {
		return filepath.Join(i.SystemServiceDir, unit), []string{}, nil
	}

	if i.UserServiceDir != "" {
		return filepath.Join(i.UserServiceDir, unit), []string{"--user"}, nil
	}

	home, err := i.homedir()
	if err != nil {
		return "", nil, fmt.Errorf("resolve home dir: %w", err)
	}

	return filepath.Join(home, ".config", "systemd", "user", unit), []string{"--user"}, nil
}

func (i *LinuxSystemdInstaller) unitName() string {
	name := strings.TrimSpace(i.ServiceName)
	if name == "" {
		name = "git-project-sync"
	}
	return name + ".service"
}

func (i *LinuxSystemdInstaller) serviceUnit() string {
	return fmt.Sprintf(`[Unit]
Description=Git Project Sync daemon
After=network-online.target

[Service]
Type=simple
ExecStart=%s --config %s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`, i.BinaryPath, i.ConfigPath)
}
