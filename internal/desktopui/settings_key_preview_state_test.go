package desktopui

import (
	"errors"
	"testing"

	"glance/internal/agentsettings"
)

func TestSettingsKeyPreviewRejectsLateResults(t *testing.T) {
	for _, action := range []string{"hide", "leave", "provider", "draft"} {
		t.Run(action, func(t *testing.T) {
			a := New(nil, "")
			a.homeSection = homeSectionSettings
			a.agentUI.settingsPageTab = "providers"
			a.agentUI.settingsTab = "openai"
			saved := agentsettings.ModelSettings{ProviderID: "openai-compatible", BaseURL: "https://api.openai.com/v1", CredentialRef: "test-ref"}
			a.agentUI.settings = saved
			a.agentUI.providerInput.SetText(saved.ProviderID)
			a.agentUI.baseURLInput.SetText(saved.BaseURL)
			a.agentUI.apiKeyInput.Mask = 0
			a.agentUI.settingsKeyPreview = agentSettingsKeyPreview{generation: 1, loading: true, results: make(chan agentSettingsKeyPreviewResult, 1)}
			a.agentUI.settingsKeyPreview.results <- agentSettingsKeyPreviewResult{generation: 1, settings: saved, secret: "late-test-key"}
			switch action {
			case "hide":
				a.toggleAgentSettingsKeyVisibility()
			case "leave":
				a.homeSection = homeSectionRecent
			case "provider":
				a.selectSettingsProvider("deepseek")
			case "draft":
				a.agentUI.apiKeyInput.SetText("new-draft")
			}
			a.processAgentSettingsKeyPreviewResult()
			if a.agentUI.settingsKeyPreview.secret != "" || a.agentUI.settingsKeyPreview.editor.Len() != 0 {
				t.Fatal("late response restored a cleared saved key")
			}
		})
	}
}

func TestSettingsKeyPreviewFailureRestoresMask(t *testing.T) {
	a := New(nil, "")
	a.homeSection = homeSectionSettings
	a.agentUI.settingsPageTab = "providers"
	a.agentUI.settingsTab = "openai"
	saved := agentsettings.ModelSettings{ProviderID: "openai-compatible", BaseURL: "https://api.openai.com/v1", CredentialRef: "test-ref"}
	a.agentUI.settings = saved
	a.agentUI.providerInput.SetText(saved.ProviderID)
	a.agentUI.baseURLInput.SetText(saved.BaseURL)
	a.agentUI.apiKeyInput.Mask = 0
	a.agentUI.settingsKeyPreview = agentSettingsKeyPreview{generation: 1, loading: true, results: make(chan agentSettingsKeyPreviewResult, 1)}
	a.agentUI.settingsKeyPreview.results <- agentSettingsKeyPreviewResult{generation: 1, settings: saved, err: errors.New("credential unavailable")}
	a.processAgentSettingsKeyPreviewResult()
	if a.agentUI.settingsKeyPreview.loading || a.agentUI.apiKeyInput.Mask != '•' || a.agentUI.settingsError == "" {
		t.Fatal("failed read did not leave loading state and restore masked display")
	}
}
