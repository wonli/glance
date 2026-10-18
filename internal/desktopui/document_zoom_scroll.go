package desktopui

import (
	"image"
	"math"
	"runtime"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"

	"glance/internal/document"
)

func zoomScrollModifier(modifiers key.Modifiers) bool {
	return modifiers.Contain(key.ModShortcut) || modifiers.Contain(key.ModCtrl)
}

func zoomScrollScale() float64 {
	if runtime.GOOS == "windows" {
		// Gio forwards the native Windows wheel delta in 120-unit detents.
		return 0.1 / 120
	}
	// Gio forwards macOS and X11 wheel/touchpad distances as continuous values.
	return 0.01
}

func zoomScrollAxisDelta(scroll f32.Point) float64 {
	if scroll.Y != 0 {
		return float64(scroll.Y)
	}
	return float64(scroll.X)
}

func zoomFromScroll(current, scrollDelta, scale, remainder, minimum, maximum float64) (zoom, nextRemainder float64, changed bool) {
	if math.IsNaN(current) || math.IsInf(current, 0) || current <= 0 {
		current = 1
	}
	if math.IsNaN(remainder) || math.IsInf(remainder, 0) {
		remainder = 0
	}
	if scrollDelta == 0 || scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return current, remainder, false
	}
	candidate := current + remainder - scrollDelta*scale
	if candidate <= minimum {
		return minimum, 0, minimum != current
	}
	if candidate >= maximum {
		return maximum, 0, maximum != current
	}
	zoom = math.Round(candidate*100) / 100
	if zoom < minimum {
		zoom = minimum
	}
	if zoom > maximum {
		zoom = maximum
	}
	nextRemainder = candidate - zoom
	return zoom, nextRemainder, zoom != current
}

func (a *App) layoutWordZoomableBody(gtx layout.Context, state *documentSessionState, blocks []document.WordBlock, body layout.Widget) layout.Dimensions {
	recordNativePinchSurface(gtx)
	a.handleWordBodyScroll(gtx, state, blocks)
	dimensions := body(gtx)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	state.wordZoomScroll.Add(gtx.Ops)
	area.Pop()
	pass.Pop()
	return dimensions
}

func (a *App) handleWordBodyScroll(gtx layout.Context, state *documentSessionState, blocks []document.WordBlock) {
	const scrollLimit = 1 << 30
	filter := pointer.Filter{
		Target:  &state.wordZoomScroll,
		Kinds:   pointer.Scroll,
		ScrollX: pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
		ScrollY: pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
	}
	for event, ok := gtx.Event(filter); ok; event, ok = gtx.Event(filter) {
		pe, ok := event.(pointer.Event)
		if !ok || pe.Kind != pointer.Scroll {
			continue
		}
		if zoomScrollModifier(pe.Modifiers) {
			delta := zoomScrollAxisDelta(pe.Scroll)
			zoom, remainder, changed := zoomFromScroll(wordStatusZoomClamp(state.wordLayoutZoom), delta, zoomScrollScale(), state.zoomScrollRemainder, wordStatusZoomMinimum, wordStatusZoomMaximum)
			state.zoomScrollRemainder = remainder
			if changed {
				setWordWheelZoom(gtx, state, zoom, pe.Position)
				if a.window != nil {
					a.window.Invalidate()
				}
			}
			continue
		}
		if pe.Scroll.Y == 0 {
			continue
		}
		scroll := float64(pe.Scroll.Y) + state.wordScrollRemainder
		whole := int(scroll)
		state.wordScrollRemainder = scroll - float64(whole)
		if whole == 0 {
			continue
		}
		if wordBodyUsesPageList(state, blocks) {
			state.wordPageListState.Position.Offset += whole
		} else {
			state.wordListState.Position.Offset += whole
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}
}

func wordBodyUsesPageList(state *documentSessionState, blocks []document.WordBlock) bool {
	return state.wordViewMode != wordViewWebLayout &&
		!(state.wordLayoutResult == nil && state.wordLayoutError == "" && len(blocks) > 0)
}

// zoomAt keeps the grid point below the pointer fixed. Headers and frozen
// panes have no scroll offset of their own, so those axes retain their origin.
func (v *gridViewport) zoomAt(scale, x, y, width, height float64) {
	ratio := scale / v.Scale
	frozenX, frozenY := v.frozenViewportExtent(width, height)
	v.setScale(scale)
	dx, dy := 0.0, 0.0
	if x >= frozenX {
		dx = max(0, x) * (ratio - 1)
	}
	if y >= frozenY {
		dy = max(0, y) * (ratio - 1)
	}
	v.scroll(dx, dy, width, height)
}

// Wheel zoom preserves the current page arrangement and anchors to canonical
// page coordinates. Page layout remains unchanged; only the viewport changes.
func setWordWheelZoom(gtx layout.Context, state *documentSessionState, zoom float64, point f32.Point) {
	oldZoom := wordStatusZoomClamp(state.wordLayoutZoom)
	zoom = wordStatusZoomClamp(zoom)
	if zoom == oldZoom {
		return
	}
	state.wordLayoutZoom = zoom
	state.wordPageFitMode = false
	result := state.wordLayoutResult
	if state.wordViewMode == wordViewWebLayout || result == nil {
		return
	}
	for _, hit := range state.wordPageVisibleInteractions {
		if !image.Pt(int(point.X), int(point.Y)).In(hit.PageRect) || hit.PageIndex >= len(result.Pages) {
			continue
		}
		columns, gap, top := 1, 0, 0
		if state.wordViewMode == wordViewPrintLayout {
			top = gtx.Dp(unit.Dp(wordPageTopGapDP))
			if state.wordMultiplePagesMode {
				columns, gap = 2, gtx.Dp(unit.Dp(wordMultiplePageGapDP))
			}
		}
		row := hit.PageIndex / columns
		first := row * columns
		width := func(index int) int {
			page := result.Pages[index]
			return max(1, int(math.Ceil(page.Clip.WidthPT*wordScenePixelsPerPoint(gtx, page, zoom))))
		}
		total := width(first)
		pageLeft := 0
		if columns == 2 && first+1 < len(result.Pages) {
			if hit.PageIndex != first {
				pageLeft = total + gap
			}
			total += gap + width(first+1)
		}
		scale := wordScenePixelsPerPoint(gtx, result.Pages[hit.PageIndex], zoom)
		localX := (float64(point.X) - float64(hit.PageRect.Min.X)) / hit.Transform.PixelsPerPoint
		localY := (float64(point.Y) - float64(hit.PageRect.Min.Y)) / hit.Transform.PixelsPerPoint
		state.wordPageListState.Position.First = row
		state.wordPageListState.Position.Offset = int(math.Round(float64(top) + localY*scale - float64(point.Y)))
		state.wordPageScrollX = clamp(float64(max(0, (gtx.Constraints.Max.X-total)/2)+pageLeft)+localX*scale-float64(point.X), 0, float64(max(0, total-gtx.Constraints.Max.X)))
		return
	}
}
