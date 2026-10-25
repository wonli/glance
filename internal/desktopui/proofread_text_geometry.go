package desktopui

import (
	"image"
	"math"
)

// A text anchor is in document page units, before zoom, canvas panning, or
// the page's internal text scrolling. Font-size changes re-shape the editor
// and produce a new anchor for the same character offset.
type proofreadingTextPagePoint struct {
	X, Y float64
}

type proofreadingXTextGeometry struct {
	Scale            float64
	MarginX, MarginY float64
	WindowTop        float64
	WindowShiftY     int
	Content          image.Rectangle
}

func proofreadingXPageTextGeometry(page proofreadingXPage, size image.Point, windowTop, scrollTop float64) proofreadingXTextGeometry {
	g := proofreadingXTextGeometry{WindowTop: windowTop, WindowShiftY: int(math.Round(windowTop - scrollTop))}
	if page.Width <= 0 || page.Height <= 0 || size.X <= 0 || size.Y <= 0 {
		return g
	}
	g.Scale = min(float64(size.X)/page.Width, float64(size.Y)/page.Height)
	g.MarginX, g.MarginY = proofreadingTextMargins(page.Width, page.Height)
	marginX, marginY := int(math.Round(g.MarginX*g.Scale)), int(math.Round(g.MarginY*g.Scale))
	g.Content = image.Rect(marginX, marginY, max(marginX, size.X-marginX), max(marginY, size.Y-marginY))
	return g
}

func (g proofreadingXTextGeometry) pagePoint(bounds image.Rectangle) proofreadingTextPagePoint {
	return proofreadingTextPagePoint{
		X: g.MarginX + float64(bounds.Min.X),
		Y: g.MarginY + g.WindowTop + float64(bounds.Min.Y+bounds.Max.Y)/2,
	}
}

// Use the same rounded document-space window shift as the native editor.
// Round to display pixels only after applying the current page scale.
func (g proofreadingXTextGeometry) pixelPoint(point proofreadingTextPagePoint) image.Point {
	return g.Content.Min.Add(image.Pt(
		int(math.Round((point.X-g.MarginX)*g.Scale)),
		int(math.Round((point.Y-g.MarginY-g.WindowTop+float64(g.WindowShiftY))*g.Scale)),
	))
}
