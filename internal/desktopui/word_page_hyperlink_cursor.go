package desktopui

import (
	"image"
	"math"
	"unicode/utf8"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"glance/internal/wordlayout"
)

func wordPageHasHyperlinks(state *documentSessionState, page wordlayout.PageScene) bool {
	targets := wordHyperlinkRunTargetsForState(state)
	for _, fragment := range page.Text {
		if targets[fragment.RunID] != "" {
			return true
		}
	}
	return false
}

func addWordPageHyperlinkCursorOps(gtx layout.Context, state *documentSessionState, page wordlayout.PageScene, pixelsPerPoint float64) {
	if pixelsPerPoint <= 0 || math.IsNaN(pixelsPerPoint) || math.IsInf(pixelsPerPoint, 0) {
		return
	}
	targets := wordHyperlinkRunTargetsForState(state)
	for _, fragment := range page.Text {
		if targets[fragment.RunID] == "" {
			continue
		}
		bounds := wordPageHyperlinkCursorBounds(fragment, pixelsPerPoint)
		if bounds.Empty() {
			continue
		}
		transform := op.Offset(bounds.Min).Push(gtx.Ops)
		area := clip.Rect{Max: bounds.Size()}.Push(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		area.Pop()
		transform.Pop()
	}
}

func wordPageHyperlinkCursorBounds(fragment wordlayout.TextFragment, pixelsPerPoint float64) image.Rectangle {
	left, right := fragment.XPT, fragment.XPT
	if len(fragment.Glyphs) > 0 {
		left = math.Inf(1)
		right = math.Inf(-1)
		for _, glyph := range fragment.Glyphs {
			glyphLeft := math.Min(glyph.XPT, glyph.XPT+glyph.AdvanceXPT)
			glyphRight := math.Max(glyph.XPT, glyph.XPT+glyph.AdvanceXPT)
			left = math.Min(left, glyphLeft)
			right = math.Max(right, glyphRight)
		}
	} else {
		fontSize := math.Max(1, fragment.Style.FontSize)
		leftWidth := math.Max(1, float64(utf8.RuneCountInString(fragment.Text))*fontSize*0.5)
		right = left + leftWidth
	}
	if math.IsInf(left, 0) || math.IsInf(right, 0) || right <= left {
		return image.Rectangle{}
	}
	fontSize := math.Max(1, fragment.Style.FontSize)
	top := fragment.BaselineYPT - math.Max(1, fontSize*0.9)
	bottom := fragment.BaselineYPT + math.Max(1, fontSize*0.3)
	return image.Rect(
		int(math.Floor(left*pixelsPerPoint)),
		int(math.Floor(top*pixelsPerPoint)),
		int(math.Ceil(right*pixelsPerPoint)),
		int(math.Ceil(bottom*pixelsPerPoint)),
	)
}
