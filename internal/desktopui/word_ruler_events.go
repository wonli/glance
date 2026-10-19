package desktopui

import (
	"image"

	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"

	"glance/internal/document"
)

func wordRulerDisplayIndents(state *documentSessionState, p document.WordParagraph) wordRulerIndents {
	v := wordRulerResolvedIndents(state, p)
	if intent := state.wordRulerIndentIntent; intent != nil && intent.paragraphID == p.ID {
		v.left, v.right = intent.leftEMU, intent.rightEMU
		if intent.firstOffsetEMU != nil {
			v.first = *intent.firstOffsetEMU
		}
	}
	if state.wordRulerIndentDragSide != 0 && state.wordRulerIndentDragParagraphID == p.ID {
		v = state.wordRulerDragPreview
	}
	return v
}

func wordRulerDraggedIndents(g wordRulerGeometry, start wordRulerIndents, side int, delta float64) wordRulerIndents {
	v := start
	change := g.emu(delta)
	if change == 0 {
		return start
	}
	// Leave at least one point for text. Negative indents can use the margins.
	minLeft := g.emu(float64(g.pageLeft - g.textLeft))
	minRight := g.emu(float64(g.textRight - g.pageRight))
	if g.cellLimited {
		minLeft, minRight = g.minimumLeftEMU, g.minimumRightEMU
	}
	width := g.emu(float64(g.textRight-g.textLeft)) - 12700
	switch side {
	case -1:
		lower := max(minLeft, minLeft-start.first)
		upper := min(width-start.right, width-start.right-start.first)
		v.left = max(lower, min(max(lower, upper), start.left+change))
	case -2:
		absolute := max(minLeft, min(width-start.right, start.left+start.first+change))
		v.first = absolute - start.left
	case -3:
		absoluteFirst := start.left + start.first
		v.left = max(minLeft, min(max(minLeft, width-start.right), start.left+change))
		v.first = absoluteFirst - v.left
	case 1:
		upper := width - max(start.left, start.left+start.first)
		v.right = max(minRight, min(max(minRight, upper), start.right-change))
	}
	return v
}

func (a *App) processWordRulerDrag(gtx layout.Context, state *documentSessionState, p document.WordParagraph, selected bool, g wordRulerGeometry) {
	if state.wordRulerIndentDragSide != 0 {
		for e, ok := gtx.Event(key.Filter{Name: key.NameEscape}); ok; e, ok = gtx.Event(key.Filter{Name: key.NameEscape}) {
			if event, ok := e.(key.Event); ok && event.State == key.Press {
				state.wordRulerIndentDragSide = 0
				state.wordRulerIndentDragParagraphID = ""
			}
		}
	}
	// Target areas are registered in the ruler viewport without per-marker
	// transforms, so all pointer positions arrive in this same coordinate space.
	if state.wordRulerIndentDragSide != 0 && (!selected || state.wordRulerIndentDragParagraphID != p.ID || a.isSaving() || g.pixelsPerEMU != state.wordRulerDragGeometry.pixelsPerEMU) {
		state.wordRulerIndentDragSide = 0
		state.wordRulerIndentDragParagraphID = ""
	}
	for e, ok := state.wordRulerIndentDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal); ok; e, ok = state.wordRulerIndentDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal) {
		switch e.Kind {
		case pointer.Press:
			if !selected || a.isSaving() || state.formatRunning.Load() || state.wordRulerIndentIntent != nil {
				continue
			}
			v := wordRulerResolvedIndents(state, p)
			for _, m := range wordRulerMarkers(gtx, g, v) {
				if !image.Pt(int(e.Position.X), int(e.Position.Y)).In(m.hit) {
					continue
				}
				if m.side < -1 {
					if _, ok := any(state.word).(document.WordParagraphRulerEditor); !ok {
						continue
					}
				}
				state.wordRulerIndentDragSide = m.side
				state.wordRulerIndentDragParagraphID = p.ID
				state.wordRulerIndentDragStartX = e.Position.X
				state.wordRulerDragStart = v
				state.wordRulerDragPreview = v
				state.wordRulerDragGeometry = g
				state.wordRulerIndentDragStartEMU = v.left
				if m.side == 1 {
					state.wordRulerIndentDragStartEMU = v.right
				} else if m.side == -2 {
					state.wordRulerIndentDragStartEMU = v.first
				}
				state.wordRulerIndentDragPreviewEMU = state.wordRulerIndentDragStartEMU
				break
			}
		case pointer.Drag, pointer.Release:
			side := state.wordRulerIndentDragSide
			if side == 0 {
				continue
			}
			// Rebase when scrolling moves the document origin during a gesture.
			originShift := g.textLeft - state.wordRulerDragGeometry.textLeft
			delta := float64(e.Position.X-state.wordRulerIndentDragStartX) - float64(originShift)
			v := wordRulerDraggedIndents(g, state.wordRulerDragStart, side, delta)
			state.wordRulerDragPreview = v
			state.wordRulerIndentDragPreviewEMU = v.left
			if side == 1 {
				state.wordRulerIndentDragPreviewEMU = v.right
			} else if side == -2 {
				state.wordRulerIndentDragPreviewEMU = v.first
			}
			if e.Kind == pointer.Drag {
				continue
			}
			if v != state.wordRulerDragStart {
				intent := &wordParagraphIndentIntent{paragraphID: p.ID, leftEMU: v.left, rightEMU: v.right}
				if side == -2 || side == -3 {
					first := v.first
					intent.firstOffsetEMU = &first
				}
				state.wordRulerIndentIntent = intent
				if wordHasPendingTextDrafts(state) || state.formatRunning.Load() {
					state.wordForceDraftFlush.Store(true)
					state.wordFlushRequested.Store(true)
					state.wordFlushAllDrafts.Store(true)
					state.wordDeferredToolbarAction = "ruler-indents"
				} else {
					a.executeWordToolbarAction("ruler-indents")
				}
			}
			state.wordRulerIndentDragSide = 0
			state.wordRulerIndentDragParagraphID = ""
		case pointer.Cancel:
			state.wordRulerIndentDragSide = 0
			state.wordRulerIndentDragParagraphID = ""
		}
	}
}
