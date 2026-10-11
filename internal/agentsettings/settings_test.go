package agentsettings

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsStoreKeepsAPIKeyOutOfConfigAndRotatesCredential(t *testing.T) {
	ctx := context.Background()
	credentials := newMemoryCredentialStore()
	path := filepath.Join(t.TempDir(), "agent", "model-settings.json")
	store, err := NewStore(path, credentials)
	if err != nil {
		t.Fatal(err)
	}

	settings := ModelSettings{ProviderID: " MiniMax ", BaseURL: " ", Model: " MiniMax-M3 "}
	if err := store.Save(ctx, settings, "secret-first-key"); err != nil {
		t.Fatalf("Save first model settings: %v", err)
	}
	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load model settings: %v", err)
	}
	if loaded.ProviderID != "minimax" || loaded.Model != "MiniMax-M3" || loaded.CredentialRef == "" {
		t.Fatalf("normalized settings = %+v", loaded)
	}
	secret, err := store.LoadAPIKey(ctx)
	if err != nil || secret != "secret-first-key" {
		t.Fatalf("LoadAPIKey = %q, %v", secret, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-first-key") || strings.Contains(string(data), "api_key") {
		t.Fatalf("model config contains secret material: %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("model settings mode=%#o; want 0600", info.Mode().Perm())
	}

	oldReference := loaded.CredentialRef
	beforeRotation, err := store.Load(ctx)
	if err != nil || beforeRotation.CredentialRef != oldReference {
		t.Fatalf("credential before rotation = %+v, %v; want ref %q", beforeRotation, err, oldReference)
	}
	settings.Model = "MiniMax-M4"
	if err := store.Save(ctx, settings, "secret-second-key"); err != nil {
		t.Fatalf("rotate model API key: %v", err)
	}
	rotated, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.CredentialRef == oldReference {
		t.Fatal("API key rotation reused the old credential reference")
	}
	if _, exists := credentials.items[oldReference]; exists {
		t.Fatalf("old credential %q was not removed after settings were replaced; rotated ref=%q", oldReference, rotated.CredentialRef)
	}
	if secret, err := store.LoadAPIKey(ctx); err != nil || secret != "secret-second-key" {
		t.Fatalf("LoadAPIKey after rotation = %q, %v", secret, err)
	}

	if err := store.ClearAPIKey(ctx); err != nil {
		t.Fatalf("ClearAPIKey: %v", err)
	}
	cleared, err := store.Load(ctx)
	if err != nil || cleared.CredentialRef != "" {
		t.Fatalf("settings after clearing API key = %+v, %v", cleared, err)
	}
	if _, err := store.LoadAPIKey(ctx); !errors.Is(err, ErrCredentialNotConfigured) {
		t.Fatalf("LoadAPIKey after clear = %v; want not configured", err)
	}
}

func TestSettingsStoreRejectsInvalidSettingsAndDoesNotFallbackToPlaintext(t *testing.T) {
	ctx := context.Background()
	credentials := newMemoryCredentialStore()
	path := filepath.Join(t.TempDir(), "model-settings.json")
	store, err := NewStore(path, credentials)
	if err != nil {
		t.Fatal(err)
	}
	for _, settings := range []ModelSettings{
		{ProviderID: "minimax"},
		{ProviderID: "unknown", Model: "model"},
		{ProviderID: "openai-compatible", Model: "model", BaseURL: "file:///tmp/key"},
		{ProviderID: "openai-compatible", Model: "model", BaseURL: "https://user:password@example.test/v1"},
	} {
		if err := store.Save(ctx, settings, "secret-key"); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("Save(%+v) error=%v; want ErrInvalidSettings", settings, err)
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid settings wrote config file: %v", err)
	}
	if len(credentials.items) != 0 {
		t.Fatalf("invalid settings stored credentials: %#v", credentials.items)
	}

	if err := store.Save(ctx, ModelSettings{ProviderID: "ollama", Model: "qwen3"}, "local-secret"); err != nil {
		t.Fatalf("save Ollama settings without a keyring dependency in file: %v", err)
	}
}

func TestSettingsStoreCleansNewCredentialWhenConfigWriteFails(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	file := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	credentials := newMemoryCredentialStore()
	store, err := NewStore(filepath.Join(file, "model-settings.json"), credentials)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Save(ctx, ModelSettings{ProviderID: "deepseek", Model: "deepseek-chat"}, "secret-key")
	if err == nil {
		t.Fatal("Save unexpectedly succeeded below a regular file")
	}
	if len(credentials.items) != 0 {
		t.Fatalf("credential was not removed after config write failure: %#v", credentials.items)
	}
}

func TestSettingsStoreDropsCloudCredentialWhenSwitchingToOllama(t *testing.T) {
	ctx := context.Background()
	credentials := newMemoryCredentialStore()
	store, err := NewStore(filepath.Join(t.TempDir(), "model-settings.json"), credentials)
	if err != nil {
		t.Fatal(err)
	}
	cloud := ModelSettings{ProviderID: "deepseek", Model: "deepseek-chat"}
	if err := store.Save(ctx, cloud, "cloud-secret"); err != nil {
		t.Fatalf("save cloud settings: %v", err)
	}
	previous, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, ModelSettings{ProviderID: "ollama", Model: "qwen3"}, "ignored-secret"); err != nil {
		t.Fatalf("switch to Ollama: %v", err)
	}
	local, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if local.CredentialRef != "" {
		t.Fatalf("Ollama settings retained cloud credential reference %q", local.CredentialRef)
	}
	if _, exists := credentials.items[previous.CredentialRef]; exists {
		t.Fatal("switching to Ollama retained the previous cloud credential")
	}
	if len(credentials.items) != 0 {
		t.Fatalf("Ollama stored an API key: %#v", credentials.items)
	}
}

func TestSettingsStoreRedactsAPIKeyFromCredentialErrors(t *testing.T) {
	credentials := newMemoryCredentialStore()
	credentials.putErr = errors.New("keychain rejected secret-sensitive-value")
	store, err := NewStore(filepath.Join(t.TempDir(), "model-settings.json"), credentials)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Save(context.Background(), ModelSettings{ProviderID: "deepseek", Model: "deepseek-chat"}, "secret-sensitive-value")
	if err == nil {
		t.Fatal("Save unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), "secret-sensitive-value") {
		t.Fatalf("credential error leaked the API key: %v", err)
	}
	if _, statErr := os.Stat(store.path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed credential write left a settings file: %v", statErr)
	}
}

type memoryCredentialStore struct {
	items  map[string]string
	putErr error
	getErr error
	delErr error
}

func newMemoryCredentialStore() *memoryCredentialStore {
	return &memoryCredentialStore{items: make(map[string]string)}
}

func (s *memoryCredentialStore) Get(ctx context.Context, reference string) (string, error) {
	if err := contextError(ctx); err != nil {
		return "", err
	}
	if s.getErr != nil {
		return "", s.getErr
	}
	secret, ok := s.items[reference]
	if !ok {
		return "", errors.New("credential not found")
	}
	return secret, nil
}

func (s *memoryCredentialStore) Put(ctx context.Context, reference, secret string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if s.putErr != nil {
		return s.putErr
	}
	s.items[reference] = secret
	return nil
}

func (s *memoryCredentialStore) Delete(ctx context.Context, reference string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if s.delErr != nil {
		return s.delErr
	}
	delete(s.items, reference)
	return nil
}
