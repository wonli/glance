package desktopui

import (
	"errors"
	"fmt"
	"strconv"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/pptx"
)

// flushPresentationTextDraft commits the active canvas editor before a save or
// when the user switches to another shape. The editor text stays transient
// while typing so file-format work does not run on every Gio frame.
func (a *App) flushPresentationTextDraft() error {
	if a.pptxDoc == nil || a.pptxView == nil || a.pptxView.editingShape == 0 {
		return nil
	}
	v := a.pptxView
	text := v.shapeEditor.Text()
	var current string
	found := false
	if slide := v.currentSlide(); slide != nil {
		for _, elem := range slide.Elements {
			if elem.ID == v.editingShape && elem.Kind == document.ElementKindText {
				current = elem.PlainText()
				found = true
				break
			}
		}
	}
	if !found {
		return fmt.Errorf("找不到当前编辑的幻灯片文本框")
	}
	if current == text {
		v.shapeDraftText = text
		if a.documentSessionState != nil {
			a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationTextDraft, false)
		}
		return nil
	}
	state := a.documentSessionState
	var slideID uint32
	if slide := v.currentSlide(); slide != nil {
		slideID = slide.ID
	}
	if slideID == 0 {
		return fmt.Errorf("找不到当前幻灯片")
	}
	if state != nil && state.pptxDoc == a.pptxDoc && a.applicationSessionRegistered(state.documentID) {
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return fmt.Errorf("演示文稿正在处理其他操作")
		}
		_, err := a.executeRibbonApplicationCommand(token, "presentation.element.text.set", "presentation_element", strconv.FormatInt(v.editingShape, 10), "", map[string]any{
			"slide_id": slideID, "element_id": v.editingShape, "text": text,
		})
		state.finishOperation(token)
		if err != nil {
			a.setStatus("幻灯片文字更新失败：" + err.Error())
			return err
		}
	} else if state == nil || state.pptxDoc != a.pptxDoc {
		if err := a.pptxDoc.UpdateSlideTextByID(slideID, v.editingShape, text); err != nil {
			a.setStatus("幻灯片文字更新失败：" + err.Error())
			return err
		}
	} else {
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return fmt.Errorf("演示文稿正在处理其他操作")
		}
		unlock := lockSessionAdapterWrite(state)
		err := a.pptxDoc.UpdateSlideTextByID(slideID, v.editingShape, text)
		unlock()
		if err != nil {
			state.finishOperation(token)
			a.setStatus("幻灯片文字更新失败：" + err.Error())
			return err
		}
		a.completeSessionOperation(state, token, nil, true)
	}
	v.snapshot = a.pptxDoc.Snapshot()
	v.shapeDraftText = text
	if state != nil {
		state.setPresentationDraftDirty(a.windowID, presentationTextDraft, false)
	}
	a.setStatus("幻灯片文字已修改；点击保存写入文件")
	return nil
}

func (a *App) flushPresentationNotesDraft(v *presentationViewState) error {
	if a == nil || a.pptxDoc == nil || v == nil {
		return nil
	}
	if v.notesDraftConflict {
		return application.ErrPresentationNotesConflict
	}
	slide := v.currentSlide()
	if slide == nil || slide.ID == 0 {
		return nil
	}
	notes := v.notesEditor.Text()
	if notes == v.notesEditorBase {
		return nil
	}
	err := a.commitPresentationNotesDraft(v, slide.ID, v.notesEditorBase, notes)
	if errors.Is(err, application.ErrPresentationNotesConflict) {
		v.notesDraftConflict = true
		a.setStatus("演讲者备注已被其他操作修改；请重新加载或明确保留本地草稿")
	}
	return err
}

func (a *App) commitPresentationNotesDraft(v *presentationViewState, slideID uint32, expectedNotes, notes string) error {
	if a == nil || a.pptxDoc == nil || v == nil || slideID == 0 {
		return nil
	}
	state := a.documentSessionState
	if state != nil && state.pptxDoc == a.pptxDoc {
		state.setPresentationDraftDirty(a.windowID, presentationNotesDraft, true)
	}
	if state != nil && state.pptxDoc == a.pptxDoc && a.applicationSessionRegistered(state.documentID) {
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return fmt.Errorf("演示文稿正在处理其他操作")
		}
		_, err := a.executeRibbonApplicationCommand(token, "presentation.slide.notes.set", "presentation_slide", fmt.Sprint(slideID), "", map[string]any{
			"slide_id": slideID, "expected_notes": expectedNotes, "notes": notes,
		})
		state.finishOperation(token)
		if err != nil {
			return err
		}
	} else if state == nil || state.pptxDoc != a.pptxDoc {
		if _, err := a.pptxDoc.UpdateSlideNotesByIDIfCurrent(slideID, expectedNotes, notes); err != nil {
			if errors.Is(err, pptx.ErrSpeakerNotesConflict) {
				return application.ErrPresentationNotesConflict
			}
			return err
		}
	} else {
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return fmt.Errorf("演示文稿正在处理其他操作")
		}
		unlock := lockSessionAdapterWrite(state)
		changed, err := a.pptxDoc.UpdateSlideNotesByIDIfCurrent(slideID, expectedNotes, notes)
		unlock()
		if err != nil {
			state.finishOperation(token)
			if errors.Is(err, pptx.ErrSpeakerNotesConflict) {
				return application.ErrPresentationNotesConflict
			}
			return err
		}
		if changed {
			a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load()+1)
		}
		state.finishOperation(token)
	}
	if state != nil && state.pptxDoc == a.pptxDoc {
		state.setPresentationDraftDirty(a.windowID, presentationNotesDraft, false)
	}
	v.snapshot = a.pptxDoc.Snapshot()
	a.resetPresentationNotesEditor(v, v.currentSlide())
	return nil
}

func presentationTextDraftDirty(v *presentationViewState) bool {
	if v == nil || v.editingShape == 0 {
		return false
	}
	if slide := v.currentSlide(); slide != nil {
		for _, elem := range slide.Elements {
			if elem.ID == v.editingShape && elem.Kind == document.ElementKindText {
				return v.shapeEditor.Text() != elem.PlainText()
			}
		}
	}
	return false
}

func presentationNotesDraftDirty(v *presentationViewState) bool {
	return v != nil && (v.notesDraftConflict || v.notesEditor.Text() != v.notesEditorBase)
}
