package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"image"
	"image/color"
)

func (a *App) layoutProofreadingPageMenuRows(gtx layout.Context) layout.Dimensions {
	s := a.proofreading
	owned := proofreadingPageOwnership{}
	if s.xPageMenuPage >= 0 && s.xPageMenuPage < len(s.pageOwnership) {
		owned = s.pageOwnership[s.xPageMenuPage]
	}
	canSplit := func(up bool) bool {
		i := s.xPageMenuPage
		j := i + 1
		if up {
			j = i - 1
		}
		return s.modelInitialAssigned && !owned.Reviewed && j >= 0 && j < len(s.pageOwnership) && !s.pageOwnership[j].Reviewed && !s.pageOwnership[j].Locked
	}
	button := func(w unit.Dp, b *widget.Clickable, icon, key string, selected, enabled bool, action func()) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(w), gtx.Dp(30)))
			if selected {
				paint.FillShape(gtx.Ops, color.NRGBA{R: 232, G: 240, B: 246, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(4)).Op(gtx.Ops))
			}
			return layoutProofreadingMenuAction(gtx, a, b, icon, a.textID(key), enabled, action)
		}
	}
	return layout.Inset{Left: 7, Right: 7, Top: 5, Bottom: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(button(94, &s.xPageUp, "arrow-up-to-line", "proofread.menu.up", false, canSplit(true), nil)),
					layout.Rigid(button(94, &s.xInsertPageBreak, "arrow-down-to-line", "proofread.menu.down", false, canSplit(false), nil)),
					layout.Rigid(button(73, &s.xUndoPaginationMenu, "rotate-ccw", "common.action.undo", false, s.controller != nil && s.controller.Model.CanUndo(), nil)),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))
				paint.FillShape(gtx.Ops, color.NRGBA{R: 233, G: 237, B: 240, A: 255}, clip.Rect{Max: size}.Op())
				return layout.Dimensions{Size: image.Pt(size.X, gtx.Dp(5))}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(button(72, &s.xPageUnreviewed, "", "proofread.menu.unreviewed", !owned.Reviewed, true, nil)),
					layout.Rigid(button(72, &s.xPageReviewed, "", "proofread.menu.reviewed", owned.Reviewed, true, nil)),
				)
			}),
			layout.Rigid(button(144, a.ensureProofreadingWorkbench().button("page-history-menu"), "", "proofread.history.page", false, true, nil)),
		)
	})
}
