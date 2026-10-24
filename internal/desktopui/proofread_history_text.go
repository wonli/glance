package desktopui

import (
	"image"
	"strings"
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/proofhistory"
)

type proofreadingHistoryTextView struct {
	text       string
	spans      []proofhistory.TextChange
	selectable widget.Selectable
	rawButton  widget.Clickable
	rawOpen    bool
}

func (a *App) layoutProofreadingHistoryContent(gtx layout.Context, change proofhistory.PageChange) layout.Dimensions {
	if !change.TextAvailable {
		return a.layoutProofreadingHistoryDiff(gtx, change.Diff)
	}
	w := a.ensureProofreadingWorkbench()
	if w.historyTextViews == nil {
		w.historyTextViews = map[string]*proofreadingHistoryTextView{}
	}
	id := change.Entry.Hash
	view := w.historyTextViews[id]
	if view == nil {
		view = &proofreadingHistoryTextView{spans: change.DisplayChanges}
		if view.spans == nil {
			view.spans = proofhistory.ChangedTextLines(change.TextChanges)
		}
		var text strings.Builder
		for _, span := range view.spans {
			text.WriteString(span.Text)
		}
		view.text = text.String()
		w.historyTextViews[id] = view
	}
	if view.rawButton.Clicked(gtx) {
		view.rawOpen = !view.rawOpen
		gtx.Execute(op.InvalidateCmd{})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 6, Bottom: 8, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutProofreadingHistoryText(gtx, view, view.spans)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.rawButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				key := "proofread.history.raw_show"
				if view.rawOpen {
					key = "proofread.history.raw_hide"
				}
				label := material.Label(a.theme, 11, a.textID(key))
				label.Color = proofreadBlue
				return layout.Inset{Top: 5, Bottom: 8, Left: 8, Right: 8}.Layout(gtx, label.Layout)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !view.rawOpen {
				return layout.Dimensions{}
			}
			return a.layoutProofreadingHistoryDiff(gtx, change.Diff)
		}),
	)
}

func (a *App) layoutProofreadingHistoryText(gtx layout.Context, view *proofreadingHistoryTextView, spans []proofhistory.TextChange) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	if view.text == "" {
		return a.layoutProofreadingHistoryDiffLine(gtx, a.textID("proofread.history.text_unchanged"))
	}
	label := material.Label(a.theme, 13, view.text)
	label.Color = proofreadInk
	label.State = &view.selectable
	dims := label.Layout(gtx)
	// Replay identical shaped glyphs through the corresponding rune regions.
	// Only three layouts are needed, even for many edits; selection and wrapping
	// belong to one persistent Selectable and use the exact same font geometry.
	colored := func(added bool) op.CallOp {
		rec := op.Record(gtx.Ops)
		tint := label
		tint.State = nil
		tint.Color = proofreadHistoryDeleted
		if added {
			tint.Color = proofreadHistoryAdded
		}
		tint.Layout(gtx)
		return rec.Stop()
	}
	added, deleted := colored(true), colored(false)
	offset := 0
	var regions []widget.Region
	for _, span := range spans {
		end := offset + utf8.RuneCountInString(span.Text)
		if span.Kind != proofhistory.TextUnchanged {
			regions = view.selectable.Regions(offset, end, regions[:0])
			for _, region := range regions {
				bounds := clip.Rect(region.Bounds).Push(gtx.Ops)
				if span.Kind == proofhistory.TextAdded {
					added.Add(gtx.Ops)
				} else {
					deleted.Add(gtx.Ops)
					y := region.Bounds.Min.Y + region.Bounds.Dy()/2
					paint.FillShape(gtx.Ops, proofreadHistoryDeleted, clip.Rect(image.Rect(region.Bounds.Min.X, y, region.Bounds.Max.X, y+max(1, gtx.Dp(1)))).Op())
				}
				bounds.Pop()
			}
		}
		offset = end
	}
	return dims
}
