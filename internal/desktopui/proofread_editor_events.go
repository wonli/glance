package desktopui

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"glance/internal/proofreading"
)

// Consume editor events before model-changing menu, layout or wheel actions.
// Rendering never copies editor text back into the document.
func (a *App) processProofreadingEditorEvents(gtx layout.Context) {
	s := a.proofreading
	if s == nil || s.controller == nil {
		return
	}
	for i := range s.xPageEditors {
		if s.visibleEditorPages != nil && !gtx.Source.Focused(&s.xPageEditors[i]) {
			visible := false
			for _, index := range s.visibleEditorPages {
				if index == i {
					visible = true
					break
				}
			}
			if !visible {
				continue
			}
		}
		editor := &s.xPageEditors[i]
		selected := false
		for {
			event, ok := editor.Update(gtx)
			if !ok {
				break
			}
			if _, ok := event.(widget.SelectEvent); ok {
				selected = true
			}
		}
		if editorHasActiveComposition(editor) {
			continue
		}
		value := editor.Text()
		changed := i < len(s.xEditorTexts) && value != s.xEditorTexts[i]
		id := proofreading.PageID(i + 1)
		if changed {
			w := s.xEditorWindows[i]
			if err := s.controller.Model.ReplaceRange(id, w.StartAnchor, w.EndAnchor, value); err != nil {
				s.draftPersistError = err.Error()
				continue
			}
			s.xEditorTexts[i] = value
			w.End = w.Start + len([]rune(value))
			snap := s.controller.Model.Snapshot()
			w.StartAnchor, _ = proofreading.AnchorAt(snap, id, w.Start)
			w.EndAnchor, _ = proofreading.AnchorAt(snap, id, w.End)
			s.xEditorWindows[i] = w
			a.proofreadingModelChanged(false)
		}
		if changed || selected {
			start, end := editor.Selection()
			snap := s.controller.Model.Snapshot()
			w := s.xEditorWindows[i]
			left, e1 := proofreading.AnchorAt(snap, id, w.Start+start)
			right, e2 := proofreading.AnchorAt(snap, id, w.Start+end)
			if e1 == nil && e2 == nil {
				s.xEditorCarets[i] = proofreadingEditorCaret{Start: left, End: right, Valid: true}
				s.xRevealCaret[i] = changed
			}
		}
	}
}
func (a *App) syncProofreadingEditors() {
	s := a.proofreading
	if s.controller == nil {
		return
	}
	nextSnapshot := s.controller.Model.Snapshot()
	for i := range s.xPages {
		a.syncProofreadingEditor(i, nextSnapshot)
	}
	s.xEditorsSnapshot = nextSnapshot
}

func (a *App) syncProofreadingEditor(i int, nextSnapshot proofreading.Snapshot) {
	s := a.proofreading
	p := s.xPages[i]
	editor := &s.xPageEditors[i]
	if editorHasActiveComposition(editor) {
		return
	}
	w := a.proofreadingVisibleEditorWindow(i, p.Content)
	runes := []rune(p.Content)
	display := string(runes[w.Start:w.End])
	value := editor.Text()
	previous := s.xEditorWindows[i]
	if value != display || w.Start != previous.Start {
		start, end := editor.Selection()
		rebase := func(at int, logical proofreading.Anchor, valid bool) int {
			if valid {
				if next, err := proofreading.OffsetAt(nextSnapshot, proofreading.PageID(i+1), logical); err == nil {
					return max(0, min(w.End-w.Start, next-w.Start))
				}
			}
			if anchor, err := proofreading.AnchorAt(s.xEditorsSnapshot, proofreading.PageID(i+1), previous.Start+at); err == nil {
				if next, err := proofreading.OffsetAt(nextSnapshot, proofreading.PageID(i+1), anchor); err == nil {
					return max(0, min(w.End-w.Start, next-w.Start))
				}
			}
			return min(len([]rune(display)), proofreadingRebaseCaret(value, display, at))
		}
		editor.SetText(display)
		caret := s.xEditorCarets[i]
		editor.SetCaret(rebase(start, caret.Start, caret.Valid), rebase(end, caret.End, caret.Valid))
	}
	w.StartAnchor, _ = proofreading.AnchorAt(nextSnapshot, proofreading.PageID(i+1), w.Start)
	w.EndAnchor, _ = proofreading.AnchorAt(nextSnapshot, proofreading.PageID(i+1), w.End)
	w.Initialized = true
	s.xEditorWindows[i] = w
	s.xEditorTexts[i] = display
}
func proofreadingRebaseCaret(old, next string, at int) int {
	a, b := []rune(old), []rune(next)
	prefix, suffix := 0, 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	if at <= prefix {
		return at
	}
	if at >= len(a)-suffix {
		return max(0, min(len(b), at+len(b)-len(a)))
	}
	return prefix
}
func (a *App) processProofreadingMenuEvents(gtx layout.Context) {
	s := a.proofreading
	if !s.xPageMenuOpen {
		return
	}
	for i := range s.xPageEditors {
		if editorHasActiveComposition(&s.xPageEditors[i]) {
			return
		}
	}
	if s.xInsertPageBreak.Clicked(gtx) {
		a.splitProofreadingPage(false)
	}
	if s.xPageUp.Clicked(gtx) {
		a.splitProofreadingPage(true)
	}
	if s.xPageUnreviewed.Clicked(gtx) {
		a.setProofreadingPageReviewed(false)
	}
	if s.xPageReviewed.Clicked(gtx) {
		a.setProofreadingPageReviewed(true)
	}
	if s.xUndoPaginationMenu.Clicked(gtx) {
		a.undoProofreadingPagination()
	}
}
func (a *App) processProofreadingHistoryKeys(gtx layout.Context) {
	s := a.proofreading
	if s.controller == nil {
		return
	}
	for i := range s.xPageEditors {
		if editorHasActiveComposition(&s.xPageEditors[i]) {
			return
		}
	}
	for {
		ev, ok := gtx.Event(key.Filter{Name: "Z", Required: key.ModShortcut, Optional: key.ModShift}, key.Filter{Name: "Y", Required: key.ModShortcut})
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		// Flush content events before history, while preventing Editor's private undo.
		a.processProofreadingEditorEvents(gtx)
		if e.Name == "Y" || e.Modifiers.Contain(key.ModShift) {
			a.redoProofreading()
		} else {
			a.undoProofreadingPagination()
		}
	}
}
func (a *App) proofreadingEditorComposing() bool {
	s := a.proofreading
	for i := range s.xPageEditors {
		if editorHasActiveComposition(&s.xPageEditors[i]) {
			return true
		}
	}
	return false
}
