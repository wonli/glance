package desktopui

import (
	"image"
	"image/color"
	"os/exec"
	"runtime"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/i18n"
	"glance/ui"
)

func (a *App) prepareAgentSettingsPage() {
	a.prepareAgentSettings(false)
	if a.agentUI.settings.ProviderID == "" {
		a.agentUI.providerInput.SetText("openai-compatible")
		a.agentUI.baseURLInput.SetText("https://api.openai.com/v1")
		if a.agentUI.modelInput.Text() == "" {
			a.agentUI.modelInput.SetText("gpt-4.1")
		}
	}
	a.agentUI.settingsTab = settingsTabFor(a.agentUI.providerInput.Text(), a.agentUI.baseURLInput.Text())
	if a.agentUI.settingsPageTab == "" {
		a.agentUI.settingsPageTab = "basic"
	}
	a.clearAgentSettingsMessages()
	if a.agentUI.settingsPageTabs == nil {
		a.agentUI.settingsPageTabs = make(map[string]*widget.Clickable)
	}
	if a.agentUI.settingsTabButtons == nil {
		a.agentUI.settingsTabButtons = make(map[string]*widget.Clickable)
	}
}

func settingsTabFor(providerID, baseURL string) string {
	switch providerID {
	case "minimax", "kimi", "ollama":
		return providerID
	case "deepseek":
		return "deepseek"
	case "openai":
		return "openai"
	case "openai-compatible":
		if baseURL == "" || baseURL == "https://api.openai.com/v1" {
			return "openai"
		}
		return "custom"
	case "":
		return "openai"
	default:
		return "custom"
	}
}

func defaultAgentModelForTab(tab string) string {
	switch tab {
	case "openai":
		return "gpt-4.1"
	case "deepseek":
		return "deepseek-chat"
	case "minimax":
		return "abab6.5s-chat"
	case "kimi":
		return "moonshot-v1-8k"
	case "ollama":
		return "llama3"
	default:
		return "gpt-4.1"
	}
}

func (a *App) settingsPageTabButton(id string) *widget.Clickable {
	if a.agentUI.settingsPageTabs == nil {
		a.agentUI.settingsPageTabs = make(map[string]*widget.Clickable)
	}
	button := a.agentUI.settingsPageTabs[id]
	if button == nil {
		button = new(widget.Clickable)
		a.agentUI.settingsPageTabs[id] = button
	}
	return button
}

func (a *App) selectSettingsPageTab(id string) {
	if id != "basic" && id != "providers" && id != "mcp" || a.agentUI.settingsPageTab == id {
		return
	}
	if id != "providers" {
		a.cancelAgentModelLookup(true)
		a.clearAgentSettingsKeyPreview()
		a.agentUI.apiKeyInput.Mask = '•'
	}
	a.resetHomeContentScroll()
	a.agentUI.settingsPageTab = id
	a.agentUI.languageMenuOpen = false
	if id == "mcp" {
		a.clearAgentSettingsKeyPreview()
		a.agentUI.apiKeyInput.Mask = '•'
		a.prepareMCPConnectionPage()
	}
}

func (a *App) settingsProviderButton(id string) *widget.Clickable {
	if a.agentUI.settingsTabButtons == nil {
		a.agentUI.settingsTabButtons = make(map[string]*widget.Clickable)
	}
	button := a.agentUI.settingsTabButtons[id]
	if button == nil {
		button = new(widget.Clickable)
		a.agentUI.settingsTabButtons[id] = button
	}
	return button
}

func (a *App) selectSettingsProvider(id string) {
	providerID, baseURL := "", ""
	switch id {
	case "openai":
		providerID, baseURL = "openai-compatible", "https://api.openai.com/v1"
	case "minimax":
		providerID, baseURL = "minimax", defaultAgentBaseURL("minimax")
	case "kimi":
		providerID, baseURL = "kimi", defaultAgentBaseURL("kimi")
	case "ollama":
		providerID, baseURL = "ollama", defaultAgentBaseURL("ollama")
	case "deepseek":
		providerID, baseURL = "deepseek", defaultAgentBaseURL("deepseek")
	case "custom":
		providerID = "openai-compatible"
	case "more":
		a.agentUI.settingsTab = id
		return
	default:
		return
	}
	if a.agentUI.settingsTab != id {
		a.clearAgentSettingsKeyPreview()
		a.agentUI.apiKeyInput.Mask = '•'
		a.agentUI.modelInput.SetText("")
		a.agentUI.apiKeyInput.SetText("")
	}
	a.agentUI.providerInput.SetText(providerID)
	a.agentUI.baseURLInput.SetText(baseURL)
	a.agentUI.settingsTab = id
	a.clearAgentSettingsMessages()
}

