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
			if !a.runWordApplicationCommand("设置排列对齐", "word.paragraph.alignment.set", map[string]any{
				"paragraph_id": pID, "alignment": string(align),
			}) {
				a.runWordCommand("设置排列对齐", func(editor document.WordTextEditor) error {
					return editor.SetParagraphAlignment(pID, align)
				})
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "应用对齐"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordLayoutWrapDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	imageID := state.selectedWordImageID
	image := findWordImage(wordBlocksForState(state), imageID)
	if image == nil {
		a.setStatus("请先选择可安全调整环绕方式的图片")
		return
	}
	if !image.WrapEditable {
		a.setStatus("该图片的环绕结构暂不支持安全修改")
		return
	}
	if image.WrapMode == "inline" {
		a.setStatus("该图片已是嵌入型排版")
		return
	}
	message := "将选中图片从“" + image.WrapMode + "”浮动环绕转换为嵌入型。此操作会移除绝对位置和浮动环绕规则，保留图片、尺寸及正文内容；段落和分页可能变化。"
	a.modal.Show("文字环绕", message, func(action string) {
		a.modal.Hide()
		if action != "apply" {
			return
		}
		args := map[string]any{"image_id": imageID, "wrap_mode": "inline"}
		if a.applicationSessionRegistered(state.documentID) {
			if !a.runWordApplicationCommand("设置图片文字环绕", "word.image.wrap.set", args) {
				a.setStatus("无法启动图片环绕调整")
			}
			return
		}
		if !a.runWordImageWrapCommand("设置图片文字环绕", imageID, "inline") {
			a.setStatus("无法启动图片环绕调整")
		}
	}, modalDialogActionSpec{ID: "apply", Label: "转换为嵌入型"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) showWordLayoutPositionDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	blocks := wordBlocksForState(state)
	imageID := state.selectedWordImageID
	image := findWordImage(blocks, imageID)
	if image == nil || !image.Editable {
		a.setStatus("请先选择可编辑的行内图片")
		return
	}
	paragraphID := state.selectedWordParagraphID
	if _, ok := selectedWordParagraph(blocks, paragraphID); !ok {
		paragraphID = ""
		for _, paragraph := range allWordParagraphs(blocks) {
			for _, inline := range paragraph.Inlines {
				if inline.Image != nil && inline.Image.ID == imageID {
					paragraphID = paragraph.ID
					break
				}
			}
			if paragraphID != "" {
				break
			}
		}
	}
	if paragraphID == "" {
		a.setStatus("未找到图片目标段落")
		return
	}
	afterInlineID := ""
	if paragraph, ok := selectedWordParagraph(blocks, paragraphID); ok {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == state.selectedWordRunID {
				afterInlineID = inline.Text.ID
				break
			}
		}
	}
	if wordImageMoveIsNoop(blocks, imageID, paragraphID, afterInlineID) {
		a.setStatus("图片已位于目标文本位置")
		return
	}
	args := map[string]any{"image_id": imageID, "paragraph_id": paragraphID, "after_inline_id": afterInlineID}
	message := "将选中的图片移动到当前光标段落的行内位置。当前 DOCX 适配器支持正文流式排版，不创建浮动定位或文字环绕。"
	a.modal.Show("对象位置", message, func(action string) {
		a.modal.Hide()
		if action != "move" {
			return
		}
		if a.applicationSessionRegistered(state.documentID) {
			if !a.runWordApplicationCommand("移动图片位置", "word.image.move", args) {
				a.setStatus("无法启动图片位置调整")
			}
			return
		}
		if !a.runWordImageCommand("移动图片位置", imageID, func(editor document.WordImageEditor) error {
			return editor.MoveInlineImage(imageID, paragraphID, afterInlineID)
		}, false) {
			a.setStatus("无法启动图片位置调整")
		}
	}, modalDialogActionSpec{ID: "move", Label: "移动到此处"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}
