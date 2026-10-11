package agentsettings

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const settingsVersion = 1

var (
	ErrInvalidSettings         = errors.New("Agent model settings are invalid")
	ErrCredentialNotConfigured = errors.New("model API key is not configured")
)

type ModelSettings struct {
	ProviderID    string `json:"provider_id"`
	BaseURL       string `json:"base_url,omitempty"`
	Model         string `json:"model"`
	CredentialRef string `json:"credential_ref,omitempty"`
}

type CredentialStore interface {
	Get(context.Context, string) (string, error)
	Put(context.Context, string, string) error
	Delete(context.Context, string) error
}

type settingsFile struct {
	Version  int           `json:"version"`
	Settings ModelSettings `json:"settings"`
}

type Store struct {
	path        string
	credentials CredentialStore
}

func NewStore(path string, credentials CredentialStore) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("model settings path is required")
	}
	return &Store{path: path, credentials: credentials}, nil
}

func NewDefaultStore(credentials CredentialStore) (*Store, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("find user config directory: %w", err)
	}
	return NewStore(filepath.Join(root, "Glance", "agent", "model-settings.json"), credentials)
}

func (s *Store) Load(ctx context.Context) (ModelSettings, error) {
	if s == nil {
		return ModelSettings{}, errors.New("model settings store is unavailable")
	}
	if err := contextError(ctx); err != nil {
		return ModelSettings{}, err
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return ModelSettings{}, nil
	}
	if err != nil {
		return ModelSettings{}, fmt.Errorf("read model settings: %w", err)
	}
	var stored settingsFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return ModelSettings{}, fmt.Errorf("decode model settings: %w", err)
	}
	if stored.Version != settingsVersion {
		return ModelSettings{}, fmt.Errorf("unsupported model settings version %d", stored.Version)
	}
	settings := normalizeSettings(stored.Settings)
	if err := validateSettings(settings); err != nil {
		return ModelSettings{}, err
	}
	return settings, nil
}

// Save writes non-secret settings to a private config file. A new API key is
// written to the system credential store under a fresh reference first, then
// the config file is atomically replaced; the previous key is removed last.
func (s *Store) Save(ctx context.Context, settings ModelSettings, apiKey string) error {
	if s == nil {
		return errors.New("model settings store is unavailable")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	settings = normalizeSettings(settings)
	if err := validateSettings(settings); err != nil {
		return err
	}
	previous, err := s.Load(ctx)
	if err != nil {
		return err
	}
	newCredentialRef := ""
	if settings.ProviderID == "ollama" {
		// Ollama is local and does not use cloud credentials. A provider change
		// must not accidentally carry a previous cloud key into this profile.
		settings.CredentialRef = ""
	} else if strings.TrimSpace(apiKey) != "" {
		if s.credentials == nil {
			return errors.New("system credential store is unavailable")
		}
		newCredentialRef, err = newCredentialReference()
		if err != nil {
			return fmt.Errorf("create credential reference: %w", err)
		}
		if err := s.credentials.Put(ctx, newCredentialRef, apiKey); err != nil {
			_ = s.credentials.Delete(context.Background(), newCredentialRef)
			safeMessage := strings.ReplaceAll(err.Error(), apiKey, "[redacted]")
			return fmt.Errorf("store model API key in system credential store: %s", safeMessage)
		}
		settings.CredentialRef = newCredentialRef
	} else if settings.CredentialRef == "" && previous.ProviderID == settings.ProviderID {
		settings.CredentialRef = previous.CredentialRef
	}
	if err := s.write(settings); err != nil {
		if newCredentialRef != "" {
			_ = s.credentials.Delete(context.Background(), newCredentialRef)
		}
		return err
	}
	if previous.CredentialRef != "" && previous.CredentialRef != settings.CredentialRef {
		if s.credentials == nil {
			return errors.New("model settings were saved, but the previous credential could not be removed because the system credential store is unavailable")
		}
		if err := s.credentials.Delete(ctx, previous.CredentialRef); err != nil {
			return fmt.Errorf("model settings were saved, but the previous credential could not be removed: %w", err)
		}
	}
	return nil
}

func (s *Store) LoadAPIKey(ctx context.Context) (string, error) {
	settings, err := s.Load(ctx)
	if err != nil {
		return "", err
	}
	if settings.CredentialRef == "" {
		return "", ErrCredentialNotConfigured
	}
	if s.credentials == nil {
		return "", errors.New("system credential store is unavailable")
	}
	secret, err := s.credentials.Get(ctx, settings.CredentialRef)
	if err != nil {
		return "", fmt.Errorf("read model API key from system credential store: %w", err)
	}
	return secret, nil
}

func (s *Store) ClearAPIKey(ctx context.Context) error {
	settings, err := s.Load(ctx)
	if err != nil {
		return err
	}
	previous := settings.CredentialRef
	if previous == "" {
		return nil
	}
	settings.CredentialRef = ""
	if err := s.write(settings); err != nil {
		return err
	}
	if previous == "" || s.credentials == nil {
		return nil
	}
	if err := s.credentials.Delete(ctx, previous); err != nil {
		return fmt.Errorf("API key reference was cleared, but its credential could not be removed: %w", err)
	}
	return nil
}

func (s *Store) write(settings ModelSettings) error {
	encoded, err := json.MarshalIndent(settingsFile{Version: settingsVersion, Settings: settings}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode model settings: %w", err)
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create model settings directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("set model settings directory permissions: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".model-settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary model settings: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("set model settings permissions: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		temporary.Close()
		return fmt.Errorf("write model settings: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync model settings: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close model settings: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace model settings: %w", err)
	}
	return nil
}

func normalizeSettings(settings ModelSettings) ModelSettings {
	settings.ProviderID = strings.ToLower(strings.TrimSpace(settings.ProviderID))
	settings.BaseURL = strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/")
	settings.Model = strings.TrimSpace(settings.Model)
	settings.CredentialRef = strings.TrimSpace(settings.CredentialRef)
	return settings
}

func validateSettings(settings ModelSettings) error {
	if settings.ProviderID == "" || settings.Model == "" {
		return fmt.Errorf("%w: provider and model are required", ErrInvalidSettings)
	}
	switch settings.ProviderID {
	case "minimax", "kimi", "deepseek", "ollama", "openai-compatible":
	default:
		return fmt.Errorf("%w: unsupported provider %q", ErrInvalidSettings, settings.ProviderID)
	}
	if settings.BaseURL != "" {
		parsed, err := url.Parse(settings.BaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return fmt.Errorf("%w: base URL must be an HTTP(S) URL without user information", ErrInvalidSettings)
		}
	}
	return nil
}

func newCredentialReference() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