func (a *App) layoutAgentSettingsPage(gtx layout.Context) layout.Dimensions {
	a.processAgentModelOptionClicks(gtx)
	if a.agentUI.settingsPageTab == "" {
		a.agentUI.settingsPageTab = "basic"
	}
	if a.agentUI.settingsPageTab == "basic" {
		return a.layoutBasicSettingsPage(gtx)
	}
	if a.agentUI.settingsPageTab == "providers" {
		for _, tab := range []struct {
			id      string
			enabled bool
		}{
			{id: "openai", enabled: true},
			{id: "minimax", enabled: true},
			{id: "kimi", enabled: true},
			{id: "deepseek", enabled: true},
			{id: "ollama", enabled: true},
			{id: "custom", enabled: true},
		} {
			if a.settingsProviderButton(tab.id).Clicked(gtx) {
				if tab.enabled {
					a.selectSettingsProvider(tab.id)
				} else {
					a.setAgentSettingsInfo(i18n.Message{ID: "settings.provider.not_integrated"})
				}
			}
		}
		if a.agentUI.settingsTest.Clicked(gtx) {
			a.handleAgentSettingsAction("test")
		}
		if a.agentUI.settingsSave.Clicked(gtx) {
			a.handleAgentSettingsAction("save")
		}
		if a.agentUI.settingsKeyToggle.Clicked(gtx) {
			a.toggleAgentSettingsKeyVisibility()
		}
		if a.agentUI.settingsKeyLink.Clicked(gtx) {
			a.openAgentAPIKeyHelp()
		}
	}
	settingsBody := func(gtx layout.Context) layout.Dimensions {
		bodyTop := unit.Dp(21)
		if a.agentUI.settingsPageTab == "mcp" {
			bodyTop = 23
		}
		return layout.Inset{Top: bodyTop}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if a.agentUI.settingsPageTab == "mcp" {
				return a.layoutMCPSettingsPage(gtx)
			}
			return a.layoutSettingsProviderPage(gtx)
		})
	}
	body := layout.Rigid(settingsBody)
	if a.agentUI.settingsPageTab == "mcp" {
		body = layout.Flexed(1, settingsBody)
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutSettingsPageHeading(gtx)
		}),
		body,
	)
	a.observeAgentModelLookup()
	return dims
}

func (a *App) layoutSettingsPageHeading(gtx layout.Context) layout.Dimensions {
	title := a.text(i18n.Message{ID: "settings.providers.title"})
	subtitle := a.text(i18n.Message{ID: "settings.providers.subtitle"})
	if a.agentUI.settingsPageTab == "mcp" {
		title = a.text(i18n.Message{ID: "settings.mcp.title"})
		subtitle = a.text(i18n.Message{ID: "settings.mcp.subtitle"})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, title, 20, settingsBlack, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := settingsSecondaryLabel(a.theme, a.tokens().Typography, subtitle)
				style.Color, style.MaxLines = settingsBlack, 2
				return style.Layout(gtx)
			})
		}),
	)
}

