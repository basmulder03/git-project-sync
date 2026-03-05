package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/basmulder03/git-project-sync/internal/core/install"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		fmt.Fprintf(os.Stderr, "syncsetup: unsupported OS %s\n", runtime.GOOS)
		return 1
	}

	fs := flag.NewFlagSet("syncsetup", flag.ContinueOnError)
	showAppVersion := fs.Bool("app-version", false, "Show syncsetup version and exit")
	releaseTag := fs.String("version", "latest", "Release tag to use for bootstrap downloads")
	repo := fs.String("repo", "basmulder03/git-project-sync", "GitHub repository owner/name")
	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "syncsetup: %v\n", err)
		return 2
	}

	if *showAppVersion {
		fmt.Printf("syncsetup %s\n", version)
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := &setupApp{
		in:      os.Stdin,
		out:     os.Stdout,
		mode:    install.ModeUser,
		version: *releaseTag,
		repo:    *repo,
		appVer:  version,
	}

	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "syncsetup: %v\n", err)
		return 1
	}

	return 0
}

type setupApp struct {
	in      io.Reader
	out     io.Writer
	mode    install.Mode
	version string
	repo    string
	appVer  string

	mu      sync.Mutex
	running bool
	message string
	logs    []string
	pending *downgradePrompt
}

type downgradePrompt struct {
	mode      install.Mode
	installed string
	target    string
	repair    bool
}

func (a *setupApp) Run(ctx context.Context) error {
	a.render()

	keys := make(chan string, 8)
	go a.readKeys(ctx, keys)

	for {
		select {
		case <-ctx.Done():
			return nil
		case key := <-keys:
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "q":
				return nil
			case "m":
				a.toggleMode()
				a.render()
			case "i":
				a.startOperation(ctx, "install/register", a.installOnly)
			case "u":
				a.startOperation(ctx, "uninstall/unregister", a.uninstallOnly)
			case "b":
				a.startOperation(ctx, "bootstrap+install", a.bootstrapAndInstall)
			case "r":
				a.startOperation(ctx, "repair/reinstall", a.repairAndReinstall)
			case "y":
				a.confirmPendingDowngrade(ctx)
			default:
				a.setMessage("unknown key")
				a.render()
			}
		}
	}
}

func (a *setupApp) toggleMode() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mode == install.ModeUser {
		a.mode = install.ModeSystem
	} else {
		a.mode = install.ModeUser
	}
	a.message = fmt.Sprintf("mode changed to %s", a.mode)
}

func (a *setupApp) startOperation(ctx context.Context, label string, fn func(context.Context, install.Mode) error) {
	a.mu.Lock()
	mode := a.mode
	a.mu.Unlock()
	a.startOperationForMode(ctx, label, mode, fn)
}

func (a *setupApp) startOperationForMode(ctx context.Context, label string, mode install.Mode, fn func(context.Context, install.Mode) error) {
	a.mu.Lock()
	if a.running {
		a.message = "operation already running"
		a.mu.Unlock()
		a.render()
		return
	}
	a.running = true
	a.message = "running " + label + "..."
	a.mu.Unlock()
	a.render()

	go func() {
		err := fn(ctx, mode)
		a.mu.Lock()
		a.running = false
		if err != nil {
			var downgradeErr *downgradeConfirmationError
			if errors.As(err, &downgradeErr) {
				a.message = downgradeErr.Error()
				a.logs = append([]string{fmt.Sprintf("%s pending confirmation (%s)", label, mode)}, a.logs...)
			} else {
				a.message = err.Error()
				a.logs = append([]string{fmt.Sprintf("%s FAILED (%s): %v", label, mode, err)}, a.logs...)
			}
		} else {
			a.message = label + " completed"
			a.pending = nil
			a.logs = append([]string{fmt.Sprintf("%s OK (%s)", label, mode)}, a.logs...)
		}
		if len(a.logs) > 6 {
			a.logs = a.logs[:6]
		}
		a.mu.Unlock()
		a.render()
	}()
}

func (a *setupApp) confirmPendingDowngrade(ctx context.Context) {
	a.mu.Lock()
	pending := a.pending
	a.mu.Unlock()
	if pending == nil {
		a.setMessage("no pending downgrade confirmation")
		a.render()
		return
	}
	label := "bootstrap+install"
	if pending.repair {
		label = "repair/reinstall"
	}
	a.startOperationForMode(ctx, label+" (downgrade confirmed)", pending.mode, func(ctx context.Context, mode install.Mode) error {
		return a.bootstrapAndInstallWithPolicy(ctx, mode, pending.repair, true)
	})
}

func (a *setupApp) installOnly(_ context.Context, mode install.Mode) error {
	binPath, configPath, err := defaultPaths(mode, runtime.GOOS)
	if err != nil {
		return err
	}
	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("syncd not found at %s (run bootstrap first with key 'b')", binPath)
	}
	if err := ensureConfig(configPath); err != nil {
		return err
	}
	installer, err := newInstaller(mode, runtime.GOOS, binPath, configPath)
	if err != nil {
		return err
	}
	return installer.Install(mode)
}

