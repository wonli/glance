package desktopui

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/op"
	"time"
)

func (a *App) queueProofreadingViewPersistence(gtx layout.Context) {
	s := a.proofreading
	if s == nil || s.controller == nil || s.draftRestoring {
		return
	}
	v := a.ensurePDFView()
	signature := fmt.Sprintf("%d/%d/%v", v.page, v.canvas.Position.Offset, s.xPageScrollOffsets)
	if signature == s.lastSavedView {
		return
	}
	if s.pendingView != signature {
		s.pendingView = signature
		s.viewSaveAt = gtx.Now.Add(400 * time.Millisecond)
	}
	if gtx.Now.Before(s.viewSaveAt) {
		gtx.Execute(op.InvalidateCmd{At: s.viewSaveAt})
		return
	}
	s.lastSavedView = signature
	a.queueProofreadingDraftPersistence()
}
