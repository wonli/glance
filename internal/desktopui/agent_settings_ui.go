package desktopui

import (
	"context"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/agent"
	"glance/internal/agentsettings"
	"glance/internal/i18n"
)

func (a *App) showAgentSettings(returnToChat bool) {
	a.prepareAgentSettings(returnToChat)
	a.modal.ShowCustomMessage(dialogIDAgentModelSettings, i18n.Message{ID: "settings.provider.dialog.title"}, i18n.Message{ID: "settings.provider.dialog.body"}, 620,
		a.layoutAgentSettings,
		a.handleAgentSettingsAction,
		modalDialogActionSpec{ID: "test", LabelRef: i18n.Message{ID: "settings.provider.test"}},
		modalDialogActionSpec{ID: "save", LabelRef: i18n.Message{ID: "settings.provider.save"}},
		modalDialogActionSpec{ID: "clear-key", LabelRef: i18n.Message{ID: "settings.provider.delete_key"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "settings.provider.cancel"}},
	)
}

func (a *App) setAgentSettingsInfo(message i18n.Message) {
	a.agentUI.settingsInfoRef = message
	a.agentUI.settingsInfo = a.text(message)
}

func (a *App) setAgentSettingsError(message i18n.Message) {
	a.agentUI.settingsErrorRef = message
	a.agentUI.settingsError = a.text(message)
}

func (a *App) clearAgentSettingsMessages() {
	a.agentUI.settingsInfo, a.agentUI.settingsError = "", ""
	a.agentUI.settingsInfoRef, a.agentUI.settingsErrorRef = i18n.Message{}, i18n.Message{}
}

func (a *App) prepareAgentSettings(returnToChat bool) {
	a.cancelAgentModelLookup(true)
	a.clearAgentSettingsKeyPreview()
	a.agentUI.apiKeyInput.Mask = '•'
	settings, err := a.ensureDesktopRuntime().loadAgentSettings(context.Background())
	a.agentUI.settings = settings
	a.agentUI.returnToChat = returnToChat
	a.clearAgentSettingsMessages()
	a.setAgentSettingsInfo(i18n.Message{ID: "settings.provider.credential_not_saved"})
	if settings.CredentialRef != "" {
		a.setAgentSettingsInfo(i18n.Message{ID: "settings.provider.credential_saved"})
	}
	if err != nil {
		a.agentUI.settingsError = err.Error()
		a.agentUI.settingsErrorRef = i18n.Message{}
	}
	if settings.ProviderID == "" {
		settings.ProviderID = "minimax"
	}
	if settings.BaseURL == "" {
		settings.BaseURL = defaultAgentBaseURL(settings.ProviderID)
	}
	a.agentUI.providerInput.SetText(settings.ProviderID)
	a.agentUI.baseURLInput.SetText(settings.BaseURL)
	a.agentUI.modelInput.SetText(settings.Model)
	a.agentUI.apiKeyInput.SetText("")
	a.agentUI.apiKeyClearRequested = false
	a.agentUI.saving = false
}

