package desktopui

import (
	"glance/internal/diagnostics"
	"glance/internal/wordlayout"
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

const wordPageCaretBlinkPeriod = time.Second

func (a *App) drawWordPageSearchHighlights(gtx layout.Context, state *documentSessionState, page wordlayout.PageScene, origin image.Point, pixelsPerPoint float64) {
	if state == nil || !a.modal.Visible || !a.modal.SearchReplace || state.wordSearchHighlightQuery == "" || a.modal.Input.Text() != state.wordSearchHighlightQuery {
		return
	}
	ranges := state.wordSearchHighlightRangesFor(a.modal.Input.Text())
	for _, fragment := range page.Text {
		for _, match := range ranges[fragment.RunID] {
			drawWordPageTextRange(gtx, fragment, match.Start, match.End, origin, pixelsPerPoint, wordPageSearchHighlightColor)
		}
	}
}

func drawWordPageSelection(gtx layout.Context, state *documentSessionState, page wordlayout.PageScene, origin image.Point, pixelsPerPoint float64) {
	if state.wordPageSelectionDrag != nil || wordPageTextSelectionActive(state.wordPageTextSelection) {
		recordWordSelectionDiagnostic(state, diagnostics.WordSelectionEvent{
			Phase: "paint", Page: page.Index, OriginX: origin.X, OriginY: origin.Y, PixelsPerPoint: pixelsPerPoint,
		})
	}
	if wordPageTextSelectionActive(state.wordPageTextSelection) {
		for _, fragment := range page.Text {
			start, end, selected := wordPageTextSelectionRange(state.wordPageTextSelection, fragment)
			if selected {
				drawWordPageFragmentSelection(gtx, fragment, start, end, origin, pixelsPerPoint)
			}
		}
		return
	}
	editor := state.wordRunEditors[state.selectedWordRunID]
	if editor == nil {
		return
	}
	start, end := editor.Selection()
	if start > end {
		start, end = end, start
	}
	if start == end {
		return
	}
	for _, fragment := range page.Text {
		if fragment.RunID == state.selectedWordRunID {
			drawWordPageFragmentSelection(gtx, fragment, start, end, origin, pixelsPerPoint)
		}
	}
}

func drawWordPageFragmentSelection(gtx layout.Context, fragment wordlayout.TextFragment, start, end int, origin image.Point, pixelsPerPoint float64) {
	drawWordPageTextRange(gtx, fragment, start, end, origin, pixelsPerPoint, wordPageSelectionColor)
}

func drawWordPageTextRange(gtx layout.Context, fragment wordlayout.TextFragment, start, end int, origin image.Point, pixelsPerPoint float64, ink color.NRGBA) {
	if start >= end {
		return
	}
	top := origin.Y + int(math.Round((fragment.BaselineYPT-fragment.Style.FontSize*0.88)*pixelsPerPoint))
	height := max(1, int(math.Ceil(fragment.Style.FontSize*1.18*pixelsPerPoint)))
	for _, glyph := range fragment.Glyphs {
		if glyph.RuneEnd <= start || glyph.RuneStart >= end {
			continue
		}
		left := origin.X + int(math.Round(glyph.XPT*pixelsPerPoint))
		right := origin.X + int(math.Ceil((glyph.XPT+glyph.AdvanceXPT)*pixelsPerPoint))
		if right <= left {
			right = left + 1
		}
		paint.FillShape(gtx.Ops, ink, clip.Rect{Min: image.Pt(left, top), Max: image.Pt(right, top+height)}.Op())
	}
}

func drawWordPageCaret(gtx layout.Context, state *documentSessionState, result *wordlayout.Result, pageIndex int, origin image.Point, pixelsPerPoint float64) {
	if state.selectedWordRunID == "" || result == nil || pageIndex < 0 || pageIndex >= len(result.Pages) {
		return
	}
	if wordPageTextSelectionActive(state.wordPageTextSelection) {
		return
	}
	editor := state.wordRunEditors[state.selectedWordRunID]
	if editor == nil || !gtx.Focused(editor) {
		return
	}
	start, end := editor.Selection()
	if start != end {
		return
	}
	position, ok := result.Locations[wordlayout.SourcePosition{RunID: state.selectedWordRunID, RuneOffset: end}]
	if !ok || position.PageIndex != pageIndex {
		return
	}
	if !wordPageCaretVisible(gtx, state, state.selectedWordRunID, end) {
		return
	}
	fontSize := wordlayout.DefaultFontSizePT
	for _, fragment := range result.Pages[pageIndex].Text {
		if fragment.RunID == state.selectedWordRunID {
			fontSize = math.Max(1, fragment.Style.FontSize)
			break
		}
	}
	x := origin.X + int(math.Round(position.XPT*pixelsPerPoint))
	top := origin.Y + int(math.Round((position.YPT-fontSize*0.86)*pixelsPerPoint))
	bottom := origin.Y + int(math.Round((position.YPT+fontSize*0.22)*pixelsPerPoint))
	paint.FillShape(gtx.Ops, wordPageCaretColor, clip.Rect{Min: image.Pt(x, top), Max: image.Pt(x+max(1, int(math.Ceil(pixelsPerPoint))), max(top+1, bottom))}.Op())
}

func wordPageCaretVisible(gtx layout.Context, state *documentSessionState, runID string, offset int) bool {
	if state.wordPageCaretBlinkRunID != runID || state.wordPageCaretBlinkOffset != offset || state.wordPageCaretBlinkStart.IsZero() {
		state.wordPageCaretBlinkRunID = runID
		state.wordPageCaretBlinkOffset = offset
		state.wordPageCaretBlinkStart = gtx.Now
	}
	elapsed := gtx.Now.Sub(state.wordPageCaretBlinkStart)
	if elapsed < 0 {
		state.wordPageCaretBlinkStart = gtx.Now
		elapsed = 0
	}
	phase := elapsed % wordPageCaretBlinkPeriod
	visibleDuration := wordPageCaretBlinkPeriod / 2
	var untilNext time.Duration
	if phase < visibleDuration {
		untilNext = visibleDuration - phase
	} else {
		untilNext = wordPageCaretBlinkPeriod - phase
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(untilNext)})
	return phase < visibleDuration
}

var (
	wordPageSearchHighlightColor = color.NRGBA{R: 255, G: 218, B: 70, A: 150}
	wordPageSelectionColor       = color.NRGBA{R: 76, G: 143, B: 222, A: 96}
	wordPageCaretColor           = color.NRGBA{R: 32, G: 104, B: 195, A: 255}
)
