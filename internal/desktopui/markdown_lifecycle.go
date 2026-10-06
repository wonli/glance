package desktopui

import (
	"gioui.org/layout"
)

func markdownHasPendingDraft(state *documentSessionState) bool {
	return state != nil && state.markdownDoc != nil && state.markdownView != nil && state.markdownView.text() != state.markdownView.sourceSynced
}
func (a *App) observeMarkdownDraft() {
	if a == nil || a.markdownView == nil || a.documentSessionState == nil {
		return
	}
	v := a.markdownView
	source, path := v.text(), a.path
	if source != v.observedDraft || path != v.observedPath {
		unlock := lockSessionAdapterWrite(a.documentSessionState)
		v.observedDraft, v.observedPath = source, path
		v.draftEpoch.Add(1)
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
