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

	"glance/internal/mcpserver"
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
	a.mcpInstances = nil
	if a.mcpInstanceButtons == nil {
		a.mcpInstanceButtons = make(map[string]*widget.Clickable)
	}
	if err := a.refreshMCPInstances(); err != nil {
		a.mcpConnectionError = err.Error()
	}
}

func (a *App) refreshMCPInstances() error {
	if a == nil || a.desktopRuntime == nil {
		return errors.New("桌面会话不可用")
	}
	instances, err := mcpserver.ListDesktopMCPInstances(a.desktopRuntime.mcpIPCOptions)
	if err != nil {
		a.mcpInstances = nil
		a.mcpSelectedInstanceID = ""
		a.mcpConnectionConfig = ""
		a.mcpConnectionViewConfig = ""
		return fmt.Errorf("读取运行中的 Glance 实例失败：%w", err)
	}
	a.mcpInstances = instances
	for id := range a.mcpInstanceButtons {
		if !mcpInstanceIsListed(instances, id) {
			delete(a.mcpInstanceButtons, id)
		}
	}
	selected := a.mcpSelectedInstanceID
	if len(instances) == 1 {
		selected = instances[0].ID
	} else if !mcpInstanceIsListed(instances, selected) {
		selected = ""
	}
	a.mcpSelectedInstanceID = selected
	a.updateMCPDesktopConfiguration()
	return nil
}

func mcpInstanceIsListed(instances []mcpserver.DesktopMCPInstance, id string) bool {
	if id == "" {
		return false
	}
	for _, instance := range instances {
		if instance.ID == id {
			return true
		}
	}
	return false
}

func (a *App) selectMCPInstance(id string) {
	if a == nil || !mcpInstanceIsListed(a.mcpInstances, id) {
		return
	}
	a.mcpSelectedInstanceID = id
	a.mcpConfigCopied = false
	a.updateMCPDesktopConfiguration()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) updateMCPDesktopConfiguration() {
	a.mcpConnectionConfig = ""
	a.mcpConnectionViewConfig = ""
	a.mcpConnectionError = ""
	a.mcpTempExecutable = false
	a.mcpConfigurationList.List.Position = layout.Position{}
	for _, instance := range a.mcpInstances {
		if instance.ID != a.mcpSelectedInstanceID {
			continue
		}
		configuration, err := mcpDesktopStdioConfiguration(instance)
		if err != nil {
			a.mcpConnectionError = err.Error()
			return
		}
		a.mcpConnectionConfig = configuration
		a.mcpConnectionViewConfig = configuration
		a.mcpTempExecutable = mcpExecutableIsInTempDir(instance.ExecutablePath)
		return
	}
	if len(a.mcpInstances) == 0 {
		a.mcpConnectionError = "没有发现正在运行的 Glance 桌面实例。请先启动 Glance，再打开此页面。"
	} else if len(a.mcpInstances) > 1 {
		a.mcpConnectionError = "检测到多个 Glance 实例，请在上方列表中点击要连接的实例。"
	}
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

func mcpDesktopStdioConfiguration(instance mcpserver.DesktopMCPInstance) (string, error) {
	if strings.TrimSpace(instance.ID) == "" || strings.TrimSpace(instance.ExecutablePath) == "" {
		return "", errors.New("所选 Glance 实例缺少 ID 或可执行文件路径")
	}
	configuration := struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}{
		Command: instance.ExecutablePath,
		Args:    []string{"mcp", "serve", "--stdio", "--instance-id", instance.ID},
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
			return homeTextLabel(gtx, a.theme, "选择桌面实例", 15, settingsBlack, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				message := "选择 MCP 客户端要连接的 Glance 窗口。"
				if len(a.mcpInstances) > 1 {
					message = "运行中有多个 Glance，请点击对应的实例；相同程序也会分别列出。"
				}
				style := material.Caption(a.theme, message)
				style.Color, style.MaxLines = settingsBlack, 2
				return style.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, a.layoutMCPInstancePicker)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if a.mcpConnectionConfig == "" {
				style := material.Caption(a.theme, a.mcpConnectionError)
				style.Color, style.MaxLines = settingsBlack, 3
				return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, style.Layout)
			}
			return a.layoutMCPStdioConfiguration(gtx)
		}),
	)
}

