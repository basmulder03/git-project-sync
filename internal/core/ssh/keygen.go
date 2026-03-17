// Package ssh provides SSH key generation, management, and git SSH configuration
// for per-source authentication without requiring a credential manager.
package ssh

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// KeyType specifies the SSH key algorithm.
type KeyType string

const (
	KeyTypeEd25519 KeyType = "ed25519"
	KeyTypeECDSA   KeyType = "ecdsa"

	// DefaultKeyType is the algorithm used when generating new keys.
	DefaultKeyType KeyType = KeyTypeEd25519
)

// KeyPair holds the paths and public-key content for a managed SSH key.
type KeyPair struct {
	// PrivateKeyPath is the absolute path to the private key file.
	PrivateKeyPath string
	// PublicKeyPath is the absolute path to the public key file.
	PublicKeyPath string
	// PublicKeyContent is the OpenSSH-format public key (ready to upload).
	PublicKeyContent string
}

// GenerateKeyPair creates a new SSH key pair at the given private-key path.
// The public key is written alongside it with a ".pub" suffix.
// keyType must be KeyTypeEd25519 (recommended) or KeyTypeECDSA.
// comment is embedded in the public key (e.g. "git-project-sync/source-id").
func GenerateKeyPair(privateKeyPath string, keyType KeyType, comment string) (*KeyPair, error) {
	if err := os.MkdirAll(filepath.Dir(privateKeyPath), 0o700); err != nil {
		return nil, fmt.Errorf("create ssh key directory: %w", err)
	}

	// Do not overwrite an existing key without explicit intent.
	if _, err := os.Stat(privateKeyPath); err == nil {
		return nil, fmt.Errorf("private key already exists at %s; delete it first to regenerate", privateKeyPath)
	}

	var privatePEM []byte
	var pubKey gossh.PublicKey
	var genErr error

	switch keyType {
	case KeyTypeEd25519, "":
		privatePEM, pubKey, genErr = generateEd25519()
	case KeyTypeECDSA:
		privatePEM, pubKey, genErr = generateECDSA()
	default:
		return nil, fmt.Errorf("unsupported key type %q (supported: ed25519, ecdsa)", keyType)
	}
	if genErr != nil {
		return nil, fmt.Errorf("generate %s key: %w", keyType, genErr)
	}

	// Write private key (mode 0600).
	if err := os.WriteFile(privateKeyPath, privatePEM, 0o600); err != nil {
		return nil, fmt.Errorf("write private key: %w", err)
	}

	// Build public key string.
	pubLine := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pubKey)))
	if comment != "" {
		pubLine = pubLine + " " + comment
	}
	pubContent := pubLine + "\n"

	publicKeyPath := privateKeyPath + ".pub"
	if err := os.WriteFile(publicKeyPath, []byte(pubContent), 0o644); err != nil {
		_ = os.Remove(privateKeyPath) // rollback
		return nil, fmt.Errorf("write public key: %w", err)
	}

	return &KeyPair{
		PrivateKeyPath:   privateKeyPath,
		PublicKeyPath:    publicKeyPath,
		PublicKeyContent: pubContent,
	}, nil
}

// LoadPublicKey reads the public key file for the given private key path.
// It returns the raw OpenSSH-format string.
func LoadPublicKey(privateKeyPath string) (string, error) {
	pubPath := privateKeyPath + ".pub"
	data, err := os.ReadFile(pubPath)
	if err != nil {
		return "", fmt.Errorf("read public key %s: %w", pubPath, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// KeyExists reports whether the private key file is present on disk.
func KeyExists(privateKeyPath string) bool {
	_, err := os.Stat(privateKeyPath)
	return err == nil
}

// PrivateKeyPathForSource returns the canonical path where the private key for
// a given source ID is stored inside the application SSH directory.
func PrivateKeyPathForSource(sshDir, sourceID string) string {
	// Sanitize the source ID so it is safe as a file name.
	safe := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(sourceID)
	return filepath.Join(sshDir, "id_"+safe)
}

// --- internal generators ---

func generateEd25519() ([]byte, gossh.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	privPEM, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal ed25519 private key: %w", err)
	}

	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		return nil, nil, fmt.Errorf("build ssh public key: %w", err)
	}

	return pem.EncodeToMemory(privPEM), sshPub, nil
}

func generateECDSA() ([]byte, gossh.PublicKey, error) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ecdsa key: %w", err)
	}

	privPEM, err := gossh.MarshalPrivateKey(ecKey, "")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal ecdsa private key: %w", err)
	}

	sshPub, err := gossh.NewPublicKey(&ecKey.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("build ssh public key: %w", err)
	}

	return pem.EncodeToMemory(privPEM), sshPub, nil
}
