package desktopui

import (
	"context"
	"testing"

	"glance/internal/agent/provider"
	"glance/internal/agentsettings"
)

func TestRuntimeListsModelsWithUnsavedProviderKeyDraft(t *testing.T) {
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	var factoryCalls, listCalls int
	runtime.agentProviderFactory = func(settings agentsettings.ModelSettings, apiKey string) (provider.ModelProvider, error) {
		factoryCalls++
		if settings.ProviderID != "deepseek" || settings.BaseURL != "https://api.example.test/v1" || apiKey != "draft-key" {
			t.Fatalf("provider factory settings=%+v API key=%q", settings, apiKey)
		}
		return runtimeModelCatalog{list: func() []provider.ModelInfo {
			listCalls++
			return []provider.ModelInfo{{ID: "deepseek-model", Name: "DeepSeek Model"}}
		}}, nil
	}
	models, err := runtime.listAgentModels(context.Background(), agentsettings.ModelSettings{
		ProviderID: "deepseek", BaseURL: "https://api.example.test/v1",
	}, "draft-key")
	if err != nil {
		t.Fatalf("listAgentModels: %v", err)
	}
	if factoryCalls != 1 || listCalls != 1 || len(models) != 1 || models[0] != (provider.ModelInfo{ID: "deepseek-model", Name: "DeepSeek Model"}) {
		t.Fatalf("model lookup calls=%d/%d models=%+v", factoryCalls, listCalls, models)
	}
}

type runtimeModelCatalog struct {
	list func() []provider.ModelInfo
}

func (catalog runtimeModelCatalog) ListModels(context.Context) ([]provider.ModelInfo, error) {
	return catalog.list(), nil
}
func (runtimeModelCatalog) Stream(context.Context, provider.ModelRequest, func(provider.ModelEvent) error) error {
	return nil
}
func (runtimeModelCatalog) Stop(context.Context, string) error { return nil }