func (a *App) layoutSettingsProviderPage(gtx layout.Context) layout.Dimensions {
	width := gtx.Dp(unit.Dp(780))
	if width > gtx.Constraints.Max.X {
		width = gtx.Constraints.Max.X
	}
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutSettingsProviderTabs(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(36)}.Layout(gtx, a.layoutAgentSettingsPageFields)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return checkedSettingsBox(gtx, a.theme)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, a.text(i18n.Message{ID: "settings.provider.set_default", Args: i18n.Args{"Provider": a.localizedSettingsTabLabel(a.agentUI.settingsTab)}}), a.tokens().Typography.SettingsSecondary, settingsBlack, false)
							})
						})
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(unit.Dp(1))
				gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(gtx.Constraints.Max.X, height), image.Pt(gtx.Constraints.Max.X, height)
				return homeRoundedPanel(gtx, homeBorder, homeBorder, unit.Dp(0), func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: gtx.Constraints.Max}
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return agentSettingsPageButton(gtx, a.theme, &a.agentUI.settingsSave, a.text(i18n.Message{ID: "settings.provider.save"}), true, a.agentUI.saving)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return agentSettingsPageButton(gtx, a.theme, &a.agentUI.settingsTest, a.text(i18n.Message{ID: "settings.provider.test"}), false, a.agentUI.saving)
						})
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.agentUI.settingsInfo == "" && a.agentUI.settingsError == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				message, textColor := a.messageText(a.agentUI.settingsInfoRef, a.agentUI.settingsInfo), settingsBlack
				if a.agentUI.settingsError != "" {
					message, textColor = a.messageText(a.agentUI.settingsErrorRef, a.agentUI.settingsError), settingsBlack
				}
				style := settingsSecondaryLabel(a.theme, a.tokens().Typography, message)
				style.Color, style.MaxLines = textColor, 2
				return style.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return homeIcon(gtx, "shield-check", 14, settingsBlack)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							style := settingsSecondaryLabel(a.theme, a.tokens().Typography, a.text(i18n.Message{ID: "settings.provider.share_warning"}))
							style.Color, style.MaxLines = settingsBlack, 2
							return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, style.Layout)
						})
					}),
				)
			})
		}),
	)
}

func checkedSettingsBox(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	size := gtx.Dp(unit.Dp(15))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(size, size), image.Pt(size, size)
	return homeRoundedPanel(gtx, color.NRGBA{R: 54, G: 60, B: 69, A: 255}, color.NRGBA{R: 54, G: 60, B: 69, A: 255}, unit.Dp(3), func(gtx layout.Context) layout.Dimensions {
		style := material.Body2(theme, "✓")
		style.TextSize = unit.Sp(11)
		style.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		return layout.Center.Layout(gtx, style.Layout)
	})
}

func (a *App) layoutSettingsProviderTabs(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(36))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height

	tabs := []struct {
		id      string
		label   string
		message i18n.Message
		enabled bool
		width   unit.Dp
	}{
		{id: "openai", label: "OpenAI", enabled: true, width: 92},
		{id: "minimax", label: "MiniMax", enabled: true, width: 100},
		{id: "kimi", label: "Kimi", enabled: true, width: 78},
		{id: "deepseek", label: "DeepSeek", enabled: true, width: 108},
		{id: "ollama", label: "Ollama", enabled: true, width: 92},
		{id: "custom", label: "", message: i18n.Message{ID: "settings.provider.tab.custom"}, enabled: true, width: 84},
	}
	children := make([]layout.FlexChild, 0, len(tabs))
	for _, tab := range tabs {
		tab := tab
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			button := a.settingsProviderButton(tab.id)
			return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				width, height := gtx.Dp(tab.width), gtx.Dp(unit.Dp(36))
				gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
				return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					selected := a.agentUI.settingsTab == tab.id
					bg, fg := homeHoverFill, settingsBlack
					if selected {
						bg, fg = homeSelectedFill, settingsBlack
					}
					if !tab.enabled {
						fg = settingsBlack
					}
					return homeRoundedPanel(gtx, bg, bg, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return homeTextLabel(gtx, a.theme, a.messageText(tab.message, tab.label), 13, fg, selected)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								dotColor := color.NRGBA{R: 165, G: 172, B: 180, A: 255}
								if tab.enabled {
									dotColor = color.NRGBA{R: 91, G: 155, B: 116, A: 255}
								}
								return layout.Inset{Right: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return homeTextLabel(gtx, a.theme, "•", 11, dotColor, true)
								})
							}),
						)
					})
				})
			})
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func settingsTabLabel(tab string) string {
	switch tab {
	case "openai":
		return "OpenAI"
	case "minimax":
		return "MiniMax"
	case "kimi":
		return "Kimi"
	case "deepseek":
		return "DeepSeek"
	case "ollama":
		return "Ollama"
	case "custom":
		return "自定义"
	default:
		return "当前服务"
	}
}

func (a *App) localizedSettingsTabLabel(tab string) string {
	switch tab {
	case "openai", "minimax", "kimi", "deepseek", "ollama":
		return settingsTabLabel(tab)
	case "custom":
		return a.text(i18n.Message{ID: "settings.provider.tab.custom"})
	default:
		return a.text(i18n.Message{ID: "settings.provider.current_service"})
	}
}