func (a *setupApp) uninstallOnly(_ context.Context, mode install.Mode) error {
	binPath, configPath, err := defaultPaths(mode, runtime.GOOS)
	if err != nil {
		return err
	}
	installer, err := newInstaller(mode, runtime.GOOS, binPath, configPath)
	if err != nil {
		return err
	}
	return installer.Uninstall(mode)
}

func (a *setupApp) bootstrapAndInstall(ctx context.Context, mode install.Mode) error {
	return a.bootstrapAndInstallWithPolicy(ctx, mode, false, false)
}

func (a *setupApp) repairAndReinstall(ctx context.Context, mode install.Mode) error {
	return a.bootstrapAndInstallWithPolicy(ctx, mode, true, true)
}

func (a *setupApp) bootstrapAndInstallWithPolicy(ctx context.Context, mode install.Mode, force, allowDowngrade bool) error {
	binPath, configPath, err := defaultPaths(mode, runtime.GOOS)
	if err != nil {
		return err
	}
	targetVersion, err := resolveTargetVersion(ctx, a.repo, a.version)
	if err != nil {
		return err
	}

	installedVersion, installedKnown, err := installedSyncdVersion(ctx, binPath)
	if err != nil {
		return err
	}

	if installedKnown && !force {
		cmp, cmpKnown := compareVersions(installedVersion, targetVersion)
		if cmpKnown {
			switch {
			case cmp == 0:
				a.setMessage(fmt.Sprintf("already at %s, reinstalling service registration", targetVersion))
				return a.installOnly(ctx, mode)
			case cmp > 0:
				if !allowDowngrade {
					a.setPendingDowngrade(mode, installedVersion, targetVersion, force)
					return &downgradeConfirmationError{installed: installedVersion, target: targetVersion}
				}
				a.addLog(fmt.Sprintf("warning: proceeding with downgrade %s -> %s", installedVersion, targetVersion))
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		return fmt.Errorf("create bin directory: %w", err)
	}

	baseURL := releaseBaseURL(a.repo, a.version)
	if runtime.GOOS == "windows" {
		if err := downloadFile(ctx, baseURL+"/syncd_windows_amd64.exe", binPath); err != nil {
			return err
		}
		syncctlPath := filepath.Join(filepath.Dir(binPath), "syncctl.exe")
		if err := downloadFile(ctx, baseURL+"/syncctl_windows_amd64.exe", syncctlPath); err != nil {
			return err
		}
	} else {
		if err := downloadFile(ctx, baseURL+"/syncd_linux_amd64", binPath); err != nil {
			return err
		}
		if err := os.Chmod(binPath, 0o755); err != nil {
			return fmt.Errorf("mark syncd executable: %w", err)
		}
		syncctlPath := filepath.Join(filepath.Dir(binPath), "syncctl")
		if err := downloadFile(ctx, baseURL+"/syncctl_linux_amd64", syncctlPath); err != nil {
			return err
		}
		if err := os.Chmod(syncctlPath, 0o755); err != nil {
			return fmt.Errorf("mark syncctl executable: %w", err)
		}
	}

	if err := ensureConfig(configPath); err != nil {
		return err
	}

	installer, err := newInstaller(mode, runtime.GOOS, binPath, configPath)
	if err != nil {
		return err
	}
	if installedKnown {
		a.addLog(fmt.Sprintf("installed version: %s -> target: %s", installedVersion, targetVersion))
	} else {
		a.addLog(fmt.Sprintf("fresh install target version: %s", targetVersion))
	}
	return installer.Install(mode)
}

func (a *setupApp) setMessage(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.message = message
}

func (a *setupApp) addLog(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logs = append([]string{message}, a.logs...)
	if len(a.logs) > 6 {
		a.logs = a.logs[:6]
	}
}

func (a *setupApp) setPendingDowngrade(mode install.Mode, installed, target string, repair bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pending = &downgradePrompt{mode: mode, installed: installed, target: target, repair: repair}
}

func (a *setupApp) render() {
	a.mu.Lock()
	mode := a.mode
	running := a.running
	message := a.message
	logs := append([]string(nil), a.logs...)
	repo := a.repo
	version := a.version
	appVer := a.appVer
	pending := a.pending
	a.mu.Unlock()

	status := "idle"
	if running {
		status = "running"
	}

	b := &strings.Builder{}
	fmt.Fprintf(b, "Git Project Sync - TUI\n")
	fmt.Fprintf(b, "Tabs: [SETUP] status\n\n")
	fmt.Fprintf(b, "Mode:         %s\n", mode)
	fmt.Fprintf(b, "Operation:    %s\n", status)
	fmt.Fprintf(b, "Installer:    %s\n", appVer)
	fmt.Fprintf(b, "Release repo: %s\n", repo)
	fmt.Fprintf(b, "Release tag:  %s\n", version)
	if message != "" {
		fmt.Fprintf(b, "\nAction: %s\n", message)
	}
	if len(logs) > 0 {
		fmt.Fprintf(b, "\nRecent actions:\n")
		for _, logLine := range logs {
			fmt.Fprintf(b, "- %s\n", logLine)
		}
	}
	if pending != nil {
		fmt.Fprintf(b, "\nDowngrade confirmation needed: installed=%s target=%s (press y to continue)\n", pending.installed, pending.target)
	}
	fmt.Fprintf(b, "\nKeys: m(toggle mode) b(bootstrap+install/update) r(repair/reinstall) i(install/register) u(uninstall/unregister) y(confirm downgrade) q(quit)\n")

	fmt.Fprintf(a.out, "\x1b[2J\x1b[H%s", b.String())
}

func (a *setupApp) readKeys(ctx context.Context, out chan<- string) {
	reader := bufio.NewReader(a.in)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		r, _, err := reader.ReadRune()
		if err != nil {
			return
		}
		out <- string(r)
	}
}

type serviceInstaller interface {
	Install(mode install.Mode) error
	Uninstall(mode install.Mode) error
}

func newInstaller(_ install.Mode, goos, binPath, configPath string) (serviceInstaller, error) {
	switch goos {
	case "linux":
		return install.NewLinuxSystemdInstaller(binPath, configPath), nil
	case "windows":
		return install.NewWindowsTaskSchedulerInstaller(binPath, configPath), nil
	default:
		return nil, fmt.Errorf("unsupported OS: %s", goos)
	}
}

func defaultPaths(mode install.Mode, goos string) (string, string, error) {
	switch goos {
	case "linux":
		if mode == install.ModeSystem {
			return "/usr/local/bin/syncd", "/etc/git-project-sync/config.yaml", nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", fmt.Errorf("resolve home dir: %w", err)
		}
		return filepath.Join(home, ".local", "bin", "syncd"), filepath.Join(home, ".config", "git-project-sync", "config.yaml"), nil
	case "windows":
		if mode == install.ModeSystem {
			return filepath.Join(os.Getenv("ProgramFiles"), "git-project-sync", "syncd.exe"), filepath.Join(os.Getenv("ProgramData"), "git-project-sync", "config.yaml"), nil
		}
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "git-project-sync", "bin", "syncd.exe"), filepath.Join(os.Getenv("APPDATA"), "git-project-sync", "config.yaml"), nil
	default:
		return "", "", fmt.Errorf("unsupported OS: %s", goos)
	}
}

