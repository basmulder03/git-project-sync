package install

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const windowsServiceName = "GitProjectSync"

// WindowsServiceInstaller installs syncd as a proper Windows Service via sc.exe.
// Running as a Windows Service avoids the cmd-window flash that occurred with the
// previous Task Scheduler approach, because services run in Session 0 with no
// visible console.
type WindowsServiceInstaller struct {
	ServiceName string
	BinaryPath  string
	ConfigPath  string
	run         func(name string, args ...string) (string, error)
	isAdmin     func() bool
	lookPath    func(file string) (string, error)
	stat        func(name string) (os.FileInfo, error)
	goos        string
}

func NewWindowsServiceInstaller(binaryPath, configPath string) *WindowsServiceInstaller {
	return &WindowsServiceInstaller{
		ServiceName: windowsServiceName,
		BinaryPath:  binaryPath,
		ConfigPath:  configPath,
		run: func(name string, args ...string) (string, error) {
			cmd := exec.Command(name, args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("%s %s failed: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
			}
			return string(out), nil
		},
		isAdmin:  defaultIsAdmin,
		lookPath: exec.LookPath,
		stat:     os.Stat,
		goos:     runtime.GOOS,
	}
}

// migrateFromTaskScheduler removes a legacy Windows Task Scheduler job with
// the given task name if one exists. This is called automatically by Install to
// ensure a smooth upgrade from the old task-based approach to Windows Services.
// Errors are logged but do not block the installation.
func (i *WindowsServiceInstaller) migrateFromTaskScheduler(taskName string) {
	// schtasks /Query exits non-zero when the task does not exist, so a non-nil
	// error here simply means there is nothing to migrate.
	if _, err := i.run("schtasks", "/Query", "/TN", taskName); err != nil {
		return
	}
	// Task exists – remove it.
	_, _ = i.run("schtasks", "/Delete", "/F", "/TN", taskName)
}

func (i *WindowsServiceInstaller) Install(mode Mode) error {
	if err := i.validate(mode); err != nil {
		return err
	}

	// Migrate any pre-existing legacy Task Scheduler job so the user doesn't
	// end up with both the old scheduler task and the new Windows Service
	// running at the same time.
	i.migrateFromTaskScheduler(i.ServiceName)

	// Build the service binary path argument: sc.exe requires the full quoted
	// command including any flags.
	binCmd := fmt.Sprintf("\"%s\" --config \"%s\"", i.BinaryPath, i.ConfigPath)

	startType := "auto"
	if mode == ModeUser {
		// "auto" delayed start keeps the service running persistently but waits
		// for the desktop session; use "demand" for user-mode installs so the
		// user can start/stop it explicitly without needing admin.
		// We still use LOCAL_SERVICE for system installs.
		startType = "demand"
	}

	// Create the service. sc.exe exits non-zero on failure.
	args := []string{
		"create", i.ServiceName,
		"binPath=", binCmd,
		"start=", startType,
		"DisplayName=", "Git Project Sync",
	}
	if mode == ModeSystem {
		args = append(args, "obj=", "LocalService")
	}

	if _, err := i.run("sc.exe", args...); err != nil {
		return &ReasonError{
			Code:    ReasonInstallRegistrationFailed,
			Message: "failed to create Windows service",
			Hint:    "confirm you are running as Administrator and the service does not already exist (use 'sc.exe delete " + i.ServiceName + "' to remove it)",
			Err:     err,
		}
	}

	// Set a description for the service entry.
	if _, err := i.run("sc.exe", "description", i.ServiceName, "Keeps local git repositories in sync with their remote default branch"); err != nil {
		// Non-fatal – the service still works without a description.
		_ = err
	}

	// Start the service immediately after installation.
	if _, err := i.run("sc.exe", "start", i.ServiceName); err != nil {
		return &ReasonError{
			Code:    ReasonInstallRegistrationFailed,
			Message: "service created but failed to start",
			Hint:    "run 'sc.exe start " + i.ServiceName + "' manually or check the Windows Event Log for details",
			Err:     err,
		}
	}

	// Verify it is running.
	if _, err := i.run("sc.exe", "query", i.ServiceName); err != nil {
		return &ReasonError{
			Code:    ReasonInstallValidationFailed,
			Message: "service registration validation failed",
			Hint:    "verify service exists via 'sc.exe query " + i.ServiceName + "'",
			Err:     err,
		}
	}

	return nil
}

func (i *WindowsServiceInstaller) Uninstall(mode Mode) error {
	if err := i.validate(mode); err != nil {
		return err
	}

	// Stop the service first (ignore errors – it may already be stopped).
	_, _ = i.run("sc.exe", "stop", i.ServiceName)

	// Delete the service entry.
	_, _ = i.run("sc.exe", "delete", i.ServiceName)

	return nil
}

func (i *WindowsServiceInstaller) validate(mode Mode) error {
	if finding := firstCriticalFinding(i.Preflight(mode)); finding != nil {
		return &ReasonError{Code: ReasonInstallValidationFailed, Message: "install preflight failed", Hint: finding.Hint, Err: finding}
	}
	return nil
}

// Preflight returns a list of findings that must all be non-critical before
// Install or Uninstall may proceed.
func (i *WindowsServiceInstaller) Preflight(mode Mode) []Finding {
	findings := make([]Finding, 0, 6)

	if i.goos != "windows" {
		findings = append(findings, Finding{
			Severity: "critical",
			Code:     ReasonInstallUnsupportedEnvironment,
			Message:  "windows service installer is unsupported on this OS",
			Hint:     "run the Windows installer on a Windows host",
		})
	}
	if mode != ModeUser && mode != ModeSystem {
		findings = append(findings, Finding{
			Severity: "critical",
			Code:     ReasonInstallInvalidMode,
			Message:  fmt.Sprintf("invalid install mode %q", mode),
			Hint:     "use user or system mode",
		})
	}
	if strings.TrimSpace(i.BinaryPath) == "" {
		findings = append(findings, Finding{
			Severity: "critical",
			Code:     ReasonInstallMissingBinaryPath,
			Message:  "binary path is required",
			Hint:     "set a valid syncd binary path",
		})
	} else if _, err := i.stat(i.BinaryPath); err != nil {
		findings = append(findings, Finding{
			Severity: "critical",
			Code:     ReasonInstallBinaryMissing,
			Message:  "syncd binary is missing",
			Hint:     "install or download syncd before registering the service",
		})
	}
	if strings.TrimSpace(i.ConfigPath) == "" {
		findings = append(findings, Finding{
			Severity: "critical",
			Code:     ReasonInstallMissingConfigPath,
			Message:  "config path is required",
			Hint:     "set a valid config path",
		})
	}
	if _, err := i.lookPath("sc.exe"); err != nil {
		findings = append(findings, Finding{
			Severity: "critical",
			Code:     ReasonInstallDependencyMissing,
			Message:  "missing required dependency: sc.exe",
			Hint:     "sc.exe is part of Windows – verify it is available in PATH",
		})
	}
	// Both user-mode service creation and system-mode require admin on Windows.
	if !i.isAdmin() {
		findings = append(findings, Finding{
			Severity: "critical",
			Code:     ReasonInstallInsufficientPrivileges,
			Message:  "creating a Windows service requires administrative privileges",
			Hint:     "rerun in an elevated (Administrator) shell",
		})
	}

	return findings
}
