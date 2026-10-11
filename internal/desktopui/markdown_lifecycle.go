package desktopui

import (
	"gioui.org/layout"
)

func markdownHasPendingDraft(state *documentSessionState) bool {
	return state != nil && state.markdownDoc != nil && state.markdownView != nil && state.markdownView.text() != state.markdownView.sourceSynced
}

func setMarkdownDraftState(state *documentSessionState, view *markdownViewState, draft, composition bool) {
	if state == nil || view == nil {
		return
	}
	if draft && !view.draftPending.Load() {
		state.markdownDraftViews.Add(1)
		view.draftPending.Store(true)
	} else if !draft && view.draftPending.Load() {
		view.draftPending.Store(false)
		state.markdownDraftViews.Add(-1)
	}
	if composition && !view.compositionPending.Load() {
		state.markdownCompositionViews.Add(1)
		view.compositionPending.Store(true)
	} else if !composition && view.compositionPending.Load() {
		view.compositionPending.Store(false)
		state.markdownCompositionViews.Add(-1)
	}
}

func releaseMarkdownDraftState(state *documentSessionState) {
	if state != nil {
		setMarkdownDraftState(state, state.markdownView, false, false)
	}
}

func (a *App) observeMarkdownDraft() {
	if a == nil || a.markdownView == nil || a.documentSessionState == nil {
		return
	}
	v := a.markdownView
	source, path := v.text(), a.path
	draft, composing := source != v.sourceSynced, markdownEditorComposing(&v.editor)
	changed := source != v.observedDraft || path != v.observedPath
	if changed || v.draftPending.Load() != draft || v.compositionPending.Load() != composing {
		unlock := lockSessionAdapterWrite(a.documentSessionState)
		setMarkdownDraftState(a.documentSessionState, v, draft, composing)
		if changed {
			v.observedDraft, v.observedPath = source, path
			v.draftEpoch.Add(1)
		}
		unlock()
	}
}
func (a *App) updateMarkdownInput(gtx layout.Context) {
	v := a.ensureMarkdownView()
	for {
		if _, ok := v.editor.Update(gtx); !ok {
			break
		}
	}
	a.observeMarkdownDraft()
	a.flushMarkdownDraft()
}
func (a *App) prepareMarkdownDeparture() {
	if a == nil || a.markdownDoc == nil || a.markdownView == nil {
		return
	}
	a.flushMarkdownDraft()
	v := a.markdownView
	if v.parseCancel != nil {
		v.parseCancel()
	}
	if v.pending {
		v.pending = false
		v.images = nil
		v.parseGeneration++
	}
	unlock := lockSessionAdapterWrite(a.documentSessionState)
	a.markdownView.draftEpoch.Add(1)
	unlock()
}
func (a *App) prepareMarkdownSave() bool {
	if a == nil || a.markdownDoc == nil {
		return true
	}
	if a.markdownView != nil && markdownEditorComposing(&a.markdownView.editor) {
		a.setStatus("请先完成输入法选字")
		return false
	}
	a.flushMarkdownDraft()
	return !markdownHasPendingDraft(a.documentSessionState)
}
