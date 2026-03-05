package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	lookPath         func(file string) (string, error)
	stat             func(name string) (os.FileInfo, error)
	goos             string
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
		geteuid:  os.Geteuid,
		homedir:  os.UserHomeDir,
		lookPath: exec.LookPath,
		stat:     os.Stat,
		goos:     runtime.GOOS,
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
		return &ReasonError{Code: ReasonInstallServiceDirCreateFailed, Message: "failed to create service directory", Hint: "check directory permissions and parent path", Err: err}
	}

	if err := os.WriteFile(servicePath, []byte(i.serviceUnit()), 0o644); err != nil {
		return &ReasonError{Code: ReasonInstallServiceWriteFailed, Message: "failed to write service unit", Hint: "verify target directory is writable", Err: err}
	}

	if err := i.run("systemctl", append(userFlag, "daemon-reload")...); err != nil {
		return &ReasonError{Code: ReasonInstallRegistrationFailed, Message: "failed to reload systemd units", Hint: "confirm systemd is available and active", Err: err}
	}
	if err := i.run("systemctl", append(userFlag, "enable", "--now", i.unitName())...); err != nil {
		return &ReasonError{Code: ReasonInstallRegistrationFailed, Message: "failed to enable/start service", Hint: "run `systemctl status` for details and verify permissions", Err: err}
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
		return &ReasonError{Code: ReasonInstallCleanupFailed, Message: "failed to remove service unit", Hint: "remove the unit file manually and rerun uninstall", Err: err}
	}

	return nil
}

func (i *LinuxSystemdInstaller) validate(mode Mode) error {
	if finding := firstCriticalFinding(i.Preflight(mode)); finding != nil {
		return &ReasonError{Code: ReasonInstallValidationFailed, Message: "install preflight failed", Hint: finding.Hint, Err: finding}
	}
	return nil
}

func (i *LinuxSystemdInstaller) Preflight(mode Mode) []Finding {
	findings := make([]Finding, 0, 6)
	if i.goos != "linux" {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallUnsupportedEnvironment, Message: "linux systemd installer is unsupported on this OS", Hint: "run Linux installer on a Linux host"})
	}
	if mode != ModeUser && mode != ModeSystem {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallInvalidMode, Message: fmt.Sprintf("invalid install mode %q", mode), Hint: "use user or system mode"})
	}
	if strings.TrimSpace(i.BinaryPath) == "" {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallMissingBinaryPath, Message: "binary path is required", Hint: "set a valid syncd binary path"})
	} else if _, err := i.stat(i.BinaryPath); err != nil {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallBinaryMissing, Message: "syncd binary is missing", Hint: "install or download syncd before registering service"})
	}
	if strings.TrimSpace(i.ConfigPath) == "" {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallMissingConfigPath, Message: "config path is required", Hint: "set a valid config path"})
	}
	if _, err := i.lookPath("systemctl"); err != nil {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallDependencyMissing, Message: "missing required dependency: systemctl", Hint: "install/enable systemd before registration"})
	}
	if mode == ModeSystem && i.geteuid() != 0 {
		findings = append(findings, Finding{Severity: "critical", Code: ReasonInstallInsufficientPrivileges, Message: "system mode requires root privileges", Hint: "rerun with sudo or use user mode"})
	}
	return findings
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
