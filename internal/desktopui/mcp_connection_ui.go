package desktopui

import (
	"encoding/json"
	"image"
	"image/color"
	"io"
	"strings"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (a *App) showMCPConnectionDialog() {
	info, available, err := a.desktopMCPConnectionInfo()
	a.mcpConnectionConfig = ""
	a.mcpConnectionError = ""
	a.mcpConfigCopied = false
	if err != nil {
		a.mcpConnectionError = err.Error()
	} else if !available {
		a.mcpConnectionError = "桌面 MCP 服务尚未启动，请重启 Glance 后重试。"
	} else {
		a.mcpConnectionConfig = info.Configuration
	}
	a.modal.ShowCustom("MCP 连接", "将此连接信息填入 MCP 客户端，客户端就能操作当前桌面会话。", 560,
		a.layoutMCPConnectionDetails,
		func(action string) { a.modal.Hide() },
		modalDialogActionSpec{ID: "close", Label: "关闭"},
	)
}

func (a *App) openMCPSettingsPage() {
	a.resetHomeContentScroll()
	a.prepareAgentSettingsPage()
	a.agentUI.settingsPageTab = "mcp"
	a.prepareMCPConnectionPage()
	a.homeSection = homeSectionSettings
}

func (a *App) prepareMCPConnectionPage() {
	a.mcpConnectionURL = ""
	a.mcpConnectionToken = ""
	a.mcpConnectionViewConfig = ""
	a.mcpConnectionConfig = ""
	a.mcpConnectionError = ""
	a.mcpConfigCopied = false
	a.mcpURLCopied = false
	a.mcpTokenCopied = false

	info, available, err := a.desktopMCPConnectionInfo()
	if err != nil {
		a.mcpConnectionError = err.Error()
		return
	}
	if !available {
		a.mcpConnectionError = "桌面 MCP 服务尚未启动，请重启 Glance 后重试。"
		return
	}
	a.mcpConnectionURL = info.URL
	a.mcpConnectionToken = info.Token
	a.mcpConnectionConfig = mcpConnectionDisplayConfig(info.Configuration)
	a.mcpConnectionViewConfig = a.mcpConnectionConfig
}

func mcpConnectionDisplayConfig(configuration string) string {
	var config struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal([]byte(configuration), &config); err != nil {
		return configuration
	}
	formatted, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return configuration
	}
	return string(formatted)
}

func (a *App) layoutMCPSettingsPage(gtx layout.Context) layout.Dimensions {
	if a.mcpCopyURLButton.Clicked(gtx) && a.mcpConnectionURL != "" {
		copyDesktopMCPValue(gtx, a.mcpConnectionURL)
		a.mcpURLCopied = true
	}
	if a.mcpCopyTokenButton.Clicked(gtx) && a.mcpConnectionToken != "" {
		copyDesktopMCPValue(gtx, a.mcpConnectionToken)
		a.mcpTokenCopied = true
	}
	if a.mcpCopyConfigButton.Clicked(gtx) && a.mcpConnectionConfig != "" {
		copyDesktopMCPValue(gtx, a.mcpConnectionConfig)
		a.mcpConfigCopied = true
	}
	if a.mcpConnectionError != "" {
		style := material.Body2(a.theme, a.mcpConnectionError)
		style.Color = settingsBlack
		style.MaxLines = 3
		return style.Layout(gtx)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			guideWidth := gtx.Dp(unit.Dp(272))
			if guideWidth > gtx.Constraints.Max.X/2 {
				guideWidth = gtx.Constraints.Max.X / 2
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return a.layoutMCPConnectionDetailsPage(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(30)), 0)}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = guideWidth, guideWidth
					return a.layoutMCPConnectionGuide(gtx)
				}),
			)
		}),
	)
}

func copyDesktopMCPValue(gtx layout.Context, value string) {
	gtx.Execute(clipboard.WriteCmd{Type: "text/plain", Data: io.NopCloser(strings.NewReader(value))})
}

func (a *App) layoutMCPConnectionDetailsPage(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutMCPConnectionValueField(gtx, "服务地址", a.mcpConnectionURL, &a.mcpCopyURLButton, a.mcpURLCopied)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutMCPConnectionValueField(gtx, "Bearer Token", a.mcpConnectionToken, &a.mcpCopyTokenButton, a.mcpTokenCopied)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						style := material.Caption(a.theme, "通用 HTTP MCP 连接参数")
						style.Color, style.MaxLines = settingsBlack, 1
						return style.Layout(gtx)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutMCPConnectionCopyButton(gtx, &a.mcpCopyConfigButton, "复制配置", a.mcpConfigCopied)
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(unit.Dp(174))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
				text := a.mcpConnectionViewConfig
				if text == "" {
					text = "{}"
				}
				style := material.Body2(a.theme, text)
				style.TextSize = unit.Sp(12)
				style.Color = settingsBlack
				style.Font.Typeface = font.Typeface("monospace")
				style.MaxLines = 8
				return homeRoundedPanel(gtx, color.NRGBA{R: 243, G: 244, B: 247, A: 255}, homeBorder, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(15), Bottom: unit.Dp(15), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, style.Layout)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Caption(a.theme, "Authorization 请求头已填入当前会话的完整 Bearer Token。")
				style.Color, style.MaxLines = settingsBlack, 1
				return style.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return homeIcon(gtx, "info", 16, settingsBlack)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							style := material.Caption(a.theme, "127.0.0.1 为本机地址，客户端需与 Glance 运行在同一台设备。")
							style.Color, style.MaxLines = settingsBlack, 2
							return style.Layout(gtx)
						})
					}),
				)
			})
		}),
	)
}

