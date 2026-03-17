package ssh

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Manager provides high-level SSH key operations for a single source account.
type Manager struct {
	// SSHDir is the directory that holds all managed SSH keys
	// (e.g. ~/.config/git-project-sync/ssh or /mnt/c/Users/Alice/AppData/Local/git-project-sync/ssh).
	SSHDir string
	// SSHConfigPath is the user SSH config file to update
	// (e.g. ~/.ssh/config).
	SSHConfigPath string

	// WindowsSSHConfigPath, when non-empty, is the path to the native Windows
	// user SSH config expressed as a WSL path
	// (e.g. /mnt/c/Users/Alice/.ssh/config).
	// When set, every gps-* Host block is mirrored to this file using
	// Windows-native IdentityFile paths so that native Windows git also works.
	WindowsSSHConfigPath string

	// WindowsSSHDir, when non-empty, is the Windows-side SSH key directory
	// expressed as a WSL path (e.g. /mnt/c/Users/Alice/AppData/Local/git-project-sync/ssh).
	// Used when computing the IdentityFile for the Windows SSH config block.
	// If empty and WindowsSSHConfigPath is set, WSLPathToWindows(SSHDir) is used.
	WindowsSSHDir string

	httpClient *http.Client
}

// NewManager creates a new SSH Manager.
// sshDir is the application-managed directory for private keys.
// sshConfigPath is the user SSH config file (usually ~/.ssh/config).
func NewManager(sshDir, sshConfigPath string, httpClient *http.Client) *Manager {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Manager{
		SSHDir:        sshDir,
		SSHConfigPath: sshConfigPath,
		httpClient:    httpClient,
	}
}

// NewManagerWSL creates an SSH Manager configured for WSL ↔ Windows interop.
// It stores keys in windowsSSHDir (a WSL path to a Windows directory) and
// mirrors SSH config Host blocks to both sshConfigPath (WSL ~/.ssh/config) and
// windowsSSHConfigPath (Windows ~/.ssh/config expressed as a WSL path).
func NewManagerWSL(sshDir, sshConfigPath, windowsSSHDir, windowsSSHConfigPath string, httpClient *http.Client) *Manager {
	m := NewManager(sshDir, sshConfigPath, httpClient)
	m.WindowsSSHDir = windowsSSHDir
	m.WindowsSSHConfigPath = windowsSSHConfigPath
	return m
}

// EnsureKey generates an SSH key pair for the given source if one does not
// already exist.  Returns the KeyPair (existing or newly created).
func (m *Manager) EnsureKey(sourceID, provider, comment string) (*KeyPair, error) {
	privPath := PrivateKeyPathForSource(m.SSHDir, sourceID)

	if KeyExists(privPath) {
		pubContent, err := LoadPublicKey(privPath)
		if err != nil {
			return nil, err
		}
		return &KeyPair{
			PrivateKeyPath:   privPath,
			PublicKeyPath:    privPath + ".pub",
			PublicKeyContent: pubContent,
		}, nil
	}

	if comment == "" {
		comment = "git-project-sync/" + sourceID
	}

	return GenerateKeyPair(privPath, DefaultKeyType, comment)
}

// RegenerateKey forcibly regenerates the SSH key pair for a source, removing
// the old key first.  USE WITH CARE — old public key must be removed from the
// provider before calling this.
func (m *Manager) RegenerateKey(sourceID, comment string) (*KeyPair, error) {
	privPath := PrivateKeyPathForSource(m.SSHDir, sourceID)
	_ = removeKeyFiles(privPath)
	if comment == "" {
		comment = "git-project-sync/" + sourceID
	}
	return GenerateKeyPair(privPath, DefaultKeyType, comment)
}

// PrivateKeyPath returns the expected private key path for a source.
func (m *Manager) PrivateKeyPath(sourceID string) string {
	return PrivateKeyPathForSource(m.SSHDir, sourceID)
}

// PublicKeyContent returns the public key string for a source.
// Returns an error if no key exists yet.
func (m *Manager) PublicKeyContent(sourceID string) (string, error) {
	return LoadPublicKey(PrivateKeyPathForSource(m.SSHDir, sourceID))
}

// HasKey reports whether a key exists for the given source.
func (m *Manager) HasKey(sourceID string) bool {
	return KeyExists(PrivateKeyPathForSource(m.SSHDir, sourceID))
}

