package desktopui

import (
	"errors"
	"fmt"
	"glance/internal/i18n"
	"strconv"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/pptx"
)

type

// flushPresentationTextDraft commits the active canvas editor before a save or
// when the user switches to another shape. The editor text stays transient
// while typing so file-format work does not run on every Gio frame.
presentationElementTextSetArguments struct {
	SlideId   uint32 `json:"slide_id"`
	ElementId int64  `json:"element_id"`
	Text      string `json:"text"`
}

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
		_, err := a.executeRibbonApplicationCommand(token, "presentation.element.text.set", "presentation_element", strconv.FormatInt(v.editingShape, 10), "", presentationElementTextSetArguments{SlideId: slideID, ElementId: v.editingShape, Text: text})
		state.finishOperation(token)
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
			return err
		}
	} else if state == nil || state.pptxDoc != a.pptxDoc {
		if err := a.pptxDoc.UpdateSlideTextByID(slideID, v.editingShape, text); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
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
			a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
			return err
		}
		a.completeSessionOperation(state, token, nil, true)
	}
	v.snapshot = a.pptxDoc.Snapshot()
	v.shapeDraftText = text
	if state != nil {
		state.setPresentationDraftDirty(a.windowID, presentationTextDraft, false)
	}
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.slide_text_changed_save_to_write_the_chang_94f5b5"}, feedbackSuccess, notificationToast)
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
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.speaker_notes_changed_in_another_operation_9711a6"}, feedbackInfo, notificationToast)
	}
	return err
}

type presentationSlideNotesSetArguments struct {
	SlideId       uint32 `json:"slide_id"`
	ExpectedNotes string `json:"expected_notes"`
	Notes         string `json:"notes"`
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
		_, err := a.executeRibbonApplicationCommand(token, "presentation.slide.notes.set", "presentation_slide", fmt.Sprint(slideID), "", presentationSlideNotesSetArguments{SlideId: slideID, ExpectedNotes: expectedNotes, Notes: notes})
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
