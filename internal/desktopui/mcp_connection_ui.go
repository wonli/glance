package desktopui

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
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

func (a *App) openMCPSettingsPage() {
	a.openAgentSettingsPage()
	a.agentUI.settingsPageTab = "mcp"
	a.prepareMCPConnectionPage()
}
func (a *App) prepareMCPConnectionPage() {
	a.mcpConnectionConfig = ""
	a.mcpConnectionViewConfig = ""
	a.mcpConnectionError = ""
	a.mcpConfigCopied = false
	if err := a.refreshMCPConnection(); err != nil {
		a.mcpConnectionError = err.Error()
	}
}

func (a *App) refreshMCPConnection() error {
	if a == nil || a.desktopRuntime == nil {
		return errors.New("桌面会话不可用")
	}
	instance, available, err := a.desktopRuntime.desktopMCPIPCInstance()
	if err != nil {
		return fmt.Errorf("读取当前 Glance MCP 服务状态失败：%w", err)
	}
	a.mcpConnectionConfig = ""
	a.mcpConnectionViewConfig = ""
	a.mcpConnectionError = ""
	a.mcpTempExecutable = false
	a.mcpConfigurationList.List.Position = layout.Position{}
	if !available {
		return errors.New("当前 Glance 桌面 MCP 服务尚未启动")
	}
	configuration, err := mcpDesktopStdioConfiguration(instance.ExecutablePath)
	if err != nil {
		return err
	}
	a.mcpConnectionConfig = configuration
	a.mcpConnectionViewConfig = configuration
	a.mcpTempExecutable = mcpExecutableIsInTempDir(instance.ExecutablePath)
	return nil
}

func mcpExecutableIsInTempDir(executablePath string) bool {
	executablePath, err := filepath.Abs(executablePath)
	if err != nil {
		return false
	}
	tempDir, err := filepath.Abs(os.TempDir())
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(executablePath); err == nil {
		executablePath = resolved
	}
	if resolved, err := filepath.EvalSymlinks(tempDir); err == nil {
		tempDir = resolved
	}
	relative, err := filepath.Rel(tempDir, executablePath)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) {
		return false
	}
	return !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func mcpDesktopStdioConfiguration(executablePath string) (string, error) {
	if strings.TrimSpace(executablePath) == "" {
		return "", errors.New("当前 Glance 可执行文件路径为空")
	}
	configuration := struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}{
		Command: executablePath,
		Args:    []string{"mcp", "serve", "--stdio"},
	}
	encoded, err := json.MarshalIndent(configuration, "", "  ")
	if err != nil {
		return "", fmt.Errorf("生成 MCP stdio 配置失败：%w", err)
	}
	return string(encoded), nil
}

func (a *App) layoutMCPSettingsPage(gtx layout.Context) layout.Dimensions {
	if a.mcpCopyConfigButton.Clicked(gtx) && a.mcpConnectionConfig != "" {
		copyDesktopMCPValue(gtx, a.mcpConnectionConfig)
		a.mcpConfigCopied = true
	}
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
}

func copyDesktopMCPValue(gtx layout.Context, value string) {
	gtx.Execute(clipboard.WriteCmd{Type: "application/json", Data: io.NopCloser(strings.NewReader(value))})
}

func (a *App) layoutMCPConnectionDetailsPage(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return settingsBodyLabel(gtx, a.theme, a.tokens().Typography, "当前桌面会话", settingsBlack, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				message := "此配置连接当前 Glance 窗口中的文档和未保存更改。"
				style := settingsSecondaryLabel(a.theme, a.tokens().Typography, message)
				style.Color, style.MaxLines = settingsBlack, 2
				return style.Layout(gtx)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if a.mcpConnectionConfig == "" {
				style := settingsSecondaryLabel(a.theme, a.tokens().Typography, a.mcpConnectionError)
				style.Color, style.MaxLines = settingsBlack, 3
				return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, style.Layout)
			}
			return a.layoutMCPStdioConfiguration(gtx)
		}),
	)
}

func (a *App) layoutMCPStdioConfiguration(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return settingsBodyLabel(gtx, a.theme, a.tokens().Typography, "MCP 客户端配置", settingsBlack, true)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutMCPConnectionCopyButton(gtx, &a.mcpCopyConfigButton, a.mcpConfigCopied)
					}),
				)
			})
		}),
		layout.Flexed(8, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeRoundedPanel(gtx, color.NRGBA{R: 243, G: 244, B: 247, A: 255}, homeBorder, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.mcpConfigurationList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
							style := material.Body2(a.theme, a.mcpConnectionViewConfig)
							style.TextSize, style.Color, style.MaxLines = a.tokens().Typography.SettingsSecondary, settingsBlack, 0
							style.Font.Typeface = font.Typeface("monospace")
							return style.Layout(gtx)
						})
					})
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				message := "MCP 客户端通过 stdio 连接当前 Glance 会话；使用期间请保持该 Glance 运行。"
				if a.mcpTempExecutable {
					message += " 当前程序位于临时目录，退出或重启后此路径可能失效。"
				}
				style := settingsSecondaryLabel(a.theme, a.tokens().Typography, message)
				style.Color, style.MaxLines = settingsBlack, 3
				return style.Layout(gtx)
			})
		}),
		layout.Flexed(2, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
	)
}

func (a *App) layoutMCPConnectionCopyButton(gtx layout.Context, button *widget.Clickable, copied bool) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(32))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		if copied {
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, a.textID("common.feedback.copied"), 11, settingsBlack, false)
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
	steps := []struct{ number, title, body string }{
		{"01", "复制配置", "复制当前 Glance 的 stdio 配置，并粘贴到 Agent 客户端。"},
		{"02", "连接当前会话", "客户端可以操作当前窗口里的文档和未保存更改。"},
		{"03", "保持运行", "使用 MCP 时请保持该 Glance 桌面进程运行。"},
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
						return settingsBodyLabel(gtx, a.theme, a.tokens().Typography, "如何连接 MCP 客户端", settingsBlack, true)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							children := make([]layout.FlexChild, 0, len(steps))
							for _, step := range steps {
								step := step
								children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return homeTextLabel(gtx, a.theme, step.number, 12, settingsBlack, true)
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return homeTextLabel(gtx, a.theme, step.title, a.tokens().Typography.SettingsBody, settingsBlack, true)
														})
													}),
												)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Inset{Top: unit.Dp(6), Left: unit.Dp(31)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													style := settingsSecondaryLabel(a.theme, a.tokens().Typography, step.body)
													style.Color, style.MaxLines = settingsBlack, 4
													return style.Layout(gtx)
												})
											}),
										)
									})
								}))
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
						})
					}),
				)
			})
		}),
	)
}
