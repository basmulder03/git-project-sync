package install

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type WindowsTaskSchedulerInstaller struct {
	TaskName   string
	BinaryPath string
	ConfigPath string
	run        func(name string, args ...string) error
	isAdmin    func() bool
}

func NewWindowsTaskSchedulerInstaller(binaryPath, configPath string) *WindowsTaskSchedulerInstaller {
	return &WindowsTaskSchedulerInstaller{
		TaskName:   "GitProjectSync",
		BinaryPath: binaryPath,
		ConfigPath: configPath,
		run: func(name string, args ...string) error {
			cmd := exec.Command(name, args...)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s %s failed: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
			}
			return nil
		},
		isAdmin: func() bool { return strings.EqualFold(os.Getenv("USERNAME"), "Administrator") },
	}
}

func (i *WindowsTaskSchedulerInstaller) Install(mode Mode) error {
	if err := i.validate(mode); err != nil {
		return err
	}

	command := fmt.Sprintf("\"%s\" --config \"%s\"", i.BinaryPath, i.ConfigPath)
	args := []string{"/Create", "/F", "/SC", "MINUTE", "/MO", "5", "/TN", i.TaskName, "/TR", command}
	if mode == ModeSystem {
		args = append(args, "/RL", "HIGHEST", "/RU", "SYSTEM")
	} else {
		args = append(args, "/RL", "LIMITED")
	}

	if err := i.run("schtasks", args...); err != nil {
		return err
	}

	if err := i.run("schtasks", "/Query", "/TN", i.TaskName); err != nil {
		return fmt.Errorf("task registration validation failed: %w", err)
	}

	return nil
}

func (i *WindowsTaskSchedulerInstaller) Uninstall(mode Mode) error {
	if err := i.validate(mode); err != nil {
		return err
	}

	_ = i.run("schtasks", "/Delete", "/F", "/TN", i.TaskName)
	return nil
}

func (i *WindowsTaskSchedulerInstaller) validate(mode Mode) error {
	if mode != ModeUser && mode != ModeSystem {
		return fmt.Errorf("invalid install mode %q", mode)
	}
	if i.BinaryPath == "" {
		return fmt.Errorf("binary path is required")
	}
	if i.ConfigPath == "" {
		return fmt.Errorf("config path is required")
	}
	if mode == ModeSystem && !i.isAdmin() {
		return fmt.Errorf("system mode requires administrative privileges")
	}
	return nil
}
