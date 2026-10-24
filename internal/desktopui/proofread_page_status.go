package desktopui

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"
	"image"
	"image/color"
)

func (a *App) drawProofreadingPageStatus(gtx layout.Context, page int, size image.Point) {
	s := a.proofreading
	if s.controller == nil || page >= len(s.pageOwnership) {
		return
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
	key := proofreadingStatusKey(reviewed, hash, page, &s.history)
	ink := proofreadMuted
	mark := "○ "
	if reviewed {
		ink = color.NRGBA{R: 30, G: 119, B: 85, A: 255}
		mark = "✓ "
		proofreadRoundedBorder(gtx, size, 0, ink, max(1, gtx.Dp(1)))
	}
	if key == "proofread.history.modified" || key == "proofread.history.failed" {
		ink = color.NRGBA{R: 162, G: 98, B: 27, A: 255}
		mark = "△ "
	}
	gtx.Constraints.Min = image.Point{}
	gtx.Constraints.Max = image.Pt(size.X, gtx.Dp(28))
	layout.Inset{Top: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := material.Label(a.theme, 10, fmt.Sprintf("%d · %s%s", page+1, mark, a.textID(key)))
		label.Color = ink
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Min}.Op())
			return layout.Dimensions{}
		}, label.Layout)
	})
}
