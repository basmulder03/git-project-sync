package update

import (
	"os"
	"path/filepath"
	"strings"
)

// LastVersionFile is the name of the file that records the last seen syncctl
// version.  It lives inside the application data directory so it persists
// across invocations.
const LastVersionFile = "last_syncctl_version"

// ReadLastVersion reads the previously recorded syncctl version from dataDir.
// Returns ("", nil) if the file does not yet exist.
func ReadLastVersion(dataDir string) (string, error) {
	path := filepath.Join(dataDir, LastVersionFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteLastVersion persists version to dataDir/<LastVersionFile>.
// The directory is created if it does not exist.
func WriteLastVersion(dataDir, version string) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dataDir, LastVersionFile)
	return os.WriteFile(path, []byte(version+"\n"), 0o644)
}

// WasUpdated reports whether currentVersion differs from the last persisted
// version stored in dataDir.  It returns (false, nil) when the file does not
// exist yet (first run) to avoid a spurious sync on a brand-new installation.
func WasUpdated(dataDir, currentVersion string) (bool, error) {
	last, err := ReadLastVersion(dataDir)
	if err != nil {
		return false, err
	}
	// No recorded version means first run – not an update.
	if last == "" {
		return false, nil
	}
	return normalizeVersion(last) != normalizeVersion(currentVersion), nil
}