func (a *App) layoutMCPInstancePicker(gtx layout.Context) layout.Dimensions {
	if len(a.mcpInstances) == 0 {
		return layout.Dimensions{}
	}
	listHeight := gtx.Dp(unit.Dp(194))
	if len(a.mcpInstances) == 1 {
		listHeight = gtx.Dp(unit.Dp(68))
	}
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = listHeight, listHeight
	return a.mcpInstancesList.Layout(gtx, len(a.mcpInstances), func(gtx layout.Context, index int) layout.Dimensions {
		instance := a.mcpInstances[index]
		button := a.mcpInstanceButtons[instance.ID]
		if button == nil {
			button = new(widget.Clickable)
			a.mcpInstanceButtons[instance.ID] = button
		}
		if button.Clicked(gtx) {
			a.selectMCPInstance(instance.ID)
		}
		return layout.Inset{Bottom: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				width, height := gtx.Constraints.Max.X, gtx.Dp(unit.Dp(58))
				gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
				fill := homeSurface
				border := homeBorder
				if instance.ID == a.mcpSelectedInstanceID {
					fill = color.NRGBA{R: 235, G: 242, B: 251, A: 255}
					border = color.NRGBA{R: 104, G: 139, B: 185, A: 255}
				}
				return mcpInstanceCardPanel(gtx, fill, border, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(10), Top: unit.Dp(7), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, fmt.Sprintf("Glance 窗口 · PID %d", instance.PID), 13, settingsBlack, true)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								style := material.Caption(a.theme, "程序："+filepath.Base(instance.ExecutablePath))
								style.TextSize, style.Color, style.MaxLines = unit.Sp(11), settingsBlack, 1
								style.Font.Typeface = font.Typeface("monospace")
								return style.Layout(gtx)
							}),
						)
					})
				})
			})
		})
	})
}

func mcpInstanceCardPanel(gtx layout.Context, fill, border color.NRGBA, radius unit.Dp, child layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Max
			borderWidth := gtx.Dp(unit.Dp(1))
			if borderWidth < 1 {
				borderWidth = 1
			}
			outerRadius := gtx.Dp(radius)
			innerRadius := outerRadius - borderWidth
			if innerRadius < 0 {
				innerRadius = 0
			}

			outer := clip.UniformRRect(image.Rectangle{Max: size}, outerRadius)
			paint.FillShape(gtx.Ops, border, outer.Op(gtx.Ops))
			innerBounds := image.Rect(borderWidth, borderWidth, size.X-borderWidth, size.Y-borderWidth)
			inner := clip.UniformRRect(innerBounds, innerRadius)
			paint.FillShape(gtx.Ops, fill, inner.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}, child,
	)
}

func (a *App) layoutMCPStdioConfiguration(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, "MCP 客户端配置", 14, settingsBlack, true)
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
							style.TextSize, style.Color, style.MaxLines = unit.Sp(12), settingsBlack, 0
							style.Font.Typeface = font.Typeface("monospace")
							return style.Layout(gtx)
						})
					})
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				message := "MCP 客户端通过 stdio 桥接进程连接所选实例；使用期间请保持该 Glance 运行。"
				if a.mcpTempExecutable {
					message += " 当前程序位于临时目录，退出或重启后此路径可能失效。"
				}
				style := material.Caption(a.theme, message)
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
	steps := []struct{ number, title, body string }{
		{"01", "选择实例", "在列表中点击需要连接的 Glance 窗口。多个窗口会分别显示。"},
		{"02", "复制配置", "复制 MCP 客户端配置，并粘贴到客户端的配置文件或设置页。"},
		{"03", "保持运行", "客户端通过 stdio 启动桥接进程；所选 Glance 退出后连接会结束。"},
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
						return homeTextLabel(gtx, a.theme, "如何连接 MCP 客户端", 14, settingsBlack, true)
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
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
						})
					}),
				)
			})
		}),
	)
}