func (a *App) handleAgentSettingsAction(action string) {
	if a.agentUI.saving {
		return
	}
	switch action {
	case "test":
		a.cancelAgentModelLookup(true)
		settings := agentsettings.ModelSettings{
			ProviderID: strings.TrimSpace(a.agentUI.providerInput.Text()),
			BaseURL:    strings.TrimSpace(a.agentUI.baseURLInput.Text()),
			Model:      strings.TrimSpace(a.agentUI.modelInput.Text()),
		}
		if settings.Model == "" {
			tab := settingsTabFor(settings.ProviderID, settings.BaseURL)
			settings.Model = defaultAgentModelForTab(tab)
		}
		apiKey, _ := a.agentSettingsAPIKeyDraft()
		if settings.ProviderID == "openai-compatible" && strings.TrimSpace(settings.BaseURL) == "" {
			a.setAgentSettingsError(i18n.Message{ID: "settings.provider.api_url_required"})
			a.agentUI.settingsInfo, a.agentUI.settingsInfoRef = "", i18n.Message{}
			return
		}
		if a.settingsEndpointChanged(settings, apiKey) {
			a.setAgentSettingsError(i18n.Message{ID: "settings.provider.api_key_changed_test"})
			a.agentUI.settingsInfo, a.agentUI.settingsInfoRef = "", i18n.Message{}
			return
		}
		a.agentUI.saving = true
		a.agentUI.settingsError, a.agentUI.settingsErrorRef = "", i18n.Message{}
		a.setAgentSettingsInfo(i18n.Message{ID: "settings.provider.probe_sending"})
		a.runAgentSettingsOperation("test", func(ctx context.Context) (agentsettings.ModelSettings, error) {
			return settings, a.ensureDesktopRuntime().testAgentSettings(ctx, settings, apiKey)
		})
	case "cancel", "close":
		a.cancelAgentModelLookup(true)
		returnToChat := a.agentUI.returnToChat
		a.modal.Hide()
		if returnToChat {
			a.showAgentChat()
		}
	case "save", "confirm":
		a.cancelAgentModelLookup(true)
		settings := agentsettings.ModelSettings{
			ProviderID: strings.TrimSpace(a.agentUI.providerInput.Text()),
			BaseURL:    strings.TrimSpace(a.agentUI.baseURLInput.Text()),
			Model:      strings.TrimSpace(a.agentUI.modelInput.Text()),
		}
		if settings.Model == "" {
			tab := settingsTabFor(settings.ProviderID, settings.BaseURL)
			settings.Model = defaultAgentModelForTab(tab)
		}
		apiKey, clearKey := a.agentSettingsAPIKeyDraft()
		if settings.ProviderID == "openai-compatible" && strings.TrimSpace(settings.BaseURL) == "" {
			a.setAgentSettingsError(i18n.Message{ID: "settings.provider.api_url_required"})
			a.agentUI.settingsInfo, a.agentUI.settingsInfoRef = "", i18n.Message{}
			return
		}
		if a.settingsEndpointChanged(settings, apiKey) && !clearKey {
			a.setAgentSettingsError(i18n.Message{ID: "settings.provider.api_key_changed_save"})
			a.agentUI.settingsInfo, a.agentUI.settingsInfoRef = "", i18n.Message{}
			return
		}
		a.agentUI.saving = true
		a.agentUI.settingsError, a.agentUI.settingsErrorRef = "", i18n.Message{}
		a.setAgentSettingsInfo(i18n.Message{ID: "settings.provider.saving_connecting"})
		a.runAgentSettingsOperation("save", func(ctx context.Context) (agentsettings.ModelSettings, error) {
			runtime := a.ensureDesktopRuntime()
			if err := runtime.saveAgentSettings(ctx, settings, apiKey); err != nil {
				return settings, err
			}
			if clearKey && strings.TrimSpace(apiKey) == "" {
				if err := runtime.clearAgentAPIKey(ctx); err != nil {
					return settings, err
				}
			}
			saved, err := runtime.loadAgentSettings(ctx)
			return saved, err
		})
	case "clear-key":
		a.cancelAgentModelLookup(true)
		a.agentUI.saving = true
		a.agentUI.settingsError, a.agentUI.settingsErrorRef = "", i18n.Message{}
		a.setAgentSettingsInfo(i18n.Message{ID: "settings.provider.deleting_key"})
		a.runAgentSettingsOperation("clear", func(ctx context.Context) (agentsettings.ModelSettings, error) {
			if err := a.ensureDesktopRuntime().clearAgentAPIKey(ctx); err != nil {
				return agentsettings.ModelSettings{}, err
			}
			return a.ensureDesktopRuntime().loadAgentSettings(ctx)
		})
	}
}

func (a *App) settingsEndpointChanged(settings agentsettings.ModelSettings, apiKey string) bool {
	previousBaseURL := strings.TrimRight(strings.TrimSpace(a.agentUI.settings.BaseURL), "/")
	if previousBaseURL == "" {
		previousBaseURL = defaultAgentBaseURL(a.agentUI.settings.ProviderID)
	}
	return strings.TrimSpace(apiKey) == "" &&
		a.agentUI.settings.ProviderID != "" &&
		a.agentUI.settings.ProviderID == settings.ProviderID &&
		previousBaseURL != strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/")
}

