package desktopui

import (
	"glance/internal/proofreading"
	"strings"
)

func (a *App) proofreadingPageLayoutCurrent(index int) bool {
	s := a.proofreading
	if s.controller == nil || index < 0 || index >= len(s.xPages) || s.controller.Layout.StyleRevision != s.xTextSizeStep {
		return false
	}
	p, ok := s.controller.Layout.Pages[proofreading.PageID(index+1)]
	if !ok {
		return false
	}
	text := s.xPages[index].Content
	preview := text
	if index+1 < len(s.xPages) && !s.pageOwnership[index].Reviewed && !s.pageOwnership[index+1].Reviewed {
		preview += s.xPages[index+1].Content
	}
	return strings.HasPrefix(p.Text, text) && strings.HasPrefix(preview, p.Text)
}

// Visible pages take priority over an old whole-book geometry job after restore.
// Initial allocation is the sole operation that still needs whole-book reflow.
func (a *App) processProofreadingLayoutRequests() {
	s := a.proofreading
	if len(s.modelRequestedPages) == 0 || !s.modelInitialAssigned {
		return
	}
	seen := map[proofreading.PageID]bool{}
	var requested []proofreading.PageID
	active := map[proofreading.PageID]bool{}
	for _, id := range s.modelLayoutActivePages {
		active[id] = true
	}
	covered := s.modelLayoutPending && !s.modelLayoutNeedsFull
	for _, id := range s.modelRequestedPages {
		if seen[id] || a.proofreadingPageLayoutCurrent(int(id)-1) {
			continue
		}
		seen[id] = true
		requested = append(requested, id)
		if !active[id] {
			covered = false
		}
	}
	s.modelRequestedPages = nil
	if len(requested) > 0 && !covered {
		a.startProofreadingModelLayoutForPages(false, requested)
	}
}
