package ssh

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigEntry represents a Host block in an SSH config file.
type ConfigEntry struct {
	// Alias is the Host pattern alias used in the ssh config (e.g. "github-myaccount").
	Alias string
	// HostName is the real hostname (e.g. "github.com").
	HostName string
	// IdentityFile is the absolute path to the private key file.
	IdentityFile string
	// User is the SSH user (e.g. "git" for GitHub/Azure DevOps).
	User string
	// StrictHostKeyChecking controls host-key verification ("yes", "no", "accept-new").
	StrictHostKeyChecking string
}

// DefaultUser returns the SSH user for a provider.
func DefaultUser(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		return "git"
	case "azuredevops", "azure":
		return "git"
	default:
		return "git"
	}
}

// DefaultHostname returns the default SSH hostname for a provider.
func DefaultHostname(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		return "github.com"
	case "azuredevops", "azure":
		return "ssh.dev.azure.com"
	default:
		return ""
	}
}

// AliasForSource returns a deterministic SSH config alias for a source ID.
// Example: "github-myaccount" → "gps-github-myaccount"
func AliasForSource(sourceID string) string {
	safe := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-", "_", "-").Replace(sourceID)
	return "gps-" + strings.ToLower(safe)
}

// EnsureSSHConfigEntry writes or updates a Host block for the given source in
// the SSH config file at configPath. Existing entries are replaced in place;
// new entries are appended.
//
// The caller is responsible for creating the parent directory.
func EnsureSSHConfigEntry(configPath string, entry ConfigEntry) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return fmt.Errorf("create ssh config directory: %w", err)
	}

	// Read existing file (it's okay if it doesn't exist yet).
	var existing string
	if data, err := os.ReadFile(configPath); err == nil {
		existing = string(data)
	}

	updated, changed := upsertHostBlock(existing, entry)
	if !changed {
		return nil
	}

	// Write with mode 0600 (SSH requires config to not be world-readable).
	if err := os.WriteFile(configPath, []byte(updated), 0o600); err != nil {
		return fmt.Errorf("write ssh config: %w", err)
	}

	return nil
}

// RemoveSSHConfigEntry removes the Host block for the given alias from the
// SSH config file.  It is a no-op if the alias does not exist.
func RemoveSSHConfigEntry(configPath, alias string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read ssh config: %w", err)
	}

	updated := removeHostBlock(string(data), alias)
	return os.WriteFile(configPath, []byte(updated), 0o600)
}

// ListSSHConfigEntries returns all Host aliases written by git-project-sync
// (i.e. aliases that start with "gps-") found in the given config file.
func ListSSHConfigEntries(configPath string) ([]string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read ssh config: %w", err)
	}

	var aliases []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(strings.ToLower(line), "host ") {
			alias := strings.TrimSpace(line[5:])
			if strings.HasPrefix(alias, "gps-") {
				aliases = append(aliases, alias)
			}
		}
	}
	return aliases, nil
}

// BuildSSHBlock renders a Host block as text.
func BuildSSHBlock(entry ConfigEntry) string {
	hostKeyChecking := entry.StrictHostKeyChecking
	if hostKeyChecking == "" {
		hostKeyChecking = "yes"
	}
	user := entry.User
	if user == "" {
		user = "git"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Host %s\n", entry.Alias)
	fmt.Fprintf(&sb, "    HostName %s\n", entry.HostName)
	fmt.Fprintf(&sb, "    User %s\n", user)
	fmt.Fprintf(&sb, "    IdentityFile %s\n", entry.IdentityFile)
	fmt.Fprintf(&sb, "    IdentitiesOnly yes\n")
	fmt.Fprintf(&sb, "    StrictHostKeyChecking %s\n", hostKeyChecking)
	return sb.String()
}

// upsertHostBlock inserts or replaces a Host block in rawConfig.
// Returns the updated content and a bool indicating whether a change was made.
func upsertHostBlock(rawConfig string, entry ConfigEntry) (string, bool) {
	block := BuildSSHBlock(entry)
	existing, start, end := findHostBlock(rawConfig, entry.Alias)
	if existing == strings.TrimRight(block, "\n") {
		return rawConfig, false
	}

	if start == -1 {
		// Append.
		sep := ""
		if len(rawConfig) > 0 && !strings.HasSuffix(rawConfig, "\n\n") {
			if strings.HasSuffix(rawConfig, "\n") {
				sep = "\n"
			} else {
				sep = "\n\n"
			}
		}
		return rawConfig + sep + block + "\n", true
	}

	// Replace.
	updated := rawConfig[:start] + block + rawConfig[end:]
	return updated, true
}

// removeHostBlock removes the Host block with the given alias from rawConfig.
func removeHostBlock(rawConfig, alias string) string {
	_, start, end := findHostBlock(rawConfig, alias)
	if start == -1 {
		return rawConfig
	}
	return rawConfig[:start] + rawConfig[end:]
}

// findHostBlock locates the start and end byte offsets of a Host block in
// rawConfig. Returns (-1, -1) if not found.  Also returns the block text.
func findHostBlock(rawConfig, alias string) (blockText string, start, end int) {
	lines := strings.Split(rawConfig, "\n")
	inBlock := false
	blockStart := -1
	charOffset := 0

	targetHost := strings.ToLower("host " + alias)

	for i, line := range lines {
		lineLen := len(line) + 1 // +1 for '\n'

		trimmed := strings.TrimSpace(line)

		if !inBlock {
			if strings.ToLower(trimmed) == targetHost ||
				strings.ToLower(strings.TrimSpace(line)) == targetHost {
				inBlock = true
				blockStart = charOffset
			}
		} else {
			// A new non-empty Host line ends the block.
			if strings.HasPrefix(strings.ToLower(trimmed), "host ") && !strings.EqualFold(strings.TrimSpace(line), "host "+alias) {
				blockText = rawConfig[blockStart:charOffset]
				return strings.TrimRight(blockText, "\n"), blockStart, charOffset
			}
			// Blank lines between blocks are included in the previous block
			// but we stop at the next Host keyword.
			if i == len(lines)-1 {
				// Last line — block extends to end.
				blockText = rawConfig[blockStart:]
				return strings.TrimRight(blockText, "\n"), blockStart, len(rawConfig)
			}
		}

		charOffset += lineLen
	}

	if inBlock && blockStart != -1 {
		blockText = rawConfig[blockStart:]
		return strings.TrimRight(blockText, "\n"), blockStart, len(rawConfig)
	}

	return "", -1, -1
}