func knownSettingsProviderTab(tab string) bool {
	switch tab {
	case "openai", "minimax", "kimi", "deepseek", "ollama", "custom":
		return true
	default:
		return false
	}
}

func (a *App) layoutAgentSettingsPageFields(gtx layout.Context) layout.Dimensions {
	a.processAgentModelOptionClicks(gtx)
	providerName := a.localizedSettingsTabLabel(a.agentUI.settingsTab)
	savedBaseURL := a.agentUI.settings.BaseURL
	if savedBaseURL == "" {
		savedBaseURL = defaultAgentBaseURL(a.agentUI.settings.ProviderID)
	}
	currentBaseURL := a.agentUI.baseURLInput.Text()
	if currentBaseURL == "" && a.agentUI.settingsTab != "custom" {
		currentBaseURL = defaultAgentBaseURL(a.agentUI.providerInput.Text())
	}
	configured := a.agentUI.settings.ProviderID == a.agentUI.providerInput.Text() &&
		strings.TrimRight(savedBaseURL, "/") == strings.TrimRight(currentBaseURL, "/") &&
		(a.agentUI.settings.CredentialRef != "" || a.agentUI.settings.ProviderID == "ollama")
	statusMessage, statusFG, statusBG := i18n.Message{ID: "settings.provider.status.unconfigured"}, settingsBlack, homeHoverFill
	if configured && strings.TrimSpace(a.agentUI.apiKeyInput.Text()) == "" {
		statusMessage, statusFG, statusBG = i18n.Message{ID: "settings.provider.status.saved"}, settingsBlack, color.NRGBA{R: 232, G: 246, B: 238, A: 255}
	} else if strings.TrimSpace(a.agentUI.providerInput.Text()) != "" {
		statusMessage = i18n.Message{ID: "settings.provider.status.unverified"}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, providerName, 20, settingsBlack, true)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									style := settingsSecondaryLabel(a.theme, a.tokens().Typography, a.text(i18n.Message{ID: "settings.provider.credentials_help"}))
									style.Color = settingsBlack
									return style.Layout(gtx)
								})
							}),
						)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// The panel paints from Max.Y, so bound it inside the list's unbounded item.
						height := gtx.Dp(unit.Dp(26))
						width := gtx.Dp(unit.Dp(76))
						gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
						gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
						return homeRoundedPanel(gtx, statusBG, statusBG, unit.Dp(16), func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, a.text(statusMessage), 12, statusFG, true)
							})
						})
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.agentUI.settingsTab == "custom" {
				return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return agentSettingsPageField(gtx, a.theme, a.tokens().Typography, a.text(i18n.Message{ID: "settings.provider.id"}), &a.agentUI.providerInput, "minimax / kimi / deepseek / ollama / openai-compatible")
				})
			}
			return layout.Dimensions{}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.agentUI.providerInput.Text() == "ollama" {
				style := settingsSecondaryLabel(a.theme, a.tokens().Typography, a.text(i18n.Message{ID: "settings.provider.ollama_note"}))
				style.Color = settingsBlack
				return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, style.Layout)
			}
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutAgentSettingsKeyField(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return agentSettingsPageFieldHelp(gtx, a.theme, a.tokens().Typography, a.text(i18n.Message{ID: "settings.provider.api_url"}), &a.agentUI.baseURLInput, a.text(i18n.Message{ID: "settings.provider.api_url_hint"}), a.text(i18n.Message{ID: "settings.provider.api_url_help"}), false)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := agentSettingsPageFieldHelp(gtx, a.theme, a.tokens().Typography, a.text(i18n.Message{ID: "settings.provider.default_model"}), &a.agentUI.modelInput, a.text(i18n.Message{ID: "settings.provider.model_hint"}), a.text(i18n.Message{ID: "settings.provider.model_help"}), true)
			a.observeAgentModelLookup()
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, a.layoutAgentModelOptions)
		}),
	)
}

func agentSettingsPageField(gtx layout.Context, theme *material.Theme, typography Typography, label string, editor *widget.Editor, hint string) layout.Dimensions {
	return agentSettingsPageFieldHelp(gtx, theme, typography, label, editor, hint, "", false)
}

