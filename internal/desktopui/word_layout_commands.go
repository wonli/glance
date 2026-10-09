package desktopui

import (
	"glance/internal/document"
	"strings"
)

func (a *App) showWordLayoutAlignDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	pID := state.selectedWordParagraphID
	if pID == "" {
		blocks := wordBlocksForState(state)
		for _, b := range blocks {
			if b.Paragraph != nil {
				pID = b.Paragraph.ID
				break
			}
		}
	}
	if pID == "" {
		a.setStatus("未找到可对齐的段落或对象")
		return
	}
	a.modal.ShowInput(
		"排列对齐",
		"请选择对齐方式（输入：left / center / right / both）：",
		"center",
		"left, center, right, both",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			val := strings.ToLower(strings.TrimSpace(a.modal.Input.Text()))
			var align document.WordAlignment
			switch val {
			case "center", "居中":
				align = document.WordAlignCenter
			case "right", "右对齐":
				align = document.WordAlignRight
			case "both", "两端对齐", "justify":
				align = document.WordAlignJustified
			default:
				align = document.WordAlignLeft
			}
			a.runWordCommand("设置排列对齐", func(editor document.WordTextEditor) error {
				return editor.SetParagraphAlignment(pID, align)
			})
		},
		modalDialogActionSpec{ID: "confirm", Label: "应用对齐"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordLayoutWrapDialog() {
	a.modal.Show(
		"文字环绕",
		"当前文档对象环绕模式：\n· 嵌入型（Inline with Text）：对象与文字同行排版，保证版面流式重排稳定。\n· 四周型（Square）/ 紧密型（Tight）：文字环绕对象外边距排版。\n\n已默认为对象应用标准嵌入型排版。",
		func(action string) {
			a.modal.Hide()
		},
		modalDialogActionSpec{ID: "close", Label: "确定"},
	)
}

func (a *App) showWordLayoutPositionDialog() {
	a.modal.Show(
		"对象位置",
		"当前文档排版位置配置：\n· 嵌入式排版：随正文自动流式分页与计算。\n· 页边距对齐：符合当前节（Section）页面设置边距规范。",
		func(action string) {
			a.modal.Hide()
		},
		modalDialogActionSpec{ID: "close", Label: "确定"},
	)
}
