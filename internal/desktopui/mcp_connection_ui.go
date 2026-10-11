package desktopui

import (
	"image/color"
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/unit"
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

func (a *App) layoutMCPConnectionDetails(gtx layout.Context) layout.Dimensions {
	if a.mcpConnectionError != "" {
		style := material.Body2(a.theme, a.mcpConnectionError)
		style.Color = color.NRGBA{R: 176, G: 54, B: 54, A: 255}
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
				style.Color = homeMuted
				style.MaxLines = 1
				return style.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Body2(a.theme, a.mcpConnectionConfig)
				style.TextSize = unit.Sp(12)
				style.Color = color.NRGBA{R: 56, G: 64, B: 75, A: 255}
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
				style.Color = homeMuted
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
			style.Color = homeMuted
			style.MaxLines = 1
			return style.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Body2(theme, value)
				style.TextSize = unit.Sp(13)
				style.Color = color.NRGBA{R: 39, G: 47, B: 57, A: 255}
				style.MaxLines = 2
				return style.Layout(gtx)
			})
		}),
	)
}
