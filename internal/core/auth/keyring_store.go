package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

type KeyringStore struct {
	service string
}

func NewKeyringStore(service string) (*KeyringStore, error) {
	if service == "" {
		return nil, errors.New("service name is required")
	}

	return &KeyringStore{service: service}, nil
}

func (s *KeyringStore) SetToken(_ context.Context, sourceID, token string) error {
	if sourceID == "" {
		return errors.New("source id is required")
	}

	if token == "" {
		return errors.New("token is required")
	}

	if err := keyring.Set(s.service, keyName(sourceID), token); err != nil {
		return fmt.Errorf("set keyring token: %w", err)
	}

	return nil
}

func (s *KeyringStore) GetToken(_ context.Context, sourceID string) (string, error) {
	if sourceID == "" {
		return "", errors.New("source id is required")
	}

	token, err := keyring.Get(s.service, keyName(sourceID))
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrTokenNotFound
		}
		return "", fmt.Errorf("get keyring token: %w", err)
	}

	return token, nil
}

func (s *KeyringStore) DeleteToken(_ context.Context, sourceID string) error {
	if sourceID == "" {
		return errors.New("source id is required")
	}

	err := keyring.Delete(s.service, keyName(sourceID))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete keyring token: %w", err)
	}

	return nil
}

func keyName(sourceID string) string {
	return "sources/" + sourceID
}
