package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
	"strings"
)

type wordParagraphAlignmentSetArguments struct {
	ParagraphId string `json:"paragraph_id"`
	Alignment   string `json:"alignment"`
}

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
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.no_paragraph_or_object_is_97b04f"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowInputMessage("word.layout.alignment", i18n.Message{ID: "word.layout.alignment.dialog.title"}, i18n.Message{ID: "word.layout.alignment.dialog.body"},
		"center",
		i18n.Message{ID: "word.layout.alignment.dialog.hint"},
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
			if !a.runWordApplicationCommand("设置排列对齐", "word.paragraph.alignment.set", wordParagraphAlignmentSetArguments{ParagraphId: pID, Alignment: string(align)}) {
				a.runWordCommand("设置排列对齐", func(editor document.WordTextEditor) error {
					return editor.SetParagraphAlignment(pID, align)
				})
			}
		},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.layout.alignment.action.apply"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type showWordLayoutWrapDialogPayload struct {
	ImageID  string `json:"image_id"`
	WrapMode string `json:"wrap_mode"`
}

func (a *App) showWordLayoutWrapDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	imageID := state.selectedWordImageID
	image := findWordImage(wordBlocksForState(state), imageID)
	if image == nil {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.select_an_image_whose_wrapping_c02ca9"}, feedbackWarning, notificationToast)
		return
	}
	if !image.WrapEditable {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.this_image_s_wrapping_structure_72958e"}, feedbackWarning, notificationToast)
		return
	}
	if image.WrapMode == "inline" {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.this_image_already_uses_inline_dfcca7"}, feedbackInfo, notificationSuppressed)
		return
	}
	wrapModeLabelID := map[string]string{"none": "word.layout.wrap.mode.none", "square": "word.layout.wrap.mode.square", "tight": "word.layout.wrap.mode.tight", "through": "word.layout.wrap.mode.through", "top_and_bottom": "word.layout.wrap.mode.top_and_bottom"}[image.WrapMode]
	wrapModeLabel := image.WrapMode
	if wrapModeLabelID != "" {
		wrapModeLabel = a.text(i18n.Message{ID: wrapModeLabelID})
	}
	a.modal.ShowMessage("word.layout.wrap", i18n.Message{ID: "word.layout.wrap.dialog.title"}, i18n.Message{ID: "word.layout.wrap.dialog.body", Args: i18n.Args{"WrapMode": wrapModeLabel}}, func(action string) {
		a.modal.Hide()
		if action != "apply" {
			return
		}
		args := showWordLayoutWrapDialogPayload{ImageID: imageID, WrapMode: "inline"}
		if a.applicationSessionRegistered(state.documentID) {
			if !a.runWordApplicationCommand("设置图片文字环绕", "word.image.wrap.set", args) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_start_image_wrapping_fc78d3"}, feedbackWarning, notificationToast)
			}
			return
		}
		if !a.runWordImageWrapCommand("设置图片文字环绕", imageID, "inline") {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_start_image_wrapping_fc78d3"}, feedbackWarning, notificationToast)
		}
	}, modalDialogActionSpec{ID: "apply", LabelRef: i18n.Message{ID: "word.layout.wrap.action.inline"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
}

type showWordLayoutPositionDialogPayload struct {
	ImageID       string `json:"image_id"`
	ParagraphID   string `json:"paragraph_id"`
	AfterInlineID string `json:"after_inline_id"`
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
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.select_an_editable_inline_image_99e997"}, feedbackWarning, notificationToast)
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
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_target_paragraph_for_the_7b50dc"}, feedbackWarning, notificationToast)
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
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_image_is_already_at_b85f9d"}, feedbackInfo, notificationSuppressed)
		return
	}
	args := showWordLayoutPositionDialogPayload{ImageID: imageID, ParagraphID: paragraphID, AfterInlineID: afterInlineID}
	a.modal.ShowMessage("word.layout.position", i18n.Message{ID: "word.layout.position.dialog.title"}, i18n.Message{ID: "word.layout.position.dialog.body"}, func(action string) {
		a.modal.Hide()
		if action != "move" {
			return
		}
		if a.applicationSessionRegistered(state.documentID) {
			if !a.runWordApplicationCommand("移动图片位置", "word.image.move", args) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_start_image_positioning_c78912"}, feedbackWarning, notificationToast)
			}
			return
		}
		if !a.runWordImageCommand("移动图片位置", imageID, func(editor document.WordImageEditor) error {
			return editor.MoveInlineImage(imageID, paragraphID, afterInlineID)
		}, false) {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_start_image_positioning_c78912"}, feedbackWarning, notificationToast)
		}
	}, modalDialogActionSpec{ID: "move", LabelRef: i18n.Message{ID: "word.layout.position.action.move"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
}
