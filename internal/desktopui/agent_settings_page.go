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
		a.agentUI.modelInput.SetText("")
	}
	a.agentUI.settingsTab = settingsTabFor(a.agentUI.providerInput.Text(), a.agentUI.baseURLInput.Text())
	if a.agentUI.settingsTab == "" {
		a.agentUI.settingsTab = "more"
	}
	if a.agentUI.settingsTabButtons == nil {
		a.agentUI.settingsTabButtons = make(map[string]*widget.Clickable)
	}
}

func settingsTabFor(providerID, baseURL string) string {
	switch providerID {
	case "deepseek":
		return "deepseek"
	case "openai-compatible":
		if baseURL == "" || baseURL == "https://api.openai.com/v1" {
			return "openai"
		}
		return "custom"
	default:
		return "more"
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
	for _, tab := range []struct {
		id      string
		enabled bool
	}{
		{id: "openai", enabled: true},
		{id: "anthropic"},
		{id: "gemini"},
		{id: "deepseek", enabled: true},
		{id: "custom", enabled: true},
	} {
		if a.settingsProviderButton(tab.id).Clicked(gtx) {
			if tab.enabled {
				a.selectSettingsProvider(tab.id)
			} else {
				a.agentUI.settingsInfo = "该服务商暂未接入；当前可用 OpenAI、DeepSeek 和自定义兼容接口。"
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
		if a.agentUI.apiKeyInput.Mask == 0 {
			a.agentUI.apiKeyInput.Mask = '•'
		} else {
			a.agentUI.apiKeyInput.Mask = 0
		}
	}
	if a.agentUI.settingsKeyLink.Clicked(gtx) {
		a.openAgentAPIKeyHelp()
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeTextLabel(gtx, a.theme, "设置", 28, homeText, true)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						style := material.Body2(a.theme, "连接你常用的大模型，为文档对话和 AI 快捷操作提供支持。")
						style.TextSize = unit.Sp(14)
						style.Color = homeMuted
						style.MaxLines = 2
						return style.Layout(gtx)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
						return layout.Inset{Top: unit.Dp(30)}.Layout(gtx, a.layoutAgentSettingsPageFields)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return homeIcon(gtx, "check-square", 18, color.NRGBA{R: 54, G: 112, B: 208, A: 255})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return homeTextLabel(gtx, a.theme, "保存后将其设为默认 AI 提供商", 13, homeText, false)
									})
								}),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return agentSettingsPageButton(gtx, a.theme, &a.agentUI.settingsTest, "测试连接", false, a.agentUI.saving)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return agentSettingsPageButton(gtx, a.theme, &a.agentUI.settingsSave, "保存设置", true, a.agentUI.saving)
									})
								}),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							message := a.agentUI.settingsInfo
							textColor := homeMuted
							if a.agentUI.settingsError != "" {
								message, textColor = a.agentUI.settingsError, color.NRGBA{R: 178, G: 54, B: 54, A: 255}
							}
							style := material.Caption(a.theme, message)
							style.Color, style.MaxLines = textColor, 3
							return style.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							style := material.Caption(a.theme, "API Key 通过系统凭据库存储，不会写入普通配置文件。")
							style.Color = homeMuted
							style.MaxLines = 2
							return style.Layout(gtx)
						})
					}),
				)
			})
		}),
	)
}

func (a *App) layoutSettingsProviderTabs(gtx layout.Context) layout.Dimensions {
	// The horizontal Flex inherits its cross-axis constraints from the page's
	// remaining height. Clamp the tab strip so its buttons cannot stretch to fill
	// the entire settings page.
	height := gtx.Dp(unit.Dp(40))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height

	tabs := []struct {
		id      string
		label   string
		enabled bool
	}{
		{id: "openai", label: "OpenAI", enabled: true},
		{id: "anthropic", label: "Anthropic"},
		{id: "gemini", label: "Gemini"},
		{id: "deepseek", label: "DeepSeek", enabled: true},
		{id: "custom", label: "自定义", enabled: true},
	}
	if a.agentUI.settingsTab == "more" {
		tabs = append(tabs, struct {
			id      string
			label   string
			enabled bool
		}{id: "more", label: "更多", enabled: true})
	}
	children := make([]layout.FlexChild, 0, len(tabs))
	for _, tab := range tabs {
		tab := tab
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			button := a.settingsProviderButton(tab.id)
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				bg, fg := color.NRGBA{A: 0}, homeText
				if a.agentUI.settingsTab == tab.id {
					bg, fg = homeSelectedFill, homeText
				} else if !tab.enabled {
					fg = color.NRGBA{R: 171, G: 177, B: 185, A: 255}
				}
				return homeRoundedPanel(gtx, bg, bg, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeTextLabel(gtx, a.theme, tab.label, 13, fg, a.agentUI.settingsTab == tab.id)
						})
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
	case "deepseek":
		return "DeepSeek"
	case "custom":
		return "自定义服务"
	default:
		return "当前服务"
	}
}