// EnsureSSHConfigEntry writes or updates the SSH config Host block for a source.
//
// When the Manager is configured for WSL interop (WindowsSSHConfigPath != ""),
// the block is also mirrored to the Windows SSH config file with a Windows-native
// IdentityFile path so that native Windows git clients use the same key.
func (m *Manager) EnsureSSHConfigEntry(sourceID, provider, hostname string) error {
	alias := AliasForSource(sourceID)
	host := hostname
	if host == "" {
		host = DefaultHostname(provider)
	}

	// --- WSL-side (or native Linux/macOS) SSH config entry ---
	wslKeyPath := PrivateKeyPathForSource(m.SSHDir, sourceID)
	entry := ConfigEntry{
		Alias:                 alias,
		HostName:              host,
		User:                  DefaultUser(provider),
		IdentityFile:          wslKeyPath,
		StrictHostKeyChecking: "accept-new",
	}
	if err := EnsureSSHConfigEntry(m.SSHConfigPath, entry); err != nil {
		return err
	}

	// --- Windows-side SSH config entry (WSL interop only) ---
	if m.WindowsSSHConfigPath == "" {
		return nil
	}

	winKeyPath, err := m.windowsIdentityFile(sourceID)
	if err != nil {
		// Non-fatal: log-worthy but don't block the WSL-side write.
		return fmt.Errorf("mirror to windows ssh config: resolve windows key path: %w", err)
	}

	winEntry := ConfigEntry{
		Alias:                 alias,
		HostName:              host,
		User:                  DefaultUser(provider),
		IdentityFile:          winKeyPath,
		StrictHostKeyChecking: "accept-new",
	}
	return EnsureSSHConfigEntry(m.WindowsSSHConfigPath, winEntry)
}

// RemoveSSHConfigEntry removes the SSH config Host block for a source from
// both the primary (WSL or native) config and, if configured, the Windows config.
func (m *Manager) RemoveSSHConfigEntry(sourceID string) error {
	alias := AliasForSource(sourceID)
	if err := RemoveSSHConfigEntry(m.SSHConfigPath, alias); err != nil {
		return err
	}
	if m.WindowsSSHConfigPath != "" {
		// Best-effort removal from Windows config.
		_ = RemoveSSHConfigEntry(m.WindowsSSHConfigPath, alias)
	}
	return nil
}

// GitEnv returns the GIT_SSH_COMMAND environment variable value that forces
// git to use the specific private key for the given source, bypassing the
// ssh-agent and credential manager entirely.
//
// The returned path is always in the format appropriate for the current runtime:
//   - WSL / Linux / macOS: POSIX path  (e.g. /mnt/c/Users/Alice/.../id_source)
//   - Native Windows:      Windows path (e.g. C:\Users\Alice\...\id_source)
//
// When running in WSL with UseWindowsKeyDir enabled, the key is stored at a
// /mnt/<drive>/... path, which is valid for WSL-invoked git.  Native Windows
// git should rely on the SSH config alias (Host block with IdentityFile set to
// the Windows-native path) rather than GIT_SSH_COMMAND.
//
// Example result:
//
//	GIT_SSH_COMMAND=ssh -i '/path/to/key' -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new
func (m *Manager) GitEnv(sourceID string) (string, string) {
	privPath := PrivateKeyPathForSource(m.SSHDir, sourceID)
	value := fmt.Sprintf(
		"ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new",
		shellEscapePath(privPath),
	)
	return "GIT_SSH_COMMAND", value
}

// GitEnvWindows returns the GIT_SSH_COMMAND value suitable for a native Windows
// git process.  It converts the SSHDir to a Windows-native path before building
// the command string.
//
// This is intended for use when the tool is compiled as a native Windows binary
// (GOOS=windows) or when explicitly spawning a Windows git subprocess from WSL.
// In all other cases, use GitEnv.
func (m *Manager) GitEnvWindows(sourceID string) (string, string, error) {
	winPath, err := m.windowsIdentityFile(sourceID)
	if err != nil {
		return "", "", fmt.Errorf("resolve windows key path: %w", err)
	}
	// Windows git uses cmd.exe; double-quote the path.
	escaped := strings.ReplaceAll(winPath, `"`, `\"`)
	value := fmt.Sprintf(
		`ssh -i "%s" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new`,
		escaped,
	)
	return "GIT_SSH_COMMAND", value, nil
}

