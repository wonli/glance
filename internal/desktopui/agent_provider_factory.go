package desktopui

import (
	"context"
	"errors"
	"strings"

	"glance/internal/agent/provider"
	"glance/internal/agent/provider/deepseek"
	"glance/internal/agent/provider/kimi"
	"glance/internal/agent/provider/minimax"
	"glance/internal/agent/provider/ollama"
	"glance/internal/agent/provider/openai"
	"glance/internal/agentsettings"
)

func newAgentProvider(settings agentsettings.ModelSettings, apiKey string) (provider.ModelProvider, error) {
	providerID := strings.ToLower(strings.TrimSpace(settings.ProviderID))
	baseURL := strings.TrimSpace(settings.BaseURL)
	if baseURL == "" {
		baseURL = defaultAgentBaseURL(providerID)
	}
	switch providerID {
	case "ollama":
		return ollama.New(ollama.Config{BaseURL: baseURL, Model: settings.Model}), nil
	case "minimax":
		if strings.TrimSpace(apiKey) == "" {
			return nil, errors.New("请填写 API Key")
		}
		return minimax.New(minimax.Config{BaseURL: baseURL, APIKey: apiKey, Model: settings.Model}), nil
	case "kimi":
		if strings.TrimSpace(apiKey) == "" {
			return nil, errors.New("请填写 API Key")
		}
		return kimi.New(kimi.Config{BaseURL: baseURL, APIKey: apiKey, Model: settings.Model}), nil
	case "deepseek":
		if strings.TrimSpace(apiKey) == "" {
			return nil, errors.New("请填写 API Key")
		}
		return deepseek.New(deepseek.Config{BaseURL: baseURL, APIKey: apiKey, Model: settings.Model}), nil
	case "openai-compatible":
		if baseURL == "" {
			return nil, errors.New("请填写兼容 Chat Completions 的服务地址")
		}
		if strings.TrimSpace(apiKey) == "" {
			return nil, errors.New("请填写 API Key")
		}
		return openai.New(openai.Config{BaseURL: baseURL, APIKey: apiKey, Model: settings.Model}), nil
	default:
		return nil, errors.New("不支持的模型服务商")
	}
}

func (r *desktopRuntime) listAgentModels(ctx context.Context, settings agentsettings.ModelSettings, apiKey string) ([]provider.ModelInfo, error) {
	if r == nil {
		return nil, errors.New("桌面运行时不可用")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	settings.ProviderID = strings.ToLower(strings.TrimSpace(settings.ProviderID))
	settings.BaseURL = strings.TrimSpace(settings.BaseURL)
	apiKey = strings.TrimSpace(apiKey)
	if settings.ProviderID != "ollama" && apiKey == "" {
		var err error
		apiKey, err = r.savedAgentAPIKeyForSettings(ctx, settings)
		if err != nil {
			return nil, err
		}
	}
	factory := r.agentProviderFactory
	if factory == nil {
		factory = newAgentProvider
	}
	modelProvider, err := factory(settings, apiKey)
	if err != nil {
		return nil, err
	}
	return modelProvider.ListModels(ctx)
}

func (r *desktopRuntime) savedAgentAPIKeyForSettings(ctx context.Context, settings agentsettings.ModelSettings) (string, error) {
	r.agentConfigMu.Lock()
	defer r.agentConfigMu.Unlock()
	r.agentMu.RLock()
	store, storeErr := r.agentSettings, r.agentSettingsErr
	r.agentMu.RUnlock()
	if storeErr != nil {
		return "", storeErr
	}
	if store == nil {
		return "", errors.New("Agent 设置存储不可用")
	}
	saved, err := store.Load(ctx)
	if err != nil {
		return "", err
	}
	if saved.ProviderID != settings.ProviderID || normalizeAgentBaseURL(saved.ProviderID, saved.BaseURL) != normalizeAgentBaseURL(settings.ProviderID, settings.BaseURL) || saved.CredentialRef == "" {
		return "", errAgentAPIKeyRequired
	}
	key, err := store.LoadAPIKey(ctx)
	if err != nil {
		return "", errors.New("无法从系统凭据库读取 API Key")
	}
	if strings.TrimSpace(key) == "" {
		return "", errAgentAPIKeyRequired
	}
	return key, nil
}

func normalizeAgentBaseURL(providerID, baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultAgentBaseURL(providerID)
	}
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}
