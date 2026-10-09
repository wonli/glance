package desktopui

import (
	"glance/internal/wordlayout"
	"image"
	"image/color"
	"math"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

func drawWordPageSelection(gtx layout.Context, state *documentSessionState, page wordlayout.PageScene, origin image.Point, pixelsPerPoint float64) {
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
	if start >= end {
		return
	}
	ink := wordPageSelectionColor
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
	fontSize := 11.0
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

var (
	wordPageSelectionColor = color.NRGBA{R: 76, G: 143, B: 222, A: 96}
	wordPageCaretColor     = color.NRGBA{R: 32, G: 104, B: 195, A: 255}
)
