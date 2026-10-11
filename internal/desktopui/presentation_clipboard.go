package desktopui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"gioui.org/io/clipboard"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"glance/internal/application"
	"glance/internal/document"
)

const presentationClipboardMaxRunes = 32767

func presentationTextElement(v *presentationViewState) (uint32, document.PresentationElement, bool) {
	if v == nil || v.pictureCrop.active {
		return 0, document.PresentationElement{}, false
	}
	slide := v.currentSlide()
	if slide == nil {
		return 0, document.PresentationElement{}, false
	}
	_, element, ok := presentationSelectedStyleElement(v)
	if !ok || !element.CanEditTextStyle || (element.Kind != document.ElementKindText && element.Kind != document.ElementKindShape) {
		return 0, document.PresentationElement{}, false
	}
	return slide.ID, element, true
}

func (a *App) handlePresentationClipboardEvents(gtx layout.Context, v *presentationViewState) {
	if v == nil {
		return
	}
	if v.clipboardWritePending {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(v.clipboardWriteText))})
		v.clipboardWritePending = false
		v.clipboardWriteText = ""
	}
	for {
		ev, ok := gtx.Event(transfer.TargetFilter{Target: &v.clipboardTarget, Type: "application/text"})
		if !ok {
			break
		}
		if _, cancelled := ev.(transfer.CancelEvent); cancelled {
			v.clipboardReadPending = false
			v.clipboardDocumentID = ""
			a.setStatus("粘贴已取消")
			continue
		}
		data, ok := ev.(transfer.DataEvent)
		if !ok {
			continue
		}
		reader := data.Open()
		value, err := io.ReadAll(io.LimitReader(reader, 4*presentationClipboardMaxRunes+1))
		_ = reader.Close()
		v.clipboardReadPending = false
		clipboardDocumentID := v.clipboardDocumentID
		clipboardDocumentVersion := v.clipboardDocumentVersion
		pasteSlideID, pasteElementID := v.clipboardPasteSlideID, v.clipboardPasteElementID
		v.clipboardDocumentID = ""
		if err != nil {
			a.setStatus("读取剪贴板失败：" + err.Error())
			continue
		}
		if clipboardDocumentID == "" || a.documentSessionState == nil || a.documentSessionState.documentID != clipboardDocumentID || a.documentGeneration != clipboardDocumentVersion {
			a.setStatus("文档已切换，已取消粘贴")
			continue
		}
		text := string(value)
		if !utf8.Valid(value) || len(value) > 4*presentationClipboardMaxRunes || len([]rune(text)) > presentationClipboardMaxRunes {
			a.setStatus("粘贴失败：剪贴板文本超过 32767 个字符或不是有效 UTF-8")
			continue
		}
		if strings.ContainsAny(text, "\r\n") {
			a.setStatus("粘贴失败：当前文本元素只支持单段落；请先拆分换行内容")
			continue
		}
		if text == "" {
			a.setStatus("剪贴板中没有可粘贴的文本")
			continue
		}
		a.startPresentationTextPaste(v, pasteSlideID, pasteElementID, text)
	}
}

func (a *App) startPresentationTextCopy(v *presentationViewState) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if v.commandPending {
		return true
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return true
	}
	slideID, element, ok := presentationTextElement(v)
	if !ok {
		a.setStatus("请先选择可编辑文本框或含文本的形状")
		return true
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatus("演示文稿正在处理其他操作")
		return true
	}
	v.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.element.text.copy", "presentation_element", fmt.Sprint(element.ID), "", map[string]any{
			"slide_id": slideID, "element_id": element.ID,
		})
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: v, generation: generation, clipboardCopy: true, err: err}
		if err == nil {
			var data struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				Text      string `json:"text"`
			}
			if json.Unmarshal(result.Data, &data) != nil || data.SlideID != slideID || data.ElementID != element.ID {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.clipboardText = data.Text
				out.clipboardElementID = element.ID
			}
		}
		select {
		case v.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

func (a *App) startPresentationClipboardPaste(gtx layout.Context, v *presentationViewState) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if v.commandPending || v.clipboardReadPending {
		return true
	}
	slideID, element, ok := presentationTextElement(v)
	if !ok {
		a.setStatus("请先选择可编辑文本框或含文本的形状")
		return true
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return true
	}
	v.clipboardDocumentID = state.documentID
	v.clipboardDocumentVersion = a.documentGeneration
	v.clipboardPasteSlideID = slideID
	v.clipboardPasteElementID = element.ID
	v.clipboardReadPending = true
	gtx.Execute(clipboard.ReadCmd{Tag: &v.clipboardTarget})
	a.setStatus("正在读取剪贴板文本")
	return true
}

func (a *App) startPresentationTextPaste(v *presentationViewState, slideID uint32, elementID int64, text string) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if v.commandPending {
		return true
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return true
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatus("演示文稿正在处理其他操作")
		return true
	}
	v.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.element.text.set", "presentation_element", fmt.Sprint(elementID), "", map[string]any{
			"slide_id": slideID, "element_id": elementID, "text": text,
		})
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: v, generation: generation, clipboardPaste: true, clipboardText: text, clipboardElementID: elementID, err: err}
		if err == nil {
			var data struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				Changed   bool   `json:"changed"`
			}
			if json.Unmarshal(result.Data, &data) != nil || data.SlideID != slideID || data.ElementID != elementID {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.clipboardChanged = data.Changed
			}
		}
		select {
		case v.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}
