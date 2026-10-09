package desktopui

import (
	"glance/internal/wordlayout"
	"math"

	"gioui.org/f32"
)

func wordPageHitTest(page wordlayout.PageScene, point f32.Point) wordPageHit {
	return wordPageHitTestWithTolerance(page, point, 0)
}

// wordPageDevicePixelTolerance makes geometrical caret midpoints stable when
// they are rounded to an integer device pixel. Without the half-pixel band,
// repeated CJK glyph centers can all round just to the trailing side and place
// the caret one rune after the character under the pointer.
func wordPageDevicePixelTolerance(pixelsPerPoint float64) float64 {
	if pixelsPerPoint <= 0 || math.IsNaN(pixelsPerPoint) || math.IsInf(pixelsPerPoint, 0) {
		return 0
	}
	return 0.5 / pixelsPerPoint
}

func wordPageHitTestWithTolerance(page wordlayout.PageScene, point f32.Point, midpointTolerancePT float64) wordPageHit {
	for index := len(page.Images) - 1; index >= 0; index-- {
		placement := page.Images[index]
		bounds := placement.Bounds
		if float64(point.X) >= bounds.XPT && float64(point.X) <= bounds.XPT+bounds.WidthPT && float64(point.Y) >= bounds.YPT && float64(point.Y) <= bounds.YPT+bounds.HeightPT {
			return wordPageHit{imageID: placement.ImageID, found: true}
		}
	}
	best := wordPageHit{}
	bestScore := math.MaxFloat64
	for _, fragment := range page.Text {
		if !fragment.Editable || fragment.RunID == "" {
			continue
		}
		top := fragment.BaselineYPT - math.Max(1, fragment.Style.FontSize*0.9)
		bottom := fragment.BaselineYPT + math.Max(1, fragment.Style.FontSize*0.3)
		y := float64(point.Y)
		if y < top-3 || y > bottom+3 {
			continue
		}
		if len(fragment.Glyphs) == 0 {
			distanceX := math.Abs(float64(point.X) - fragment.XPT)
			distanceY := math.Abs(y - fragment.BaselineYPT)
			score := distanceY*4 + distanceX
			if score < bestScore {
				best = wordPageHit{runID: fragment.RunID, runeOffset: fragment.RuneStart, found: true}
				bestScore = score
			}
			continue
		}
		for _, glyph := range fragment.Glyphs {
			left := glyph.XPT
			right := glyph.XPT + glyph.AdvanceXPT
			if right < left {
				left, right = right, left
			}
			x := math.Max(left, math.Min(right, float64(point.X)))
			distanceX := math.Abs(float64(point.X) - x)
			distanceY := math.Abs(y - fragment.BaselineYPT)
			score := distanceY*4 + distanceX
			if score >= bestScore {
				continue
			}
			offset := glyph.RuneStart
			if float64(point.X) > (left+right)/2+midpointTolerancePT {
				offset = glyph.RuneEnd
			}
			best = wordPageHit{runID: fragment.RunID, runeOffset: offset, found: true}
			bestScore = score
		}
	}
	return best
}

// wordPageHitTestNearest extends an active drag to the nearest editable caret
// even when the pointer drifts outside the narrow text-line hit band. Clicks
// still use wordPageHitTest so blank page clicks do not unexpectedly focus text.
func wordPageHitTestNearest(page wordlayout.PageScene, point f32.Point) wordPageHit {
	return wordPageHitTestNearestWithTolerance(page, point, 0)
}

func wordPageHitTestNearestWithTolerance(page wordlayout.PageScene, point f32.Point, midpointTolerancePT float64) wordPageHit {
	best := wordPageHit{}
	bestScore := math.MaxFloat64
	y := float64(point.Y)
	for _, fragment := range page.Text {
		if !fragment.Editable || fragment.RunID == "" {
			continue
		}
		if len(fragment.Glyphs) == 0 {
			distanceX := math.Abs(float64(point.X) - fragment.XPT)
			distanceY := math.Abs(y - fragment.BaselineYPT)
			score := distanceY*4 + distanceX
			if score < bestScore {
				best = wordPageHit{runID: fragment.RunID, runeOffset: fragment.RuneStart, found: true}
				bestScore = score
			}
			continue
		}
		for _, glyph := range fragment.Glyphs {
			left, right := glyph.XPT, glyph.XPT+glyph.AdvanceXPT
			if right < left {
				left, right = right, left
			}
			x := math.Max(left, math.Min(right, float64(point.X)))
			distanceX := math.Abs(float64(point.X) - x)
			distanceY := math.Abs(y - fragment.BaselineYPT)
			score := distanceY*4 + distanceX
			if score >= bestScore {
				continue
			}
			offset := glyph.RuneStart
			if float64(point.X) > (left+right)/2+midpointTolerancePT {
				offset = glyph.RuneEnd
			}
			best = wordPageHit{runID: fragment.RunID, runeOffset: offset, found: true}
			bestScore = score
		}
	}
	return best
}

func wordPageHitTestRun(page wordlayout.PageScene, point f32.Point, runID string) wordPageHit {
	best := wordPageHit{}
	bestScore := math.MaxFloat64
	for _, fragment := range page.Text {
		if fragment.RunID != runID {
			continue
		}
		if len(fragment.Glyphs) == 0 {
			if !best.found {
				best = wordPageHit{runID: runID, runeOffset: fragment.RuneStart, found: true}
			}
			continue
		}
		for _, glyph := range fragment.Glyphs {
			left, right := glyph.XPT, glyph.XPT+glyph.AdvanceXPT
			if right < left {
				left, right = right, left
			}
			x := math.Max(left, math.Min(right, float64(point.X)))
			yDistance := math.Abs(float64(point.Y) - fragment.BaselineYPT)
			score := yDistance*2 + math.Abs(float64(point.X)-x)
			if score >= bestScore {
				continue
			}
			offset := glyph.RuneStart
			if float64(point.X) > (left+right)/2 {
				offset = glyph.RuneEnd
			}
			best, bestScore = wordPageHit{runID: runID, runeOffset: offset, found: true}, score
		}
	}
	return best
}
