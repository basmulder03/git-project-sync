package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type FallbackStore struct {
	service      string
	storagePath  string
	keyEnvVar    string
	resolvedKey  []byte
	resolvedInit bool
}

type fallbackPayload struct {
	Service string            `json:"service"`
	Tokens  map[string]string `json:"tokens"`
}

type encryptedPayload struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func NewFallbackStore(service, storagePath, keyEnvVar string) (*FallbackStore, error) {
	if service == "" {
		return nil, errors.New("service name is required")
	}
	if storagePath == "" {
		return nil, errors.New("storage path is required")
	}
	if keyEnvVar == "" {
		keyEnvVar = "GIT_PROJECT_SYNC_FALLBACK_KEY"
	}

	return &FallbackStore{
		service:     service,
		storagePath: storagePath,
		keyEnvVar:   keyEnvVar,
	}, nil
}

func (s *FallbackStore) SetToken(_ context.Context, sourceID, token string) error {
	if sourceID == "" {
		return errors.New("source id is required")
	}
	if token == "" {
		return errors.New("token is required")
	}

	payload, err := s.readPayload()
	if err != nil {
		return err
	}

	payload.Tokens[sourceID] = token
	return s.writePayload(payload)
}

func (s *FallbackStore) GetToken(_ context.Context, sourceID string) (string, error) {
	if sourceID == "" {
		return "", errors.New("source id is required")
	}

	payload, err := s.readPayload()
	if err != nil {
		return "", err
	}

	token, ok := payload.Tokens[sourceID]
	if !ok || token == "" {
		return "", ErrTokenNotFound
	}

	return token, nil
}

func (s *FallbackStore) DeleteToken(_ context.Context, sourceID string) error {
	if sourceID == "" {
		return errors.New("source id is required")
	}

	payload, err := s.readPayload()
	if err != nil {
		return err
	}

	delete(payload.Tokens, sourceID)
	return s.writePayload(payload)
}

func (s *FallbackStore) readPayload() (fallbackPayload, error) {
	if _, err := os.Stat(s.storagePath); errors.Is(err, os.ErrNotExist) {
		return fallbackPayload{Service: s.service, Tokens: map[string]string{}}, nil
	}

	raw, err := os.ReadFile(s.storagePath)
	if err != nil {
		return fallbackPayload{}, fmt.Errorf("read fallback token file: %w", err)
	}

	var encrypted encryptedPayload
	if err := json.Unmarshal(raw, &encrypted); err != nil {
		return fallbackPayload{}, fmt.Errorf("parse fallback token file: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(encrypted.Nonce)
	if err != nil {
		return fallbackPayload{}, fmt.Errorf("decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil {
		return fallbackPayload{}, fmt.Errorf("decode ciphertext: %w", err)
	}

	plaintext, err := s.decrypt(nonce, ciphertext)
	if err != nil {
		return fallbackPayload{}, err
	}

	var payload fallbackPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return fallbackPayload{}, fmt.Errorf("parse fallback payload: %w", err)
	}

	if payload.Tokens == nil {
		payload.Tokens = map[string]string{}
	}

	return payload, nil
}

func (s *FallbackStore) writePayload(payload fallbackPayload) error {
	if payload.Tokens == nil {
		payload.Tokens = map[string]string{}
	}

	payload.Service = s.service

	plaintext, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal fallback payload: %w", err)
	}

	nonce, ciphertext, err := s.encrypt(plaintext)
	if err != nil {
		return err
	}

	encoded, err := json.Marshal(encryptedPayload{
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	})
	if err != nil {
		return fmt.Errorf("marshal encrypted payload: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.storagePath), 0o700); err != nil {
		return fmt.Errorf("create fallback store directory: %w", err)
	}

	if err := os.WriteFile(s.storagePath, encoded, 0o600); err != nil {
		return fmt.Errorf("write fallback token file: %w", err)
	}

	return nil
}

func (s *FallbackStore) encrypt(plaintext []byte) ([]byte, []byte, error) {
	gcm, err := s.cipherBlock()
	if err != nil {
		return nil, nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return nonce, ciphertext, nil
}

func (s *FallbackStore) decrypt(nonce, ciphertext []byte) ([]byte, error) {
	gcm, err := s.cipherBlock()
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt fallback token file: %w", err)
	}

	return plaintext, nil
}

func (s *FallbackStore) cipherBlock() (cipher.AEAD, error) {
	key, err := s.encryptionKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AES-GCM cipher: %w", err)
	}

	return gcm, nil
}

func (s *FallbackStore) encryptionKey() ([]byte, error) {
	if s.resolvedInit {
		return s.resolvedKey, nil
	}

	raw := os.Getenv(s.keyEnvVar)
	if raw == "" {
		return nil, fmt.Errorf("fallback key env var %s is not set", s.keyEnvVar)
	}

	hash := sha256.Sum256([]byte(raw))
	s.resolvedKey = hash[:]
	s.resolvedInit = true
	return s.resolvedKey, nil
}
