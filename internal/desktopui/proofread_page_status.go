package desktopui

import "image/color"

// Review indicators belong to the PDF page navigator (Pen wiBkP), never the
// editable X paper. Selection changes stroke width independently of status.
func (a *App) proofreadingPageStatusKey(page int) string {
	s := a.proofreading
	if s == nil || s.controller == nil || page < 0 || page >= len(s.pageOwnership) {
		return "proofread.history.unreviewed"
	}
	reviewed := s.pageOwnership[page].Reviewed
	hash := ""
	if reviewed {
		hash = s.history.confirmed[page]
		token, found := s.history.tokens[page]
		if hash != "" && (!found || token != s.controller.Model.ContentToken(proofreadingPageID(page))) {
			hash = "modified"
		}
	}
	return proofreadingStatusKey(reviewed, hash, page, &s.history)
}

func proofreadingThumbnailStatus(key string) (label string, fill, border, ink color.NRGBA) {
	fill = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	border = color.NRGBA{R: 221, G: 227, B: 232, A: 255}
	ink = proofreadMuted
	label = "proofread.pagination.status.unreviewed"
	switch key {
	case "proofread.history.confirmed_no_snapshot", "proofread.history.saved":
		fill = color.NRGBA{R: 241, G: 249, B: 245, A: 255}
		border = color.NRGBA{R: 111, G: 160, B: 136, A: 255}
		ink = color.NRGBA{R: 30, G: 119, B: 85, A: 255}
		label = "proofread.pagination.status.reviewed"
		if key == "proofread.history.saved" {
			label = "proofread.pagination.status.saved"
		}
	case "proofread.history.modified", "proofread.history.saving":
		fill = color.NRGBA{R: 255, G: 252, B: 243, A: 255}
		border = color.NRGBA{R: 216, G: 162, B: 69, A: 255}
		ink = color.NRGBA{R: 153, G: 107, B: 35, A: 255}
		label = "proofread.pagination.status.modified"
		if key == "proofread.history.saving" {
			label = "proofread.pagination.status.saving"
		}
	case "proofread.history.failed":
		fill = color.NRGBA{R: 255, G: 245, B: 242, A: 255}
		border = color.NRGBA{R: 198, G: 83, B: 72, A: 255}
		ink = color.NRGBA{R: 166, G: 71, B: 62, A: 255}
		label = "proofread.pagination.status.failed"
	}
	return
}
