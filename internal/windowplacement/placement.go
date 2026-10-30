// Package windowplacement keeps initial desktop windows within the display work area.
package windowplacement

import "image"

func fitToWorkArea(window, work image.Rectangle) image.Rectangle {
	if window.Empty() || work.Empty() {
		return window
	}
	width, height := window.Dx(), window.Dy()
	if width > work.Dx() || height > work.Dy() {
		// Leave room around an oversized initial window instead of making it
		// indistinguishable from a maximized window on a small display.
		width = min(width, max(1, work.Dx()*9/10))
		height = min(height, max(1, work.Dy()*9/10))
		x := work.Min.X + (work.Dx()-width)/2
		y := work.Min.Y + (work.Dy()-height)/2
		return image.Rect(x, y, x+width, y+height)
	}
	x := max(work.Min.X, min(window.Min.X, work.Max.X-width))
	y := max(work.Min.Y, min(window.Min.Y, work.Max.Y-height))
	return image.Rect(x, y, x+width, y+height)
}
