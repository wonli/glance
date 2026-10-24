package desktopui

import "glance/internal/proofreading"

// Legacy range metadata is used only during migration and as a derived menu view.
// Character ownership itself lives exclusively in proofreading.Model.
type proofreadingPageOwnership struct {
	Start, End       int
	Reviewed, Locked bool
	LockEnd          int
}

func (a *App) setProofreadingPageReviewed(reviewed bool) {
	s := a.proofreading
	if s.controller == nil {
		return
	}
	if err := s.controller.Model.Review(proofreading.PageID(s.xPageMenuPage+1), reviewed); err != nil {
		s.draftPersistError = err.Error()
		return
	}
	resetProofreadingNavigatorInput(s)
	a.proofreadingModelChanged(false)
}
func (a *App) splitProofreadingPage(up bool) {
	s := a.proofreading
	if s.controller == nil {
		return
	}
	i := s.xPageMenuPage
	if i < 0 || i >= len(s.xPageEditors) {
		return
	}
	id := proofreading.PageID(i + 1)
	snapshot := s.controller.Model.PageSnapshot(id)
	at, _ := s.xPageEditors[i].Selection()
	at += s.xEditorWindows[i].Start
	if caret := s.xEditorCarets[i]; caret.Valid {
		if stable, stableErr := proofreading.OffsetAt(snapshot, id, caret.Start); stableErr == nil {
			at = stable
		}
	}
	anchor, err := proofreading.AnchorAt(snapshot, id, at)
	if err != nil {
		s.draftPersistError = err.Error()
		return
	}
	resetProofreadingNavigatorInput(s)
	if err := s.controller.Model.SplitAt(id, anchor, up); err != nil {
		s.draftPersistError = err.Error()
		return
	}
	s.xPageMenuOpen = false
	a.proofreadingModelChanged(false)
}

func resetProofreadingNavigatorInput(s *proofreadingWorkspace) {
	if s == nil {
		return
	}
	s.navigator.pending = nil
	s.navigator.remainder = 0
	s.navigator.reveal = false
	s.navigator.feedback = 0
	s.navigator.sample = ""
	s.navigator.feedbackKey = "proofread.navigator.ready"
}
