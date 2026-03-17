package ssh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	coressh "github.com/basmulder03/git-project-sync/internal/core/ssh"
)

func TestGenerateKeyPair_Ed25519(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "id_ed25519")

	kp, err := coressh.GenerateKeyPair(privPath, coressh.KeyTypeEd25519, "test-comment")
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	// Private key file must exist and be 0600.
	info, err := os.Stat(kp.PrivateKeyPath)
	if err != nil {
		t.Fatalf("private key stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("private key mode = %o, want 600", info.Mode().Perm())
	}

	// Public key file must exist.
	if _, err := os.Stat(kp.PublicKeyPath); err != nil {
		t.Fatalf("public key stat: %v", err)
	}

	// Public key content should be non-empty and contain the key type.
	if !strings.HasPrefix(kp.PublicKeyContent, "ssh-ed25519 ") {
		t.Errorf("public key content unexpected: %s", kp.PublicKeyContent)
	}

	// Comment should be embedded.
	if !strings.Contains(kp.PublicKeyContent, "test-comment") {
		t.Errorf("comment not found in public key: %s", kp.PublicKeyContent)
	}
}

func TestGenerateKeyPair_ECDSA(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "id_ecdsa")

	kp, err := coressh.GenerateKeyPair(privPath, coressh.KeyTypeECDSA, "")
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	if !strings.HasPrefix(kp.PublicKeyContent, "ecdsa-sha2-nistp256 ") {
		t.Errorf("unexpected pub key type: %s", kp.PublicKeyContent)
	}
}

func TestGenerateKeyPair_NoOverwrite(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "id_ed25519")

	if _, err := coressh.GenerateKeyPair(privPath, coressh.KeyTypeEd25519, ""); err != nil {
		t.Fatalf("first generate: %v", err)
	}

	_, err := coressh.GenerateKeyPair(privPath, coressh.KeyTypeEd25519, "")
	if err == nil {
		t.Error("expected error when key already exists, got nil")
	}
}

func TestGenerateKeyPair_UnsupportedType(t *testing.T) {
	dir := t.TempDir()
	_, err := coressh.GenerateKeyPair(filepath.Join(dir, "id_rsa"), "rsa", "")
	if err == nil {
		t.Error("expected error for unsupported key type")
	}
}

func TestKeyExists(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "id_ed25519")

	if coressh.KeyExists(privPath) {
		t.Error("expected KeyExists=false before key is created")
	}

	if _, err := coressh.GenerateKeyPair(privPath, coressh.KeyTypeEd25519, ""); err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	if !coressh.KeyExists(privPath) {
		t.Error("expected KeyExists=true after key is created")
	}
}

func TestLoadPublicKey(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "id_ed25519")

	kp, err := coressh.GenerateKeyPair(privPath, coressh.KeyTypeEd25519, "my-comment")
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	got, err := coressh.LoadPublicKey(privPath)
	if err != nil {
		t.Fatalf("LoadPublicKey: %v", err)
	}

	// LoadPublicKey trims trailing whitespace; content in KeyPair also trimmed.
	if got != strings.TrimSpace(kp.PublicKeyContent) {
		t.Errorf("LoadPublicKey mismatch:\ngot  %q\nwant %q", got, kp.PublicKeyContent)
	}
}

func TestPrivateKeyPathForSource(t *testing.T) {
	tests := []struct {
		sshDir   string
		sourceID string
		want     string
	}{
		{"/ssh", "github-acme", "/ssh/id_github-acme"},
		{"/ssh", "azure/corp", "/ssh/id_azure_corp"},
		{"/ssh", "my source", "/ssh/id_my_source"},
	}

	for _, tc := range tests {
		got := coressh.PrivateKeyPathForSource(tc.sshDir, tc.sourceID)
		// Normalize path separator for cross-platform comparison
		got = filepath.ToSlash(got)
		want := filepath.ToSlash(tc.want)
		if got != want {
			t.Errorf("PrivateKeyPathForSource(%q, %q) = %q, want %q", tc.sshDir, tc.sourceID, got, want)
		}
	}
}
