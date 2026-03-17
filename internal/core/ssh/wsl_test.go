package ssh

import (
	"runtime"
	"testing"
)

// TestManualWindowsToWSL verifies the path conversion fallback (no wslpath).
func TestManualWindowsToWSL(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{`C:\Users\Alice\AppData\Local\foo`, "/mnt/c/Users/Alice/AppData/Local/foo", true},
		{`D:\work\project`, "/mnt/d/work/project", true},
		{`c:\lowercase`, "/mnt/c/lowercase", true},
		{`\\server\share`, "", false}, // UNC path — not supported
		{`relative\path`, "", false},
		{``, "", false},
	}

	for _, tc := range cases {
		got, ok := manualWindowsToWSL(tc.in)
		if ok != tc.wantOK {
			t.Errorf("manualWindowsToWSL(%q): ok=%v, want ok=%v", tc.in, ok, tc.wantOK)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("manualWindowsToWSL(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestManualWSLToWindows verifies the reverse path conversion fallback.
func TestManualWSLToWindows(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"/mnt/c/Users/Alice/AppData/Local/foo", `C:\Users\Alice\AppData\Local\foo`, true},
		{"/mnt/d/work/project", `D:\work\project`, true},
		{"/mnt/c", `C:\`, true},
		{"/home/alice/.ssh", "", false}, // not under /mnt/
		{"/mnt/", "", false},            // no drive letter
		{"relative/path", "", false},
	}

	for _, tc := range cases {
		got, ok := manualWSLToWindows(tc.in)
		if ok != tc.wantOK {
			t.Errorf("manualWSLToWindows(%q): ok=%v, want ok=%v", tc.in, ok, tc.wantOK)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("manualWSLToWindows(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestShellEscapePathUnix verifies POSIX single-quote escaping (used on Linux/WSL).
// This test only applies to non-Windows platforms because Windows uses a
// different quoting strategy (double-quotes) and the test expectations are
// specifically for the POSIX branch.
func TestShellEscapePathUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX single-quote escaping is not used on Windows")
	}
	cases := []struct {
		in   string
		want string
	}{
		{"/home/alice/.ssh/id_source", "'/home/alice/.ssh/id_source'"},
		{"/path/with spaces/key", "'/path/with spaces/key'"},
		{"/path/with'quote/key", `'/path/with'\''quote/key'`},
	}
	for _, tc := range cases {
		// Call the package-private helper via the exported path through manager.
		// Since shellEscapePath is unexported, test it through a full GitEnv call.
		// We just validate the escaping logic for the unix branch directly here.
		got := shellEscapePath(tc.in)
		if got != tc.want {
			t.Errorf("shellEscapePath(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestNewManagerWSL verifies that NewManagerWSL sets the Windows interop fields.
func TestNewManagerWSL(t *testing.T) {
	m := NewManagerWSL(
		"/mnt/c/Users/Alice/AppData/Local/git-project-sync/ssh",
		"/home/alice/.ssh/config",
		"/mnt/c/Users/Alice/AppData/Local/git-project-sync/ssh",
		"/mnt/c/Users/Alice/.ssh/config",
		nil,
	)

	if m.SSHDir != "/mnt/c/Users/Alice/AppData/Local/git-project-sync/ssh" {
		t.Errorf("SSHDir mismatch: %q", m.SSHDir)
	}
	if m.SSHConfigPath != "/home/alice/.ssh/config" {
		t.Errorf("SSHConfigPath mismatch: %q", m.SSHConfigPath)
	}
	if m.WindowsSSHConfigPath != "/mnt/c/Users/Alice/.ssh/config" {
		t.Errorf("WindowsSSHConfigPath mismatch: %q", m.WindowsSSHConfigPath)
	}
	if m.WindowsSSHDir != "/mnt/c/Users/Alice/AppData/Local/git-project-sync/ssh" {
		t.Errorf("WindowsSSHDir mismatch: %q", m.WindowsSSHDir)
	}
}

// TestManagerEnsureSSHConfigEntry_DualSync verifies that when a Windows SSH config
// path is configured, the Host block is mirrored with a Windows-native IdentityFile.
func TestManagerEnsureSSHConfigEntry_DualSync(t *testing.T) {
	t.Skip("Requires wslpath — integration test only")
}

// TestManagerGitEnv verifies the GIT_SSH_COMMAND format for WSL-side git.
func TestManagerGitEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("GIT_SSH_COMMAND path format differs on native Windows; use TestManagerGitEnvWindows")
	}
	m := NewManager("/mnt/c/gps/ssh", "/home/alice/.ssh/config", nil)

	key, val := m.GitEnv("github-alice")
	if key != "GIT_SSH_COMMAND" {
		t.Errorf("expected GIT_SSH_COMMAND, got %q", key)
	}

	// Path should be the WSL path, single-quoted (on Linux).
	wantPrefix := "ssh -i '/mnt/c/gps/ssh/id_github-alice'"
	if len(val) < len(wantPrefix) || val[:len(wantPrefix)] != wantPrefix {
		t.Errorf("GitEnv value: got %q, want prefix %q", val, wantPrefix)
	}

	// Must contain IdentitiesOnly=yes.
	if !containsStr(val, "IdentitiesOnly=yes") {
		t.Errorf("GitEnv value missing IdentitiesOnly=yes: %q", val)
	}
}

// TestIsWSL is a soft smoke test — we cannot guarantee the test runner is/isn't
// in WSL, but we can verify the function returns a bool without panicking.
func TestIsWSL(t *testing.T) {
	result := IsWSL()
	t.Logf("IsWSL() = %v", result)
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
