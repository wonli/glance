package desktopui

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
)

const (
	wordRulerHeightDP = 33
)

var wordRulerBackground = color.NRGBA{R: 250, G: 251, B: 252, A: 255}

const wordEMUPerDP = int64(9525)

type wordParagraphIndentIntent struct {
	paragraphID string
	leftEMU     int64
	rightEMU    int64
}

func (a *App) layoutWordHorizontalRuler(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(wordRulerHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	width := gtx.Constraints.Max.X
	pageWidth, pageLeft := wordPageGeometry(width, gtx.Dp(unit.Dp(state.wordCanvasWidthDP)))
	pageRight := pageLeft + pageWidth
	margin := wordPageHorizontalMargin(pageWidth, gtx.Dp(unit.Dp(72)))
	textLeft := pageLeft + margin
	textRight := pageLeft + pageWidth - margin
	selectedParagraph, hasSelectedParagraph := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, wordRulerBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, a.tokens().Colors.Toolbar.Divider, clip.Rect{Min: image.Pt(0, height-gtx.Dp(unit.Dp(1))), Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
			bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			semantic.DescriptionOp("Word 水平标尺").Add(gtx.Ops)
			bounds.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			baseLeft, baseRight := textLeft, textRight
			leftIndent, rightIndent := wordRulerParagraphIndents(state, selectedParagraph)
			leftMarker := max(pageLeft, min(pageRight, textLeft+max(0, wordEMUToPixels(gtx, leftIndent))))
			rightMarker := max(pageLeft, min(pageRight, textRight-max(0, wordEMUToPixels(gtx, rightIndent))))
			canStartIndentDrag := !a.isSaving() && !state.formatRunning.Load()
			indentDragActive := state.wordRulerIndentDragSide != 0
			if hasSelectedParagraph && (canStartIndentDrag || indentDragActive) {
				hitWidth := max(1, gtx.Dp(unit.Dp(9)))
				for _, marker := range []struct {
					x    int
					side int
					name string
				}{{leftMarker, -1, "拖动调整段落左缩进"}, {rightMarker, 1, "拖动调整段落右缩进"}} {
					if !canStartIndentDrag && marker.side != state.wordRulerIndentDragSide {
						continue
					}
					area := clip.Rect{Min: image.Pt(max(0, marker.x-hitWidth), 0), Max: image.Pt(min(width, marker.x+hitWidth), height)}.Push(gtx.Ops)
					semantic.DescriptionOp(marker.name).Add(gtx.Ops)
					pointer.CursorColResize.Add(gtx.Ops)
					state.wordRulerIndentDrag.Add(gtx.Ops)
					area.Pop()
				}
				a.updateWordRulerIndentDrag(gtx, state, selectedParagraph, leftMarker, rightMarker, pageLeft, pageRight, baseLeft, baseRight)
			}
			for _, edge := range []int{pageLeft, pageRight} {
				hitWidth := max(1, gtx.Dp(unit.Dp(10)))
				area := clip.Rect{Min: image.Pt(max(0, edge-hitWidth), 0), Max: image.Pt(min(width, edge+hitWidth), height)}.Push(gtx.Ops)
				semantic.DescriptionOp("拖动标尺调整画布宽度").Add(gtx.Ops)
				pointer.CursorColResize.Add(gtx.Ops)
				state.wordCanvasResizeDrag.Add(gtx.Ops)
				area.Pop()
				paintWordRulerResizeHandle(gtx, edge, height)
			}
			a.updateWordCanvasResize(gtx, state, pageLeft, pageRight)
			baseline := height - gtx.Dp(unit.Dp(1))
			tickStart := min(pageRight, pageLeft+gtx.Dp(unit.Dp(15)))
			tickEnd := max(tickStart, pageRight-gtx.Dp(unit.Dp(31)))
			tickCount := 15
			majorTickHeight := gtx.Dp(unit.Dp(12))
			mediumTickHeight := gtx.Dp(unit.Dp(8))
			minorTickHeight := gtx.Dp(unit.Dp(5))
			for index := 0; index < tickCount; index++ {
				x := wordRulerTickX(tickStart, tickEnd, index, tickCount-1)
				paint.FillShape(gtx.Ops, rulerTickColor, clip.Rect{Min: image.Pt(x, baseline-majorTickHeight), Max: image.Pt(x+gtx.Dp(unit.Dp(1)), baseline)}.Op())
				layoutWordRulerLabelAt(gtx, a.theme, strconv.Itoa(index+1), image.Pt(x+gtx.Dp(unit.Dp(3)), gtx.Dp(unit.Dp(1))))
				if index == tickCount-1 {
					continue
				}
				nextX := wordRulerTickX(tickStart, tickEnd, index+1, tickCount-1)
				for subdivision := 1; subdivision < 5; subdivision++ {
					minorX := x + int(math.Round(float64(nextX-x)*float64(subdivision)/5))
					tickHeight := minorTickHeight
					if subdivision == 2 {
						tickHeight = mediumTickHeight
					}
					paint.FillShape(gtx.Ops, rulerTickColor, clip.Rect{Min: image.Pt(minorX, baseline-tickHeight), Max: image.Pt(minorX+gtx.Dp(unit.Dp(1)), baseline)}.Op())
				}
			}
			leftIndent, rightIndent = wordRulerParagraphIndents(state, selectedParagraph)
			leftMarker = max(pageLeft, min(pageRight, textLeft+max(0, wordEMUToPixels(gtx, leftIndent))))
			rightMarker = max(pageLeft, min(pageRight, textRight-max(0, wordEMUToPixels(gtx, rightIndent))))
			paintWordRulerMarker(gtx, leftMarker, height)
			paintWordRulerMarker(gtx, rightMarker, height)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
	)
}

func wordRulerTickX(start, end, index, lastIndex int) int {
	if lastIndex <= 0 {
		return start
	}
	return start + int(math.Round(float64(end-start)*float64(index)/float64(lastIndex)))
}

func wordRulerParagraphIndents(state *documentSessionState, paragraph document.WordParagraph) (left, right int64) {
	left, right = paragraph.LeftIndentEMU, paragraph.RightIndentEMU
	if state == nil {
		return left, right
	}
	if intent := state.wordRulerIndentIntent; intent != nil && intent.paragraphID == paragraph.ID {
		return intent.leftEMU, intent.rightEMU
	}
	if state.wordRulerIndentDragParagraphID == paragraph.ID {
		if state.wordRulerIndentDragSide < 0 {
			left = state.wordRulerIndentDragPreviewEMU
		} else if state.wordRulerIndentDragSide > 0 {
			right = state.wordRulerIndentDragPreviewEMU
		}
	}
	return left, right
}

func wordEMUToPixels(gtx layout.Context, value int64) int {
	return int(math.Round(float64(value) / float64(wordEMUPerDP) * float64(gtx.Metric.PxPerDp)))
}

func (a *App) updateWordRulerIndentDrag(gtx layout.Context, state *documentSessionState, paragraph document.WordParagraph, leftMarker, rightMarker, pageLeft, pageRight, baseLeft, baseRight int) {
	for event, ok := state.wordRulerIndentDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal); ok; event, ok = state.wordRulerIndentDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal) {
		switch event.Kind {
		case pointer.Press:
			state.wordRulerIndentDragSide = 0
			if math.Abs(float64(event.Position.X-float32(leftMarker))) <= 10 {
				state.wordRulerIndentDragSide = -1
				state.wordRulerIndentDragStartEMU = paragraph.LeftIndentEMU
			} else if math.Abs(float64(event.Position.X-float32(rightMarker))) <= 10 {
				state.wordRulerIndentDragSide = 1
				state.wordRulerIndentDragStartEMU = paragraph.RightIndentEMU
			}
			if state.wordRulerIndentDragSide != 0 {
				state.wordRulerIndentDragStartX = event.Position.X
				state.wordRulerIndentDragPreviewEMU = state.wordRulerIndentDragStartEMU
				state.wordRulerIndentDragParagraphID = paragraph.ID
			}
		case pointer.Drag:
			if state.wordRulerIndentDragSide == 0 {
				continue
			}
			pixelsPerDP := max32(0.1, gtx.Metric.PxPerDp)
			deltaDP := float64(event.Position.X-state.wordRulerIndentDragStartX) / float64(pixelsPerDP)
			if state.wordRulerIndentDragSide > 0 {
				deltaDP = -deltaDP
			}
			value := state.wordRulerIndentDragStartEMU + int64(math.Round(deltaDP*float64(wordEMUPerDP)))
			marginDP := max(0, baseLeft-pageLeft)
			otherIndent := paragraph.LeftIndentEMU
			if state.wordRulerIndentDragSide < 0 {
				otherIndent = paragraph.RightIndentEMU
			}
			availableDP := max(0, baseRight-baseLeft-gtx.Dp(unit.Dp(72))-max(0, wordEMUToPixels(gtx, otherIndent)))
			minimum := -int64(marginDP) * wordEMUPerDP
			maximum := int64(availableDP) * wordEMUPerDP
			state.wordRulerIndentDragPreviewEMU = max(minimum, min(maximum, value))
		case pointer.Release:
			if state.wordRulerIndentDragSide != 0 && state.wordRulerIndentDragPreviewEMU != state.wordRulerIndentDragStartEMU {
				left, right := paragraph.LeftIndentEMU, paragraph.RightIndentEMU
				if state.wordRulerIndentDragSide < 0 {
					left = state.wordRulerIndentDragPreviewEMU
				} else {
					right = state.wordRulerIndentDragPreviewEMU
				}
				intent := &wordParagraphIndentIntent{paragraphID: paragraph.ID, leftEMU: left, rightEMU: right}
				state.wordRulerIndentIntent = intent
				if wordHasPendingTextDrafts(state) {
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

func paintWordRulerResizeHandle(gtx layout.Context, x, height int) {
	width := min(gtx.Dp(unit.Dp(7)), gtx.Constraints.Max.X)
	// Opening a sidebar can put the page edge exactly on the viewport edge.
	// Keep the full grip inside that viewport rather than clipping half of it.
	x = max(width/2, min(gtx.Constraints.Max.X-(width+1)/2, x))
	handleHeight := gtx.Dp(unit.Dp(19))
	top := (height - handleHeight) / 2
	paint.FillShape(gtx.Ops, rulerResizeHandleColor, clip.UniformRRect(image.Rect(x-width/2, top, x+(width+1)/2, top+handleHeight), gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
}

func layoutWordRulerLabelAt(gtx layout.Context, theme *material.Theme, value string, position image.Point) {
	label := material.Caption(theme, value)
	label.TextSize = unit.Sp(9)
	label.Color = rulerLabelColor
	recorder := op.Record(gtx.Ops)
	label.Layout(gtx)
	call := recorder.Stop()
	transform := op.Offset(position).Push(gtx.Ops)
	call.Add(gtx.Ops)
	transform.Pop()
}

func paintWordRulerMarker(gtx layout.Context, x, height int) {
	markerWidth := gtx.Dp(unit.Dp(7))
	markerHeight := gtx.Dp(unit.Dp(5))
	var marker clip.Path
	marker.Begin(gtx.Ops)
	marker.MoveTo(f32.Pt(float32(x-markerWidth/2), float32(height-markerHeight)))
	marker.LineTo(f32.Pt(float32(x+markerWidth/2), float32(height-markerHeight)))
	marker.LineTo(f32.Pt(float32(x), float32(height-gtx.Dp(unit.Dp(1)))))
	marker.Close()
	paint.FillShape(gtx.Ops, rulerMarkerColor, clip.Outline{Path: marker.End()}.Op())
}
