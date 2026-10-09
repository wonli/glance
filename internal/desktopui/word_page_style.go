package desktopui

import (
	"fmt"
	"glance/internal/document"
	"strings"
)

func (a *App) showWordPageColorDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordPageBackgroundEditor)
	if !ok {
		a.setStatus("当前文档不支持设置页面背景色")
		return
	}
	currentColor, _ := editor.GetPageBackgroundColor()
	prompt := "请选择或输入页面底色："
	if currentColor != "" {
		prompt = fmt.Sprintf("当前页面底色为 #%s。请选择或修改底色：", currentColor)
	}

	a.modal.ShowInput("页面颜色", prompt, currentColor, "16 进制颜色代码（如 FAF8F5，留空为纯白）", func(action string) {
		a.modal.Hide()
		switch action {
		case "confirm":
			val := strings.TrimSpace(a.modal.Input.Text())
			a.applyWordPageColor(val)
		case "white":
			a.applyWordPageColor("")
		case "parchment":
			a.applyWordPageColor("FAF8F5")
		case "green":
			a.applyWordPageColor("E8F5E9")
		case "blue":
			a.applyWordPageColor("E3F2FD")
		case "dark":
			a.applyWordPageColor("263238")
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "应用自定义"},
		modalDialogActionSpec{ID: "parchment", Label: "羊皮纸米黄"},
		modalDialogActionSpec{ID: "green", Label: "护眼绿"},
		modalDialogActionSpec{ID: "white", Label: "纯白"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) applyWordPageColor(hexColor string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordPageBackgroundEditor)
	if !ok {
		return
	}
	clean := strings.TrimPrefix(strings.TrimSpace(hexColor), "#")
	if err := editor.SetPageBackgroundColor(clean); err != nil {
		a.setStatus("设置页面颜色失败：" + err.Error())
		return
	}
	if clean == "" || strings.EqualFold(clean, "none") {
		state.wordBackgroundColor = nil
		a.setStatus("已清除页面背景色")
	} else {
		c, err := parseHexColor(clean)
		if err == nil {
			state.wordBackgroundColor = &c
		}
		a.setStatus(fmt.Sprintf("已设置页面背景色为 #%s", strings.ToUpper(clean)))
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) showWordPageBorderDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordPageSetupEditor)
	if !ok {
		a.setStatus("当前文档不支持设置页面边框")
		return
	}
	prompt := "请选择页面边框样式："
	a.modal.ShowInput("页面边框", prompt, "box", "样式: box(单线), double(双线), dashed(虚线), none(无)", func(action string) {
		a.modal.Hide()
		var borderType string
		switch action {
		case "confirm":
			borderType = strings.TrimSpace(a.modal.Input.Text())
		case "box":
			borderType = "box"
		case "double":
			borderType = "double"
		case "dashed":
			borderType = "dashed"
		case "none":
			borderType = "none"
		default:
			return
		}
		if err := editor.SetPageBorders(borderType); err != nil {
			a.setStatus("设置页面边框失败：" + err.Error())
			return
		}
		if borderType == "none" || borderType == "" {
			a.setStatus("已清除页面边框")
		} else {
			a.setStatus(fmt.Sprintf("已设置页面边框（%s）", borderType))
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "应用自定义"},
		modalDialogActionSpec{ID: "box", Label: "单线方框"},
		modalDialogActionSpec{ID: "double", Label: "双线方框"},
		modalDialogActionSpec{ID: "dashed", Label: "虚线方框"},
		modalDialogActionSpec{ID: "none", Label: "无边框"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