// GitHubUploadKey performs the GitHub device-flow OAuth to upload the public
// key to the user's GitHub account.  clientID may be empty for the default.
func (m *Manager) GitHubUploadKey(ctx context.Context, sourceID, keyTitle, clientID string, progress DeviceFlowProgress) error {
	pubKey, err := m.PublicKeyContent(sourceID)
	if err != nil {
		return fmt.Errorf("read public key: %w", err)
	}
	return GitHubUploadSSHKey(ctx, pubKey, keyTitle, clientID, progress, m.httpClient)
}

// AzureDevOpsUploadKey uploads the public key to Azure DevOps using the given
// PAT token (which must have "SSH Public Keys: Read & Write" permission).
func (m *Manager) AzureDevOpsUploadKey(ctx context.Context, sourceID, pat, organization, description string) error {
	pubKey, err := m.PublicKeyContent(sourceID)
	if err != nil {
		return fmt.Errorf("read public key: %w", err)
	}
	return AzureDevOpsUploadSSHKey(ctx, pat, organization, pubKey, description, m.httpClient)
}

// SSHAliasForSource returns the SSH config alias for a source.
func (m *Manager) SSHAliasForSource(sourceID string) string {
	return AliasForSource(sourceID)
}

// CloneURLWithAlias converts an HTTPS clone URL to the SSH equivalent,
// substituting the SSH config alias so the correct identity file is selected.
func (m *Manager) CloneURLWithAlias(httpsURL, sourceID, provider string) (string, bool) {
	alias := AliasForSource(sourceID)

	switch strings.ToLower(provider) {
	case "github":
		// https://github.com/owner/repo.git → git@gps-<sourceID>:owner/repo.git
		path := strings.TrimPrefix(httpsURL, "https://github.com/")
		// Strip embedded credential if present
		if idx := strings.Index(path, "@"); idx >= 0 {
			path = strings.TrimPrefix(path, path[:idx+1])
			path = strings.TrimPrefix(path, "github.com/")
		}
		path = strings.TrimSuffix(path, ".git")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 2 {
			return CloneURLForGitHub(parts[0], parts[1], "", alias), true
		}

	case "azuredevops", "azure":
		// https://dev.azure.com/org/project/_git/repo
		httpsClean := httpsURL
		if idx := strings.Index(httpsClean, "@dev.azure.com"); idx >= 0 {
			httpsClean = "https://dev.azure.com" + httpsClean[idx+len("@dev.azure.com"):]
		}
		path := strings.TrimPrefix(httpsClean, "https://dev.azure.com/")
		parts := strings.Split(path, "/")
		if len(parts) >= 4 && parts[2] == "_git" {
			return CloneURLForAzureDevOps(parts[0], parts[1], parts[3], alias), true
		}
	}

	return httpsURL, false
}

// windowsIdentityFile returns the Windows-native path for the private key of
// the given source (e.g. "C:\Users\Alice\AppData\Local\git-project-sync\ssh\id_mysource").
//
// It derives the path by converting the WSL key directory to a Windows path
// using WSLPathToWindows, then appending the key filename.
func (m *Manager) windowsIdentityFile(sourceID string) (string, error) {
	// Determine which SSH dir to convert.
	keyDir := m.WindowsSSHDir
	if keyDir == "" {
		keyDir = m.SSHDir
	}

	// Convert the WSL key dir to a Windows path.
	winDir, ok := WSLPathToWindows(keyDir)
	if !ok {
		return "", fmt.Errorf("cannot convert WSL path %q to Windows path (is wslpath available?)", keyDir)
	}

	// Build the filename the same way PrivateKeyPathForSource does.
	safe := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(sourceID)
	return filepath.Join(winDir, "id_"+safe), nil
}

// removeKeyFiles removes the private and public key files.
func removeKeyFiles(privateKeyPath string) error {
	_ = removeIfExists(privateKeyPath)
	_ = removeIfExists(privateKeyPath + ".pub")
	return nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// shellEscapePath wraps a path in quotes appropriate for the SSH command line.
//
// On Windows (native, not WSL) git spawns ssh through cmd.exe, so paths must
// be double-quoted and backslashes kept as-is.
// On Linux/macOS/WSL, the shell is POSIX-compatible and single-quote quoting
// is used instead (robust against spaces and most special characters).
func shellEscapePath(p string) string {
	if runtime.GOOS == "windows" {
		// Windows: use double-quotes; escape embedded double-quotes with \".
		escaped := strings.ReplaceAll(p, `"`, `\"`)
		return `"` + escaped + `"`
	}
	// Unix/WSL: single-quote wrapping; escape embedded single-quotes.
	if !strings.Contains(p, "'") {
		return "'" + p + "'"
	}
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}
