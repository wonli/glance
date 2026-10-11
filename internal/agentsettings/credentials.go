package agentsettings

import (
	"context"
	"errors"
	"strings"

	keyring "github.com/zalando/go-keyring"
)

const credentialService = "glance.agent.provider"

type SystemCredentialStore struct{}

func (SystemCredentialStore) Get(ctx context.Context, reference string) (string, error) {
	if err := contextError(ctx); err != nil {
		return "", err
	}
	if strings.TrimSpace(reference) == "" {
		return "", ErrCredentialNotConfigured
	}
	secret, err := keyring.Get(credentialService, reference)
	if err != nil {
		return "", err
	}
	if err := contextError(ctx); err != nil {
		return "", err
	}
	return secret, nil
}

func (SystemCredentialStore) Put(ctx context.Context, reference, secret string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(reference) == "" || secret == "" {
		return errors.New("credential reference and secret are required")
	}
	if err := keyring.Set(credentialService, reference, secret); err != nil {
		return err
	}
	return nil
}

func (SystemCredentialStore) Delete(ctx context.Context, reference string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(reference) == "" {
		return nil
	}
	err := keyring.Delete(credentialService, reference)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