func ensureConfig(configPath string) error {
	if configPath == "" {
		return fmt.Errorf("config path is required")
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check config path: %w", err)
	}
	defaultConfig := "daemon:\n  interval: 5m\nrepositories: []\nsources: []\n"
	if err := os.WriteFile(configPath, []byte(defaultConfig), 0o644); err != nil {
		return fmt.Errorf("write default config: %w", err)
	}
	return nil
}

func releaseBaseURL(repo, version string) string {
	if version == "" || version == "latest" {
		return "https://github.com/" + repo + "/releases/latest/download"
	}
	return "https://github.com/" + repo + "/releases/download/" + version
}

func resolveTargetVersion(ctx context.Context, repo, requested string) (string, error) {
	if requested != "" && requested != "latest" {
		return normalizeVersion(requested), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return "", fmt.Errorf("create release metadata request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "git-project-sync-syncsetup")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch latest release metadata: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch latest release metadata: unexpected status %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode release metadata: %w", err)
	}
	if payload.TagName == "" {
		return "", fmt.Errorf("release metadata missing tag_name")
	}
	return normalizeVersion(payload.TagName), nil
}

func installedSyncdVersion(ctx context.Context, binPath string) (string, bool, error) {
	if _, err := os.Stat(binPath); err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("check existing installation: %w", err)
	}
	cmd := exec.CommandContext(ctx, binPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", true, nil
	}
	version := parseVersionFromOutput(string(output))
	if version == "" {
		return "", true, nil
	}
	return version, true, nil
}

func parseVersionFromOutput(output string) string {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) == 0 {
		return ""
	}
	for i := len(fields) - 1; i >= 0; i-- {
		v := normalizeVersion(fields[i])
		if v != "" {
			return v
		}
	}
	return ""
}

func normalizeVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "v")
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return ""
	}
	for i := range parts {
		if _, err := strconv.Atoi(parts[i]); err != nil {
			return ""
		}
	}
	if len(parts) == 2 {
		parts = append(parts, "0")
	}
	return strings.Join(parts, ".")
}

func compareVersions(left, right string) (int, bool) {
	l := normalizeVersion(left)
	r := normalizeVersion(right)
	if l == "" || r == "" {
		return 0, false
	}
	lParts := strings.Split(l, ".")
	rParts := strings.Split(r, ".")
	for i := 0; i < 3; i++ {
		lv, _ := strconv.Atoi(lParts[i])
		rv, _ := strconv.Atoi(rParts[i])
		if lv < rv {
			return -1, true
		}
		if lv > rv {
			return 1, true
		}
	}
	return 0, true
}

type downgradeConfirmationError struct {
	installed string
	target    string
}

func (e *downgradeConfirmationError) Error() string {
	return fmt.Sprintf("downgrade detected: installed %s > target %s, press 'y' to continue", e.installed, e.target)
}

func downloadFile(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: unexpected status %d", url, resp.StatusCode)
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("move downloaded file: %w", err)
	}
	return nil
}
