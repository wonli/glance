package desktopui

func (a *App) undoProofreadingPagination() {
	s := a.proofreading
	if s == nil || s.controller == nil || !s.controller.Model.Undo() {
		return
	}
	s.xPageMenuOpen = false
	resetProofreadingNavigatorInput(s)
	a.proofreadingModelChanged(false)
}
func (a *App) redoProofreading() {
	s := a.proofreading
	if s == nil || s.controller == nil || !s.controller.Model.Redo() {
		return
	}
	s.xPageMenuOpen = false
	resetProofreadingNavigatorInput(s)
	a.proofreadingModelChanged(false)
}
