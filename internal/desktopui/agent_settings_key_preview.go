package desktopui

import (
	"context"
	"errors"
	"strings"
	"time"

	"gioui.org/widget"

	"glance/internal/agentsettings"
	"glance/internal/i18n"
)

type agentSettingsKeyPreviewResult struct {
	generation uint64
	settings   agentsettings.ModelSettings
	secret     string
	err        error
}

type agentSettingsKeyPreview struct {
	editor     widget.Editor
	generation uint64
	secret     string
	loading    bool
	cancel     context.CancelFunc
	results    chan agentSettingsKeyPreviewResult
}

func (a *App) clearAgentSettingsKeyPreview() {
	preview := &a.agentUI.settingsKeyPreview
	preview.generation++
	preview.secret = ""
	preview.editor.SetText("")
	preview.loading = false
	if preview.cancel != nil {
		preview.cancel()
		preview.cancel = nil
	}
}

func (a *App) settingsSavedKeyMatchesDraft() bool {
	saved := a.agentUI.settings
	if saved.CredentialRef == "" || saved.ProviderID != a.agentUI.providerInput.Text() {
		return false
	}
	baseURL := saved.BaseURL
	if baseURL == "" {
		baseURL = defaultAgentBaseURL(saved.ProviderID)
	}
	current := a.agentUI.baseURLInput.Text()
	if current == "" && a.agentUI.settingsTab != "custom" {
		current = defaultAgentBaseURL(saved.ProviderID)
	}
	return strings.TrimRight(baseURL, "/") == strings.TrimRight(current, "/")
}

func (a *App) toggleAgentSettingsKeyVisibility() {
	if a.agentUI.apiKeyInput.Mask == 0 {
		a.captureEditedAgentSettingsKeyPreview()
		a.clearAgentSettingsKeyPreview()
		a.agentUI.apiKeyInput.Mask = '•'
		return
	}
	a.agentUI.apiKeyInput.Mask = 0
	if a.agentUI.apiKeyClearRequested {
		return
	}
	if a.agentUI.apiKeyInput.Len() != 0 || !a.settingsSavedKeyMatchesDraft() {
		return
	}
	a.clearAgentSettingsKeyPreview()
	preview := &a.agentUI.settingsKeyPreview
	if preview.results == nil {
		preview.results = make(chan agentSettingsKeyPreviewResult, 1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	preview.cancel = cancel
	preview.loading = true
	generation, settings, results := preview.generation, a.agentUI.settings, preview.results
	rt := a.ensureDesktopRuntime()
	if !a.launchManagedBackground(func() {
		defer cancel()
		secret, err := rt.loadAgentAPIKeyPreview(ctx, settings)
		select {
		case results <- agentSettingsKeyPreviewResult{generation: generation, settings: settings, secret: secret, err: err}:
		case <-a.shutdown:
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}) {
		a.clearAgentSettingsKeyPreview()
		a.agentUI.apiKeyInput.Mask = '•'
	}
}

func (r *desktopRuntime) loadAgentAPIKeyPreview(ctx context.Context, expected agentsettings.ModelSettings) (string, error) {
	r.agentMu.RLock()
	store, storeErr := r.agentSettings, r.agentSettingsErr
	r.agentMu.RUnlock()
	if storeErr != nil {
		return "", storeErr
	}
	if store == nil {
		return "", errors.New("Agent 设置存储不可用")
	}
	before, err := store.Load(ctx)
	if err != nil {
		return "", err
	}
	if before != expected {
		return "", errors.New("设置已更改，请重新打开设置页")
	}
	secret, err := store.LoadAPIKey(ctx)
	if err != nil {
		return "", errors.New("无法从系统凭据库读取 API Key")
	}
	after, err := store.Load(ctx)
	if err != nil {
		return "", err
	}
	if after != expected {
		return "", errors.New("设置已更改，请重新打开设置页")
	}
	return secret, nil
}

func (a *App) captureEditedAgentSettingsKeyPreview() {
	preview := &a.agentUI.settingsKeyPreview
	if preview.secret == "" || preview.editor.Text() == preview.secret {
		return
	}
	value := preview.editor.Text()
	a.agentUI.apiKeyInput.SetText(value)
	a.agentUI.apiKeyClearRequested = value == ""
}

func (a *App) agentSettingsAPIKeyDraft() (string, bool) {
	preview := &a.agentUI.settingsKeyPreview
	if preview.secret != "" {
		value := preview.editor.Text()
		if value == preview.secret {
			return "", false
		}
		return value, value == ""
	}
	value := a.agentUI.apiKeyInput.Text()
	return value, value == "" && a.agentUI.apiKeyClearRequested
}

func (a *App) processAgentSettingsKeyPreviewResult() {
	preview := &a.agentUI.settingsKeyPreview
	leaving := a.homeSection != homeSectionSettings || a.agentUI.settingsPageTab != "providers" || (a.activeTabID != "" && a.activeTabID != "recent")
	invalidated := leaving || !a.settingsSavedKeyMatchesDraft() || (preview.secret == "" && a.agentUI.apiKeyInput.Len() != 0)
	if (preview.loading || preview.secret != "") && invalidated {
		a.captureEditedAgentSettingsKeyPreview()
		a.clearAgentSettingsKeyPreview()
		if leaving {
			a.agentUI.apiKeyInput.Mask = '•'
		}
	}
	select {
	case result := <-preview.results:
		if result.generation != preview.generation || result.settings != a.agentUI.settings || a.agentUI.apiKeyInput.Mask != 0 || a.agentUI.apiKeyInput.Len() != 0 || !a.settingsSavedKeyMatchesDraft() {
			return
		}
		preview.loading = false
		preview.secret = result.secret
		preview.editor.SingleLine = true
		preview.editor.ReadOnly = false
		preview.editor.SetText(result.secret)
		if result.err != nil {
			a.agentUI.settingsError = result.err.Error()
			a.agentUI.apiKeyInput.Mask = '•'
		}
	default:
	}
}

func (a *App) agentSettingsKeyHint() string {
	if a.agentUI.apiKeyClearRequested || (a.agentUI.settingsKeyPreview.secret != "" && a.agentUI.settingsKeyPreview.editor.Text() == "") {
		return a.text(i18n.Message{ID: "settings.provider.api_key_replace_hint"})
	}
	if !a.settingsSavedKeyMatchesDraft() {
		return a.text(i18n.Message{ID: "settings.provider.api_key_hint"})
	}
	if a.agentUI.apiKeyInput.Mask == 0 {
		if a.agentUI.settingsKeyPreview.loading {
			return a.text(i18n.Message{ID: "settings.provider.loading_key"})
		}
	}
	return "••••••••••••••••••••••••"
}