func (a *App) layoutMCPConnectionValueField(gtx layout.Context, label, value string, copyButton *widget.Clickable, copied bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			style := material.Caption(a.theme, label)
			style.Color, style.MaxLines = settingsBlack, 1
			return style.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(unit.Dp(44))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
				return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								style := material.Body2(a.theme, value)
								style.TextSize, style.Color = unit.Sp(13), settingsBlack
								style.Font.Typeface = font.Typeface("monospace")
								style.MaxLines = 1
								return layout.Stack{Alignment: layout.W}.Layout(gtx, layout.Stacked(style.Layout))
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return a.layoutMCPConnectionCopyButton(gtx, copyButton, "复制", copied)
						}),
					)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if label != "Bearer Token" {
					return layout.Dimensions{}
				}
				style := material.Caption(a.theme, "密钥用于授权访问当前会话，请仅提供给可信的 Agent。")
				style.Color, style.MaxLines = settingsBlack, 1
				return style.Layout(gtx)
			})
		}),
	)
}

func (a *App) layoutMCPConnectionCopyButton(gtx layout.Context, button *widget.Clickable, label string, copied bool) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(32))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		if copied {
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, "已复制", 11, settingsBlack, false)
					})
				})
			})
		}
		width := gtx.Dp(unit.Dp(32))
		gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return homeIcon(gtx, "copy", 16, settingsBlack)
		})
	})
}

func (a *App) layoutMCPConnectionGuide(gtx layout.Context) layout.Dimensions {
	steps := []struct {
		number string
		title  string
		body   string
	}{
		{"01", "打开客户端 MCP 设置", "在支持 HTTP MCP 的 Agent 中，添加一个新的 MCP 服务。"},
		{"02", "填入连接信息", "设置服务地址，并添加 Authorization 请求头，值为 Bearer + 空格 + Token。"},
		{"03", "建立连接", "保存后连接服务器，即可通过 Agent 操作当前桌面会话。"},
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width, height := gtx.Dp(unit.Dp(1)), gtx.Dp(unit.Dp(265))
			gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
			paint.FillShape(gtx.Ops, homeBorder, clip.Rect{Max: image.Pt(width, height)}.Op())
			return layout.Dimensions{Size: image.Pt(width, height)}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, "如何连接其他 Agent", 14, settingsBlack, true)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							rows := make([]layout.FlexChild, 0, len(steps))
							for _, step := range steps {
								step := step
								rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return homeTextLabel(gtx, a.theme, step.number, 12, settingsBlack, true)
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return homeTextLabel(gtx, a.theme, step.title, 13, settingsBlack, true)
														})
													}),
												)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Inset{Top: unit.Dp(6), Left: unit.Dp(31)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													style := material.Caption(a.theme, step.body)
													style.Color, style.MaxLines = settingsBlack, 4
													return style.Layout(gtx)
												})
											}),
										)
									})
								}))
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
						})
					}),
				)
			})
		}),
	)
}

func (a *App) layoutMCPConnectionDetails(gtx layout.Context) layout.Dimensions {
	if a.mcpConnectionError != "" {
		style := material.Body2(a.theme, a.mcpConnectionError)
		style.Color = settingsBlack
		style.MaxLines = 3
		return style.Layout(gtx)
	}
	if a.mcpConnectionConfig == "" {
		return layout.Dimensions{}
	}

	info, _, _ := a.desktopMCPConnectionInfo()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return mcpConnectionValue(gtx, a.theme, "服务地址", info.URL)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return mcpConnectionValue(gtx, a.theme, "Bearer Token", info.Token)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Caption(a.theme, "通用 HTTP MCP 连接参数")
				style.Color = settingsBlack
				style.MaxLines = 1
				return style.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Body2(a.theme, a.mcpConnectionConfig)
				style.TextSize = unit.Sp(12)
				style.Color = settingsBlack
				style.MaxLines = 6
				return homeRoundedPanel(gtx, color.NRGBA{R: 247, G: 248, B: 250, A: 255}, homeBorder, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, style.Layout)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				buttonLabel := "复制连接参数"
				if a.mcpConfigCopied {
					buttonLabel = "已复制"
				}
				button := material.Button(a.theme, &a.mcpCopyConfigButton, buttonLabel)
				if a.mcpCopyConfigButton.Clicked(gtx) {
					gtx.Execute(clipboard.WriteCmd{Type: "application/json", Data: io.NopCloser(strings.NewReader(a.mcpConnectionConfig))})
					a.mcpConfigCopied = true
				}
				return button.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Caption(a.theme, "服务只监听本机；Token 仅保存在当前进程，退出 Glance 后失效。")
				style.Color = settingsBlack
				style.MaxLines = 2
				return style.Layout(gtx)
			})
		}),
	)
}

func mcpConnectionValue(gtx layout.Context, theme *material.Theme, label, value string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			style := material.Caption(theme, label)
			style.Color = settingsBlack
			style.MaxLines = 1
			return style.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Body2(theme, value)
				style.TextSize = unit.Sp(13)
				style.Color = settingsBlack
				style.MaxLines = 2
				return style.Layout(gtx)
			})
		}),
	)
}
