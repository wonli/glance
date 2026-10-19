package desktopui

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"gioui.org/f32"
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

const wordRulerHeightDP = 28

var wordRulerBackground = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
var wordRulerMargin = color.NRGBA{R: 224, G: 228, B: 234, A: 255}
var wordRulerMajor = color.NRGBA{R: 124, G: 135, B: 149, A: 255}
var wordRulerMinor = color.NRGBA{R: 170, G: 179, B: 191, A: 255}
var wordRulerGlyph = color.NRGBA{R: 53, G: 95, B: 150, A: 255}

type wordParagraphIndentIntent struct {
	paragraphID       string
	leftEMU, rightEMU int64
	firstOffsetEMU    *int64
}

type wordRulerIndents struct{ left, right, first int64 }

func (a *App) layoutWordHorizontalRuler(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(wordRulerHeightDP))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	// A real window paginates asynchronously. Until its first usable page is
	// accepted, keep the ruler slot stable without exposing legacy canvas grips
	// or guessed page geometry. Existing accepted pages remain visible on edits.
	result := state.wordLayoutResult
	waiting := result == nil && (a.wordLayoutResults != nil || state.wordLayoutPendingKey != nil)
	unavailable := result != nil && (!result.CanPreview || len(result.Pages) == 0)
	if waiting || unavailable {
		state.wordRulerIndentDragSide = 0
		state.wordRulerIndentDragParagraphID = ""
		paint.FillShape(gtx.Ops, wordWorkspaceBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
		area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
		semantic.DescriptionOp(a.textID("word.accessibility.ruler")).Add(gtx.Ops)
		area.Pop()
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	g := wordRulerGeometryForState(gtx, state)
	p, selected := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
	a.processWordRulerDrag(gtx, state, p, selected, g)
	bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	defer bounds.Pop()
	semantic.DescriptionOp(a.textID("word.accessibility.ruler")).Add(gtx.Ops)
	paint.FillShape(gtx.Ops, wordWorkspaceBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
	bar := image.Rect(g.pageLeft, 0, g.pageRight, height)
	barClip := clip.UniformRRect(bar, gtx.Dp(2)).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, wordRulerBackground, clip.Rect(bar).Op())
	for _, r := range []image.Rectangle{image.Rect(g.pageLeft, 0, g.textLeft, height), image.Rect(g.textRight, 0, g.pageRight, height)} {
		paint.FillShape(gtx.Ops, wordRulerMargin, clip.Rect(r).Op())
	}
	baseline := height - gtx.Dp(1)
	// Quarter-centimetre subdivisions use document units, independently of viewport width.
	start := int(math.Ceil(float64(g.emu(float64(max(0, g.pageLeft)-g.textLeft))) / float64(wordEMUPerCM/4)))
	end := int(math.Floor(float64(g.emu(float64(min(gtx.Constraints.Max.X, g.pageRight)-g.textLeft))) / float64(wordEMUPerCM/4)))
	for quarter := start; quarter <= end; quarter++ {
		x := g.textLeft + g.pixels(int64(quarter)*wordEMUPerCM/4)
		tick := 4
		c := wordRulerMinor
		if quarter%4 == 0 {
			tick = 8
			c = wordRulerMajor
		} else if quarter%2 == 0 {
			tick = 6
		}
		paint.FillShape(gtx.Ops, c, clip.Rect(image.Rect(x, baseline-gtx.Dp(unit.Dp(tick)), x+max(1, gtx.Dp(1)), baseline)).Op())
		if quarter%4 == 0 && x >= g.textLeft && x <= g.textRight {
			a.layoutWordRulerText(gtx, strconv.Itoa(quarter/4), image.Pt(x, gtx.Dp(2)), true, 10)
		}
	}
	a.layoutWordRulerText(gtx, "cm", image.Pt(g.pageLeft+gtx.Dp(8), gtx.Dp(2)), false, 9)
	border := color.NRGBA{R: 204, G: 210, B: 219, A: 255}
	for _, r := range []image.Rectangle{image.Rect(g.pageLeft, 0, g.pageRight, gtx.Dp(1)), image.Rect(g.pageLeft, baseline, g.pageRight, height), image.Rect(g.pageLeft, 0, g.pageLeft+gtx.Dp(1), height), image.Rect(g.pageRight-gtx.Dp(1), 0, g.pageRight, height)} {
		paint.FillShape(gtx.Ops, border, clip.Rect(r).Op())
	}
	for _, x := range []int{g.textLeft, g.textRight} {
		paint.FillShape(gtx.Ops, color.NRGBA{R: 154, G: 169, B: 189, A: 255}, clip.Rect(image.Rect(x, 0, x+max(1, gtx.Dp(1)), height)).Op())
	}
	if selected {
		v := wordRulerDisplayIndents(state, p)
		for _, marker := range wordRulerMarkers(gtx, g, v) {
			paintWordRulerIndentMarker(gtx, marker)
			if (!a.isSaving() && !state.formatRunning.Load() && state.wordRulerIndentIntent == nil) || state.wordRulerIndentDragSide != 0 {
				if marker.side < -1 {
					if _, ok := any(state.word).(document.WordParagraphRulerEditor); !ok {
						continue
					}
				}
				area := clip.Rect(marker.hit).Push(gtx.Ops)
				semantic.DescriptionOp(marker.name).Add(gtx.Ops)
				pointer.CursorColResize.Add(gtx.Ops)
				state.wordRulerIndentDrag.Add(gtx.Ops)
				area.Pop()
			}
		}
	}
	barClip.Pop()
	if state.wordRulerIndentDragSide != 0 {
		area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
		pointer.CursorColResize.Add(gtx.Ops)
		state.wordRulerIndentDrag.Add(gtx.Ops)
		area.Pop()
	}
	// Legacy text-flow fallback keeps its existing canvas resize affordance.
	if !g.canonical {
		for _, edge := range []int{g.pageLeft, g.pageRight} {
			area := clip.Rect(image.Rect(max(0, edge-gtx.Dp(10)), 0, min(gtx.Constraints.Max.X, edge+gtx.Dp(10)), height)).Push(gtx.Ops)
			semantic.DescriptionOp(a.textID("word.accessibility.resize_ruler")).Add(gtx.Ops)
			pointer.CursorColResize.Add(gtx.Ops)
			state.wordCanvasResizeDrag.Add(gtx.Ops)
			area.Pop()
			paintWordRulerResizeHandle(gtx, edge, height)
		}
		a.updateWordCanvasResize(gtx, state, g.pageLeft, g.pageRight)
	}
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func wordRulerParagraphIndents(state *documentSessionState, p document.WordParagraph) (int64, int64) {
	if state == nil {
		return p.LeftIndentEMU, p.RightIndentEMU
	}
	v := wordRulerDisplayIndents(state, p)
	return v.left, v.right
}

func (a *App) layoutWordRulerText(gtx layout.Context, text string, pos image.Point, center bool, size unit.Sp) {
	gtx.Constraints.Min = image.Point{}
	label := material.Caption(a.theme, text)
	label.TextSize = size
	label.Color = color.NRGBA{R: 86, G: 97, B: 112, A: 255}
	rec := op.Record(gtx.Ops)
	dims := label.Layout(gtx)
	call := rec.Stop()
	if center {
		pos.X -= dims.Size.X / 2
	}
	off := op.Offset(pos).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
}

type wordRulerMarker struct {
	side, x, top, bottom int
	name                 string
	hit                  image.Rectangle
}

func wordRulerMarkers(gtx layout.Context, g wordRulerGeometry, v wordRulerIndents) []wordRulerMarker {
	specs := []struct {
		side, x, top, bottom int
		name                 string
	}{
		{-2, g.textLeft + g.pixels(v.left+v.first), 0, 7, "拖动调整段落首行缩进"},
		{-3, g.textLeft + g.pixels(v.left), 18, 25, "拖动调整段落悬挂缩进"},
		{-1, g.textLeft + g.pixels(v.left), 25, 28, "拖动调整段落左缩进"},
		{1, g.textRight - g.pixels(v.right), 19, 27, "拖动调整段落右缩进"},
	}
	markers := make([]wordRulerMarker, 0, 4)
	for _, s := range specs {
		top, bottom := gtx.Dp(unit.Dp(s.top)), gtx.Dp(unit.Dp(s.bottom))
		markers = append(markers, wordRulerMarker{s.side, s.x, top, bottom, s.name, image.Rect(s.x-gtx.Dp(9), top, s.x+gtx.Dp(9), bottom)})
	}
	return markers
}
func paintWordRulerIndentMarker(gtx layout.Context, m wordRulerMarker) {
	w := gtx.Dp(5)
	if m.side == -1 {
		paint.FillShape(gtx.Ops, wordRulerGlyph, clip.Rect(image.Rect(m.x-w, m.top, m.x+w, m.bottom)).Op())
		return
	}
	var path clip.Path
	path.Begin(gtx.Ops)
	base, tip := m.bottom, m.top
	if m.side == -2 {
		base, tip = m.top, m.bottom
	}
	path.MoveTo(f32.Pt(float32(m.x-w), float32(base)))
	path.LineTo(f32.Pt(float32(m.x+w), float32(base)))
	path.LineTo(f32.Pt(float32(m.x), float32(tip)))
	path.Close()
	spec := path.End()
	paint.FillShape(gtx.Ops, wordRulerGlyph, clip.Outline{Path: spec}.Op())
	paint.FillShape(gtx.Ops, wordRulerBackground, clip.Stroke{Path: spec, Width: float32(gtx.Dp(1))}.Op())
}
func paintWordRulerResizeHandle(gtx layout.Context, x, height int) {
	width := min(gtx.Dp(7), gtx.Constraints.Max.X)
	x = max(width/2, min(gtx.Constraints.Max.X-(width+1)/2, x))
	h := gtx.Dp(19)
	top := (height - h) / 2
	paint.FillShape(gtx.Ops, rulerResizeHandleColor, clip.UniformRRect(image.Rect(x-width/2, top, x+(width+1)/2, top+h), gtx.Dp(3)).Op(gtx.Ops))
}
