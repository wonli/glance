package desktopui

import (
	"glance/internal/document"
)

// releaseMarkdownViewResources drops view-local caches, decoded images,
// background parse cancellations and widget state.
func (state *documentSessionState) releaseMarkdownViewResources() {
	if state == nil {
		return
	}
	if v := state.markdownView; v != nil {
		if v.parseCancel != nil {
			v.parseCancel()
			v.parseCancel = nil
		}
		v.releaseMarkdownCharts()
		v.images = nil
		v.errors = nil
		v.buttons = nil
		v.tasks = nil
		v.formulas = nil
		v.links = nil
		v.cachedText = ""
		v.sourceLines = nil
		v.editor.SetText("")
		v.search.SetText("")
		v.snapshot = document.MarkdownSnapshot{}
		state.markdownView = nil
	}
	if state.markdownDoc != nil {
		_ = state.markdownDoc.Close()
		state.markdownDoc = nil
	}
	requestMemoryRelease()
}
