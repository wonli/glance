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
)

func (a *App) showAgentSettings(returnToChat bool) {
	a.prepareAgentSettings(returnToChat)
	a.modal.ShowCustom("模型设置", "配置模型服务后，可以在 Glance 中直接让 Agent 查询和编辑文档。聊天内容及 Agent 通过工具明确读取的文档片段会发送到该模型服务；不会自动上传整份文档。", 620,
		a.layoutAgentSettings,
		a.handleAgentSettingsAction,
		modalDialogActionSpec{ID: "test", Label: "测试连接"},
		modalDialogActionSpec{ID: "save", Label: "保存"},
		modalDialogActionSpec{ID: "clear-key", Label: "删除 API Key"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) prepareAgentSettings(returnToChat bool) {
	a.clearAgentSettingsKeyPreview()
	a.agentUI.apiKeyInput.Mask = '•'
	settings, err := a.ensureDesktopRuntime().loadAgentSettings(context.Background())
	a.agentUI.settings = settings
	a.agentUI.returnToChat = returnToChat
	a.agentUI.settingsError = ""
	a.agentUI.settingsInfo = "API Key 只保存到系统凭据库，不写入普通配置或聊天记录。"
	if settings.CredentialRef != "" {
		a.agentUI.settingsInfo = "API Key 已配置；留空可保留，输入新 Key 会轮换旧 Key。"
	}
	if err != nil {
		a.agentUI.settingsError = err.Error()
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
	a.agentUI.saving = false
}

func (a *App) handleAgentSettingsAction(action string) {
	if a.agentUI.saving {
		return
	}
	switch action {
	case "test":
		settings := agentsettings.ModelSettings{
			ProviderID: a.agentUI.providerInput.Text(),
			BaseURL:    a.agentUI.baseURLInput.Text(),
			Model:      a.agentUI.modelInput.Text(),
		}
		apiKey := a.agentUI.apiKeyInput.Text()
		if settings.ProviderID == "openai-compatible" && strings.TrimSpace(settings.BaseURL) == "" {
			a.agentUI.settingsError = "兼容服务需要填写 API 地址。"
			a.agentUI.settingsInfo = ""
			return
		}
		if a.settingsEndpointChanged(settings, apiKey) {
			a.agentUI.settingsError = "API 地址已更改，请重新输入该服务的 API Key 后再测试。"
			a.agentUI.settingsInfo = ""
			return
		}
		a.agentUI.saving = true
		a.agentUI.settingsError = ""
		a.agentUI.settingsInfo = "正在发送不含文档内容的探测消息…"
		a.runAgentSettingsOperation("test", func(ctx context.Context) (agentsettings.ModelSettings, error) {
			return settings, a.ensureDesktopRuntime().testAgentSettings(ctx, settings, apiKey)
		})
	case "cancel", "close":
		returnToChat := a.agentUI.returnToChat
		a.modal.Hide()
		if returnToChat {
			a.showAgentChat()
		}
	case "save", "confirm":
		settings := agentsettings.ModelSettings{
			ProviderID: a.agentUI.providerInput.Text(),
			BaseURL:    a.agentUI.baseURLInput.Text(),
			Model:      a.agentUI.modelInput.Text(),
		}
		apiKey := a.agentUI.apiKeyInput.Text()
		if settings.ProviderID == "openai-compatible" && strings.TrimSpace(settings.BaseURL) == "" {
			a.agentUI.settingsError = "兼容服务需要填写 API 地址。"
			a.agentUI.settingsInfo = ""
			return
		}
		if a.settingsEndpointChanged(settings, apiKey) {
			a.agentUI.settingsError = "API 地址已更改，请重新输入该服务的 API Key 后再保存。"
			a.agentUI.settingsInfo = ""
			return
		}
		a.agentUI.saving = true
		a.agentUI.settingsError = ""
		a.agentUI.settingsInfo = "正在保存设置并连接能力目录…"
		a.runAgentSettingsOperation("save", func(ctx context.Context) (agentsettings.ModelSettings, error) {
			if err := a.ensureDesktopRuntime().saveAgentSettings(ctx, settings, apiKey); err != nil {
				return settings, err
			}
			saved, err := a.ensureDesktopRuntime().loadAgentSettings(ctx)
			return saved, err
		})
	case "clear-key":
		a.agentUI.saving = true
		a.agentUI.settingsError = ""
		a.agentUI.settingsInfo = "正在从系统凭据库删除 API Key…"
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
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return agentSettingsField(gtx, a.theme, "Provider ID", &a.agentUI.providerInput, "minimax / kimi / deepseek / ollama / openai-compatible")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return agentSettingsField(gtx, a.theme, "服务地址（留空使用内置默认地址）", &a.agentUI.baseURLInput, "https://api.example.com/v1")
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return agentSettingsField(gtx, a.theme, "模型名", &a.agentUI.modelInput, "填写服务商提供的模型名")
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.agentUI.providerInput.Text() == "ollama" {
				style := material.Caption(a.theme, "Ollama 在本机运行，不需要 API Key。默认地址为 http://127.0.0.1:11434。")
				style.Color = homeMuted
				style.MaxLines = 2
				return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, style.Layout)
			}
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return agentSettingsField(gtx, a.theme, "API Key（留空保留已保存的 Key）", &a.agentUI.apiKeyInput, "输入后会安全保存")
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			message := a.agentUI.settingsInfo
			textColor := homeMuted
			if a.agentUI.settingsError != "" {
				message, textColor = a.agentUI.settingsError, color.NRGBA{R: 178, G: 54, B: 54, A: 255}
			}
			style := material.Caption(a.theme, message)
			style.Color = textColor
			style.MaxLines = 3
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, style.Layout)
		}),
	)
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
			if result.operation == "test" {
				_, a.agentUI.settingsError = agent.DescribeTurnError(result.err)
			}
			a.agentUI.settingsInfo = ""
			return
		}
		a.clearAgentSettingsKeyPreview()
		a.agentUI.apiKeyInput.Mask = '•'
		a.agentUI.settings = result.settings
		a.agentUI.settingsError = ""
		a.agentUI.apiKeyInput.SetText("")
		switch result.operation {
		case "test":
			a.agentUI.settingsInfo = "模型连接成功；探测消息未包含文档内容。"
		case "clear":
			a.agentUI.settingsInfo = "API Key 已从系统凭据库删除。"
			if a.agentUI.returnToChat {
				a.modal.Hide()
				a.showAgentChat()
			}
		case "save":
			a.agentUI.settingsInfo = "设置已保存。"
			returnToChat := a.agentUI.returnToChat
			a.agentUI.returnToChat = false
			a.modal.Hide()
			a.setStatus("模型设置已保存")
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
