package ssh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	coressh "github.com/basmulder03/git-project-sync/internal/core/ssh"
)

func TestAliasForSource(t *testing.T) {
	tests := []struct {
		sourceID string
		want     string
	}{
		{"github-acme", "gps-github-acme"},
		{"azure/corp", "gps-azure-corp"},
		{"My Source", "gps-my-source"},
		{"source_one", "gps-source-one"},
	}

	for _, tc := range tests {
		got := coressh.AliasForSource(tc.sourceID)
		if got != tc.want {
			t.Errorf("AliasForSource(%q) = %q, want %q", tc.sourceID, got, tc.want)
		}
	}
}

func TestDefaultUser(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"github", "git"},
		{"GitHub", "git"},
		{"azuredevops", "git"},
		{"azure", "git"},
		{"unknown", "git"},
	}

	for _, tc := range tests {
		got := coressh.DefaultUser(tc.provider)
		if got != tc.want {
			t.Errorf("DefaultUser(%q) = %q, want %q", tc.provider, got, tc.want)
		}
	}
}

func TestDefaultHostname(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"github", "github.com"},
		{"azuredevops", "ssh.dev.azure.com"},
		{"azure", "ssh.dev.azure.com"},
		{"unknown", ""},
	}

	for _, tc := range tests {
		got := coressh.DefaultHostname(tc.provider)
		if got != tc.want {
			t.Errorf("DefaultHostname(%q) = %q, want %q", tc.provider, got, tc.want)
		}
	}
}

func TestEnsureSSHConfigEntry_NewFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".ssh", "config")

	entry := coressh.ConfigEntry{
		Alias:                 "gps-github-acme",
		HostName:              "github.com",
		User:                  "git",
		IdentityFile:          "/home/user/.local/share/git-project-sync/ssh/id_github-acme",
		StrictHostKeyChecking: "accept-new",
	}

	if err := coressh.EnsureSSHConfigEntry(configPath, entry); err != nil {
		t.Fatalf("EnsureSSHConfigEntry: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Host gps-github-acme") {
		t.Errorf("expected Host block, got:\n%s", content)
	}
	if !strings.Contains(content, "HostName github.com") {
		t.Errorf("expected HostName directive, got:\n%s", content)
	}
	if !strings.Contains(content, "IdentitiesOnly yes") {
		t.Errorf("expected IdentitiesOnly directive, got:\n%s", content)
	}
}

func TestEnsureSSHConfigEntry_Idempotent(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	entry := coressh.ConfigEntry{
		Alias:                 "gps-test",
		HostName:              "github.com",
		User:                  "git",
		IdentityFile:          "/tmp/id_test",
		StrictHostKeyChecking: "accept-new",
	}

	// Write twice.
	if err := coressh.EnsureSSHConfigEntry(configPath, entry); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := coressh.EnsureSSHConfigEntry(configPath, entry); err != nil {
		t.Fatalf("second write: %v", err)
	}

	data, _ := os.ReadFile(configPath)
	// Should have exactly one Host block.
	count := strings.Count(string(data), "Host gps-test")
	if count != 1 {
		t.Errorf("expected 1 Host block, got %d:\n%s", count, string(data))
	}
}

func TestRemoveSSHConfigEntry(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	entry := coressh.ConfigEntry{
		Alias:        "gps-todelete",
		HostName:     "github.com",
		User:         "git",
		IdentityFile: "/tmp/id_todelete",
	}

	if err := coressh.EnsureSSHConfigEntry(configPath, entry); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := coressh.RemoveSSHConfigEntry(configPath, "gps-todelete"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	data, _ := os.ReadFile(configPath)
	if strings.Contains(string(data), "gps-todelete") {
		t.Errorf("alias still present after removal:\n%s", string(data))
	}
}

func TestListSSHConfigEntries(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	for _, alias := range []string{"gps-a", "gps-b"} {
		entry := coressh.ConfigEntry{
			Alias:        alias,
			HostName:     "github.com",
			User:         "git",
			IdentityFile: "/tmp/id_" + alias,
		}
		if err := coressh.EnsureSSHConfigEntry(configPath, entry); err != nil {
			t.Fatalf("write %s: %v", alias, err)
		}
	}

	// Add a non-gps entry manually.
	nonGPS := "\nHost personal\n    HostName github.com\n    User git\n"
	f, _ := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(nonGPS)
	_ = f.Close()

	aliases, err := coressh.ListSSHConfigEntries(configPath)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(aliases) != 2 {
		t.Errorf("expected 2 gps aliases, got %d: %v", len(aliases), aliases)
	}
	for _, a := range aliases {
		if !strings.HasPrefix(a, "gps-") {
			t.Errorf("unexpected alias: %s", a)
		}
	}
}