func (a *App) runAgentSettingsOperation(operation string, run func(context.Context) (agentsettings.ModelSettings, error)) {
	ctx, cancel := context.WithCancel(context.Background())
	watcherStarted := a.launchManagedBackground(func() {
		select {
		case <-a.shutdown:
			cancel()
		case <-ctx.Done():
		}
	})
	if !watcherStarted {
		cancel()
		return
	}
	if !a.launchManagedBackground(func() {
		defer cancel()
		settings, err := run(ctx)
		select {
		case a.agentUI.settingsDone <- agentSettingsResult{operation: operation, settings: settings, err: err}:
		case <-a.shutdown:
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}) {
		cancel()
	}
}

func (a *App) layoutAgentSettings(gtx layout.Context) layout.Dimensions {
	a.processAgentModelOptionClicks(gtx)
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return agentSettingsField(gtx, a.theme, "Provider ID", &a.agentUI.providerInput, "minimax / kimi / deepseek / ollama / openai-compatible")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return agentSettingsField(gtx, a.theme, a.text(i18n.Message{ID: "settings.provider.api_url"}), &a.agentUI.baseURLInput, a.text(i18n.Message{ID: "settings.provider.api_url_hint"}))
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return agentSettingsField(gtx, a.theme, a.text(i18n.Message{ID: "settings.provider.default_model"}), &a.agentUI.modelInput, a.text(i18n.Message{ID: "settings.provider.model_hint"}))
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var dims layout.Dimensions
			if a.agentUI.providerInput.Text() == "ollama" {
				style := material.Caption(a.theme, a.text(i18n.Message{ID: "settings.provider.ollama_default_url"}))
				style.Color = homeMuted
				style.MaxLines = 2
				dims = layout.Inset{Top: unit.Dp(12)}.Layout(gtx, style.Layout)
			} else {
				dims = layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return agentSettingsField(gtx, a.theme, a.text(i18n.Message{ID: "settings.provider.api_key_label"}), &a.agentUI.apiKeyInput, a.text(i18n.Message{ID: "settings.provider.api_key_hint"}))
				})
			}
			a.observeAgentModelLookup()
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutAgentModelOptions(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			message := a.messageText(a.agentUI.settingsInfoRef, a.agentUI.settingsInfo)
			textColor := homeMuted
			if a.agentUI.settingsError != "" {
				message, textColor = a.messageText(a.agentUI.settingsErrorRef, a.agentUI.settingsError), color.NRGBA{R: 178, G: 54, B: 54, A: 255}
			}
			style := material.Caption(a.theme, message)
			style.Color = textColor
			style.MaxLines = 3
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, style.Layout)
		}),
	)
	a.observeAgentModelLookup()
	return dims
}

func agentSettingsField(gtx layout.Context, theme *material.Theme, label string, editor *widget.Editor, hint string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			style := material.Caption(theme, label)
			style.Color = homeMuted
			style.MaxLines = 1
			return style.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Editor(theme, editor, hint)
				return style.Layout(gtx)
			})
		}),
	)
}

func defaultAgentBaseURL(providerID string) string {
	switch providerID {
	case "openai-compatible":
		return "https://api.openai.com/v1"
	case "minimax":
		return "https://api.minimaxi.com/v1"
	case "kimi":
		return "https://api.moonshot.cn/v1"
	case "deepseek":
		return "https://api.deepseek.com"
	case "ollama":
		return "http://127.0.0.1:11434"
	default:
		return ""
	}
}

func (a *App) processAgentSettingsResult() {
	select {
	case result := <-a.agentUI.settingsDone:
		a.agentUI.saving = false
		if result.err != nil {
			a.agentUI.settingsError = result.err.Error()
			a.agentUI.settingsErrorRef = i18n.Message{}
			if result.operation == "test" {
				_, a.agentUI.settingsError = agent.DescribeTurnError(result.err)
				a.agentUI.settingsErrorRef = i18n.Message{}
			}
			a.agentUI.settingsInfo, a.agentUI.settingsInfoRef = "", i18n.Message{}
			return
		}
		a.clearAgentSettingsKeyPreview()
		a.agentUI.apiKeyInput.Mask = '•'
		a.agentUI.settings = result.settings
		a.agentUI.settingsError, a.agentUI.settingsErrorRef = "", i18n.Message{}
		a.agentUI.apiKeyInput.SetText("")
		a.agentUI.apiKeyClearRequested = false
		switch result.operation {
		case "test":
			a.setAgentSettingsInfo(i18n.Message{ID: "settings.provider.probe_success"})
		case "clear":
			a.setAgentSettingsInfo(i18n.Message{ID: "settings.provider.key_deleted"})
			if a.agentUI.returnToChat {
				a.modal.Hide()
				a.showAgentChat()
			}
		case "save":
			a.setAgentSettingsInfo(i18n.Message{ID: "settings.provider.saved"})
			returnToChat := a.agentUI.returnToChat
			a.agentUI.returnToChat = false
			a.modal.Hide()
			a.setStatusMessage(i18n.Message{ID: "settings.provider.saved_status"}, feedbackSuccess, notificationToast)
			if returnToChat {
				a.showAgentChat()
			}
		}
	default:
		return
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
