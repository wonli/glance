package desktopui

import (
	"fmt"
	"glance/internal/document"
	"strings"
)

func (a *App) showWordCoverPageDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordCoverPageEditor)
	if !ok {
		a.setStatus("当前文档不支持插入封面")
		return
	}
	a.modal.ShowInput("插入封面", "请输入封面文档标题：", "工作报告", "输入主标题（副标题和作者将自动补全）", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		title := strings.TrimSpace(a.modal.Input.Text())
		if title == "" {
			title = "工作报告"
		}
		err := editor.InsertCoverPage(title, "业务总结与未来规划", "项目负责人")
		if err != nil {
			a.setStatus("插入封面失败：" + err.Error())
			return
		}
		if state.word != nil {
			state.setWordBlocks(state.word.WordBlocks())
			a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
		}
		a.setStatus("已成功在文档首页插入标准封面页")
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "插入封面"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordLanguageDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordLanguageEditor)
	if !ok {
		a.setStatus("当前文档不支持设置语言")
		return
	}
	runID := state.selectedWordRunID
	targetDesc := "当前选区"
	if runID == "" {
		targetDesc = "全文档"
	}
	prompt := fmt.Sprintf("请选择或输入要为%s设置的校对语言代码：", targetDesc)
	a.modal.ShowInput("语言设置", prompt, "zh-CN", "例如: zh-CN (中文), en-US (英语), ja-JP (日语)", func(action string) {
		a.modal.Hide()
		var lang string
		switch action {
		case "confirm":
			lang = strings.TrimSpace(a.modal.Input.Text())
		case "zh-CN":
			lang = "zh-CN"
		case "en-US":
			lang = "en-US"
		case "ja-JP":
			lang = "ja-JP"
		default:
			return
		}
		if lang == "" {
			lang = "zh-CN"
		}
		err := editor.SetLanguage(runID, lang)
		if err != nil {
			a.setStatus("设置语言失败：" + err.Error())
			return
		}
		a.setStatus(fmt.Sprintf("已成功将%s语言设置为 %s", targetDesc, lang))
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "应用自定义"},
		modalDialogActionSpec{ID: "zh-CN", Label: "简体中文"},
		modalDialogActionSpec{ID: "en-US", Label: "英语 (美国)"},
		modalDialogActionSpec{ID: "ja-JP", Label: "日语"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordBookmarkDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordBookmarkEditor)
	if !ok {
		a.setStatus("当前文档不支持插入书签")
		return
	}
	runID := state.selectedWordRunID
	a.modal.ShowInput("插入书签", "请输入书签名称（仅支持字母、数字与下划线）：", "Bookmark1", "例如 Bookmark_1", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		name := strings.TrimSpace(a.modal.Input.Text())
		if name == "" {
			name = "Bookmark1"
		}
		err := editor.InsertBookmark(runID, name)
		if err != nil {
			a.setStatus("插入书签失败：" + err.Error())
			return
		}
		if state.word != nil {
			state.setWordBlocks(state.word.WordBlocks())
			a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
		}
		a.setStatus(fmt.Sprintf("已成功插入书签 “%s”", name))
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "插入书签"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordHyperlinkDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordHyperlinkEditor)
	if !ok {
		a.setStatus("当前文档不支持插入超链接")
		return
	}
	runID := state.selectedWordRunID
	var defaultText string
	if run, ok := selectedWordRun(wordBlocksForState(state), runID); ok {
		defaultText = run.Text
	}
	if defaultText == "" {
		defaultText = "访问链接"
	}

	a.modal.ShowDualInput("插入超链接", "为当前位置设置超链接：",
		"https://example.com", "链接地址 (例如 https://example.com 或 #书签名称)",
		defaultText, "显示文本",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			urlVal := strings.TrimSpace(a.modal.Input.Text())
			textVal := strings.TrimSpace(a.modal.SecondInput.Text())
			if urlVal == "" {
				a.setStatus("超链接地址不能为空")
				return
			}
			if textVal == "" {
				textVal = urlVal
			}
			err := editor.InsertHyperlink(runID, textVal, urlVal)
			if err != nil {
				a.setStatus("插入超链接失败：" + err.Error())
				return
			}
			if state.word != nil {
				state.setWordBlocks(state.word.WordBlocks())
				a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
			}
			a.setStatus("已成功插入超链接")
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "插入"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordTextBoxDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordTextBoxEditor)
	if !ok {
		a.setStatus("当前文档不支持插入文本框")
		return
	}
	a.modal.ShowInput("插入文本框", "请输入文本框引用文字：", "重要提示文本", "输入内容", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		text := strings.TrimSpace(a.modal.Input.Text())
		if text == "" {
			text = "文本框内容"
		}
		err := editor.InsertTextBox("", text)
		if err != nil {
			a.setStatus("插入文本框失败：" + err.Error())
			return
		}
		if state.word != nil {
			state.setWordBlocks(state.word.WordBlocks())
			a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
		}
		a.setStatus("已成功插入文本框")
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "插入文本框"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordHeaderDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordHeaderFooterEditor)
	if !ok {
		a.setStatus("当前文档不支持设置页眉")
		return
	}
	a.modal.ShowInput("插入页眉", "请输入页眉文字：", "文档标题", "输入页眉内容", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		text := strings.TrimSpace(a.modal.Input.Text())
		if text == "" {
			text = "页眉"
		}
		err := editor.SetHeader(text)
		if err != nil {
			a.setStatus("设置页眉失败：" + err.Error())
			return
		}
		a.setStatus("已成功设置页眉")
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "设置页眉"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordFooterDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordHeaderFooterEditor)
	if !ok {
		a.setStatus("当前文档不支持设置页脚")
		return
	}
	a.modal.ShowInput("插入页脚", "请输入页脚文字：", "内部机密 · 请勿外传", "输入页脚内容", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		text := strings.TrimSpace(a.modal.Input.Text())
		if text == "" {
			text = "页脚"
		}
		err := editor.SetFooter(text)
		if err != nil {
			a.setStatus("设置页脚失败：" + err.Error())
			return
		}
		a.setStatus("已成功设置页脚")
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "设置页脚"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordPageNumberDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordHeaderFooterEditor)
	if !ok {
		a.setStatus("当前文档不支持插入页码")
		return
	}
	a.modal.ShowInput("插入页码", "选择页码对齐方式：", "center", "居中(center), 靠右(right), 靠左(left)", func(action string) {
		a.modal.Hide()
		var align string
		switch action {
		case "confirm":
			align = strings.TrimSpace(a.modal.Input.Text())
		case "center":
			align = "center"
		case "right":
			align = "right"
		case "left":
			align = "left"
		default:
			return
		}
		if align == "" {
			align = "center"
		}
		err := editor.SetPageNumber(align)
		if err != nil {
			a.setStatus("插入页码失败：" + err.Error())
			return
		}
		a.setStatus("已成功在页脚插入页码")
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "center", Label: "居中对齐"},
		modalDialogActionSpec{ID: "right", Label: "靠右对齐"},
		modalDialogActionSpec{ID: "left", Label: "靠左对齐"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
