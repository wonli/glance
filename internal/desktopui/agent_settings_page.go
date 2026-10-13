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
		a.agentUI.settingsPageTab = "providers"
	}
	a.agentUI.settingsInfo = ""
	a.agentUI.settingsError = ""
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
	case "openai-compatible":
		if baseURL == "" || baseURL == "https://api.openai.com/v1" {
			return "openai"
		}
		return "custom"
	default:
		return "custom"
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
	if id != "providers" && id != "mcp" || a.agentUI.settingsPageTab == id {
		return
	}
	if id != "providers" {
		a.cancelAgentModelLookup(true)
	}
	a.resetHomeContentScroll()
	a.agentUI.settingsPageTab = id
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
	a.agentUI.settingsInfo = ""
	a.agentUI.settingsError = ""
}

func (a *App) layoutAgentSettingsPage(gtx layout.Context) layout.Dimensions {
	a.processAgentModelOptionClicks(gtx)
	if a.agentUI.settingsPageTab == "" {
		a.agentUI.settingsPageTab = "providers"
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
					a.agentUI.settingsInfo = "该服务商暂未接入。"
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
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutSettingsPageHeading(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
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
		}),
	)
	a.observeAgentModelLookup()
	return dims
}

func (a *App) layoutSettingsPageHeading(gtx layout.Context) layout.Dimensions {
	title, subtitle := "AI 提供商", "配置模型密钥和接口，选择用于文档 AI 的默认提供商。"
	if a.agentUI.settingsPageTab == "mcp" {
		title = "MCP 连接"
		subtitle = "将以下连接信息填入其他 Agent 的 MCP 客户端，即可操作当前桌面会话。"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, title, 20, settingsBlack, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Caption(a.theme, subtitle)
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
								return homeTextLabel(gtx, a.theme, "将 "+settingsTabLabel(a.agentUI.settingsTab)+" 设为默认 AI 提供商", 13, settingsBlack, false)
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
						return agentSettingsPageButton(gtx, a.theme, &a.agentUI.settingsSave, "保存配置", true, a.agentUI.saving)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return agentSettingsPageButton(gtx, a.theme, &a.agentUI.settingsTest, "测试连接", false, a.agentUI.saving)
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
				message, textColor := a.agentUI.settingsInfo, settingsBlack
				if a.agentUI.settingsError != "" {
					message, textColor = a.agentUI.settingsError, settingsBlack
				}
				style := material.Caption(a.theme, message)
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
							style := material.Caption(a.theme, "请勿分享 API Key；调用费用由对应模型提供商收取。")
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
		enabled bool
		width   unit.Dp
	}{
		{id: "openai", label: "OpenAI", enabled: true, width: 92},
		{id: "minimax", label: "MiniMax", enabled: true, width: 100},
		{id: "kimi", label: "Kimi", enabled: true, width: 78},
		{id: "deepseek", label: "DeepSeek", enabled: true, width: 108},
		{id: "ollama", label: "Ollama", enabled: true, width: 92},
		{id: "custom", label: "自定义", enabled: true, width: 84},
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
									return homeTextLabel(gtx, a.theme, tab.label, 13, fg, selected)
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

func (a *App) layoutAgentSettingsPageFields(gtx layout.Context) layout.Dimensions {
	a.processAgentModelOptionClicks(gtx)
	providerName := settingsTabLabel(a.agentUI.settingsTab)
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
	status, statusFG, statusBG := "未配置", settingsBlack, homeHoverFill
	if configured && strings.TrimSpace(a.agentUI.apiKeyInput.Text()) == "" {
		status, statusFG, statusBG = "已保存", settingsBlack, color.NRGBA{R: 232, G: 246, B: 238, A: 255}
	} else if strings.TrimSpace(a.agentUI.providerInput.Text()) != "" {
		status = "待验证"
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
									style := material.Caption(a.theme, "独立保存每个提供商的密钥与模型偏好。")
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
								return homeTextLabel(gtx, a.theme, status, 12, statusFG, true)
							})
						})
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.agentUI.settingsTab == "custom" {
				return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return agentSettingsPageField(gtx, a.theme, "服务商 ID", &a.agentUI.providerInput, "minimax / kimi / deepseek / ollama / openai-compatible")
				})
			}
			return layout.Dimensions{}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.agentUI.providerInput.Text() == "ollama" {
				style := material.Caption(a.theme, "Ollama 在本机运行，不需要 API Key。")
				style.Color = settingsBlack
				return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, style.Layout)
			}
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutAgentSettingsKeyField(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return agentSettingsPageFieldHelp(gtx, a.theme, "API 地址", &a.agentUI.baseURLInput, "https://api.example.com/v1", "支持官方接口或兼容 OpenAI 的代理地址。")
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := agentSettingsPageFieldHelp(gtx, a.theme, "默认模型", &a.agentUI.modelInput, "填写服务商提供的模型名", "用于新建 AI 对话，可在会话中切换。")
			a.observeAgentModelLookup()
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, a.layoutAgentModelOptions)
		}),
	)
}

func agentSettingsPageField(gtx layout.Context, theme *material.Theme, label string, editor *widget.Editor, hint string) layout.Dimensions {
	return agentSettingsPageFieldHelp(gtx, theme, label, editor, hint, "")
}

func agentSettingsPageFieldHelp(gtx layout.Context, theme *material.Theme, label string, editor *widget.Editor, hint, helper string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			style := material.Caption(theme, label)
			style.Color = settingsBlack
			style.MaxLines = 1
			return style.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(unit.Dp(44))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
				return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
					settingsFieldFocus(gtx, editor)
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = 0
								style := material.Editor(theme, editor, hint)
								style.TextSize = defaultShellTokens.Typography.Body
								style.Color = settingsBlack
								style.HintColor = settingsBlack
								return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, style.Layout)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if label != "默认模型" {
								return layout.Dimensions{}
							}
							return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeIcon(gtx, "chevron-down", 14, settingsBlack)
							})
						}),
					)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if helper == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Caption(theme, helper)
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
					style := material.Caption(a.theme, "API Key")
					style.Color = settingsBlack
					return style.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.agentUI.settingsKeyLink.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, "获取 API Key ↗", 12, settingsBlack, false)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(unit.Dp(44))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
				return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
					editor := &a.agentUI.apiKeyInput
					if editor.Mask == 0 && editor.Len() == 0 && a.settingsSavedKeyMatchesDraft() && a.agentUI.settingsKeyPreview.secret != "" {
						editor = &a.agentUI.settingsKeyPreview.editor
					}
					settingsFieldFocus(gtx, editor)
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = 0
								hint := a.agentSettingsKeyHint()

								style := material.Editor(a.theme, editor, hint)
								style.TextSize = a.tokens().Typography.Body
								style.Color, style.HintColor = settingsBlack, settingsBlack
								return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, style.Layout)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
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
						}),
					)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			help := "保存后以掩码显示；API Key 保存在系统凭据库中。"
			if a.agentUI.settingsTab == "openai" {
				help = "在 OpenAI 控制台创建密钥；保存后以掩码显示。"
			} else if a.agentUI.settingsTab == "deepseek" {
				help = "在 DeepSeek 开放平台创建密钥；保存后以掩码显示。"
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Caption(a.theme, help)
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
		a.setStatus("请前往所选模型服务商的网站创建 API Key")
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
			a.setStatus("无法打开 API Key 页面")
		}
	}) {
		a.setStatus("无法打开 API Key 页面")
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
