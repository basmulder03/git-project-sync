package install

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type WindowsTaskSchedulerInstaller struct {
	TaskName   string
	BinaryPath string
	ConfigPath string
	run        func(name string, args ...string) error
	isAdmin    func() bool
	lookPath   func(file string) (string, error)
	stat       func(name string) (os.FileInfo, error)
	goos       string
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
		isAdmin:  defaultIsAdmin,
		lookPath: exec.LookPath,
		stat:     os.Stat,
		goos:     runtime.GOOS,
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
		return &ReasonError{Code: ReasonInstallRegistrationFailed, Message: "failed to register scheduled task", Hint: "confirm Task Scheduler service is running and permissions are sufficient", Err: err}
	}

	if err := i.run("schtasks", "/Query", "/TN", i.TaskName); err != nil {
		return &ReasonError{Code: ReasonInstallValidationFailed, Message: "task registration validation failed", Hint: "verify scheduled task exists and is queryable", Err: err}
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
	if finding := firstCriticalFinding(i.Preflight(mode)); finding != nil {
		return &ReasonError{Code: ReasonInstallValidationFailed, Message: "install preflight failed", Hint: finding.Hint, Err: finding}
	}
	return nil
}

func (i *WindowsTaskSchedulerInstaller) Preflight(mode Mode) []Finding {
	findings := make([]Finding, 0, 6)
	if i.goos != "windows" {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallUnsupportedEnvironment, Message: "windows task scheduler installer is unsupported on this OS", Hint: "run Windows installer on a Windows host"})
	}
	if mode != ModeUser && mode != ModeSystem {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallInvalidMode, Message: fmt.Sprintf("invalid install mode %q", mode), Hint: "use user or system mode"})
	}
	if strings.TrimSpace(i.BinaryPath) == "" {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallMissingBinaryPath, Message: "binary path is required", Hint: "set a valid syncd binary path"})
	} else if _, err := i.stat(i.BinaryPath); err != nil {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallBinaryMissing, Message: "syncd binary is missing", Hint: "install or download syncd before registering task"})
	}
	if strings.TrimSpace(i.ConfigPath) == "" {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallMissingConfigPath, Message: "config path is required", Hint: "set a valid config path"})
	}
	if _, err := i.lookPath("schtasks"); err != nil {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallDependencyMissing, Message: "missing required dependency: schtasks", Hint: "ensure Task Scheduler tools are available in PATH"})
	}
	if mode == ModeSystem && !i.isAdmin() {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallInsufficientPrivileges, Message: "system mode requires administrative privileges", Hint: "rerun in elevated shell or use user mode"})
	}
	return findings
}