func agentSettingsPageFieldHelp(gtx layout.Context, theme *material.Theme, typography Typography, label string, editor *widget.Editor, hint, helper string, showDropdown bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return settingsBodyLabel(gtx, theme, typography, label, settingsBlack, false)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				field := ui.TextField{Editor: editor, Hint: hint, Style: ui.TextFieldStyle{
					Height: 44, LeftInset: 12, RightInset: 12, Radius: 8, Text: ui.TextStyle{Size: typography.SettingsBody},
					Fill: homeSurface, Border: homeBorder, Foreground: settingsBlack, Hint: settingsBlack, TextOffsetY: shellOpticalTextOffset(opticalSettingsField),
				}}
				if showDropdown {
					field.Style.RightInset = 0
					field.Style.SuffixWidth = 26
					field.Suffix = func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeIcon(gtx, "chevron-down", 14, settingsBlack)
							})
						})
					}
				}
				return field.Layout(gtx, theme)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if helper == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := settingsSecondaryLabel(theme, typography, helper)
				style.Color, style.MaxLines = settingsBlack, 1
				return style.Layout(gtx)
			})
		}),
	)
}

func (a *App) layoutAgentSettingsKeyField(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return settingsBodyLabel(gtx, a.theme, a.tokens().Typography, "API Key", settingsBlack, false)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.agentUI.settingsKeyLink.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, a.text(i18n.Message{ID: "settings.provider.get_api_key"}), 12, settingsBlack, false)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				editor := &a.agentUI.apiKeyInput
				if editor.Mask == 0 && editor.Len() == 0 && a.settingsSavedKeyMatchesDraft() && a.agentUI.settingsKeyPreview.secret != "" {
					editor = &a.agentUI.settingsKeyPreview.editor
				}
				field := ui.TextField{Editor: editor, Hint: a.agentSettingsKeyHint(), Style: ui.TextFieldStyle{
					Height: 44, LeftInset: 12, SuffixWidth: 40, Radius: 8, Text: ui.TextStyle{Size: a.tokens().Typography.SettingsBody},
					Fill: homeSurface, Border: homeBorder, Foreground: settingsBlack, Hint: settingsBlack, TextOffsetY: shellOpticalTextOffset(opticalSettingsField),
				}}
				field.Suffix = func(gtx layout.Context) layout.Dimensions {
					return a.agentUI.settingsKeyToggle.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						icon := "eye-off"
						if a.agentUI.apiKeyInput.Mask == 0 {
							icon = "eye"
						}
						return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeIcon(gtx, icon, 18, settingsBlack)
							})
						})
					})
				}
				return field.Layout(gtx, a.theme)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			help := i18n.Message{ID: "settings.provider.key_help.default"}
			if a.agentUI.settingsTab == "openai" {
				help = i18n.Message{ID: "settings.provider.key_help.openai"}
			} else if a.agentUI.settingsTab == "deepseek" {
				help = i18n.Message{ID: "settings.provider.key_help.deepseek"}
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := settingsSecondaryLabel(a.theme, a.tokens().Typography, a.text(help))
				style.Color, style.MaxLines = settingsBlack, 1
				return style.Layout(gtx)
			})
		}),
	)
}

func (a *App) openAgentAPIKeyHelp() {
	url := ""
	switch a.agentUI.settingsTab {
	case "openai":
		url = "https://platform.openai.com/api-keys"
	case "deepseek":
		url = "https://platform.deepseek.com/api_keys"
	case "custom", "more":
		a.setStatusMessage(i18n.Message{ID: "settings.provider.api_key_help"}, feedbackWarning, notificationToast)
		return
	}
	if url == "" {
		return
	}
	if !a.launchManagedBackground(func() {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", "--", url)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		if err := cmd.Run(); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "settings.provider.open_key_page_failed"}, err, notificationToast)
		}
	}) {
		a.setStatusMessage(i18n.Message{ID: "settings.provider.open_key_page_failed"}, feedbackError, notificationToast)
	}
}

func agentSettingsPageButton(gtx layout.Context, theme *material.Theme, button *widget.Clickable, label string, primary, disabled bool) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(112)), gtx.Dp(unit.Dp(40))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	fg, bg, border := settingsBlack, homeSurface, homeBorder
	if primary {
		fg, bg, border = settingsBlack, homeSelectedFill, homeBorder
	}
	if disabled {
		fg, bg = settingsBlack, homeHoverFill
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, bg, border, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeTextLabel(gtx, theme, label, 13, fg, true)
			})
		})
	})
}