func (a *App) layoutAgentSettingsPageFields(gtx layout.Context) layout.Dimensions {
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
	status, statusFG, statusBG := "未配置", homeMuted, homeHoverFill
	if configured {
		status, statusFG, statusBG = "已配置", color.NRGBA{R: 49, G: 133, B: 85, A: 255}, color.NRGBA{R: 232, G: 246, B: 238, A: 255}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, providerName, 20, homeText, true)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									style := material.Caption(a.theme, "配置此服务的 API Key、接口地址和默认模型。")
									style.Color = homeMuted
									return style.Layout(gtx)
								})
							}),
						)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// The panel paints from Max.Y, so bound it inside the list's unbounded item.
						height := gtx.Dp(unit.Dp(26))
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
			if a.agentUI.settingsTab == "custom" || a.agentUI.settingsTab == "more" {
				return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return agentSettingsPageField(gtx, a.theme, "服务商 ID", &a.agentUI.providerInput, "minimax / kimi / deepseek / ollama / openai-compatible")
				})
			}
			return layout.Dimensions{}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.agentUI.providerInput.Text() == "ollama" {
				style := material.Caption(a.theme, "Ollama 在本机运行，不需要 API Key。")
				style.Color = homeMuted
				return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, style.Layout)
			}
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutAgentSettingsKeyField(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return agentSettingsPageField(gtx, a.theme, "API 地址", &a.agentUI.baseURLInput, "https://api.example.com/v1")
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return agentSettingsPageField(gtx, a.theme, "默认模型", &a.agentUI.modelInput, "填写服务商提供的模型名")
		}),
	)
}

func agentSettingsPageField(gtx layout.Context, theme *material.Theme, label string, editor *widget.Editor, hint string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			style := material.Caption(theme, label)
			style.Color = homeText
			style.MaxLines = 1
			return style.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(unit.Dp(44))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
				return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						style := material.Editor(theme, editor, hint)
						style.TextSize = unit.Sp(14)
						style.Color = homeText
						style.HintColor = homeMuted
						return layout.Stack{Alignment: layout.W}.Layout(gtx, layout.Stacked(style.Layout))
					})
				})
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
					style.Color = homeText
					return style.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.agentUI.settingsKeyLink.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, "获取 API Key", 12, color.NRGBA{R: 54, G: 112, B: 208, A: 255}, false)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(unit.Dp(44))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
				return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								style := material.Editor(a.theme, &a.agentUI.apiKeyInput, "输入 API Key")
								style.TextSize = unit.Sp(14)
								style.Color, style.HintColor = homeText, homeMuted
								return layout.Stack{Alignment: layout.W}.Layout(gtx, layout.Stacked(style.Layout))
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return a.agentUI.settingsKeyToggle.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								icon := "eye-off"
								if a.agentUI.apiKeyInput.Mask == 0 {
									icon = "eye"
								}
								return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return homeIcon(gtx, icon, 18, homeMuted)
								})
							})
						}),
					)
				})
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
	width, height := gtx.Dp(unit.Dp(120)), gtx.Dp(unit.Dp(40))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	fg, bg, border := homeText, homeSurface, homeBorder
	if primary {
		fg, bg, border = color.NRGBA{R: 255, G: 255, B: 255, A: 255}, color.NRGBA{R: 54, G: 112, B: 208, A: 255}, color.NRGBA{R: 54, G: 112, B: 208, A: 255}
	}
	if disabled {
		fg, bg = homeMuted, homeHoverFill
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, bg, border, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeTextLabel(gtx, theme, label, 13, fg, true)
			})
		})
	})
}
