package graphcanvas

import (
	"image"
)

const (
	MinZoom = float32(0.02)
	MaxZoom = float32(5.0)
)

// Camera manages viewport transformation, panning, and zooming.
type Camera struct {
	OffsetX float32
	OffsetY float32
	Zoom    float32
}

// NewCamera initializes a camera with default 1.0 zoom centered at origin.
func NewCamera() *Camera {
	return &Camera{
		OffsetX: 0,
		OffsetY: 0,
		Zoom:    0.10, // Full galaxy overview zoom
	}
}

// ScreenToWorld converts screen pixel coordinates to world coordinates.
func (c *Camera) ScreenToWorld(screenPt image.Point, screenSize image.Point) (float32, float32) {
	halfW := float32(screenSize.X) * 0.5
	halfH := float32(screenSize.Y) * 0.5

	wx := (float32(screenPt.X) - halfW - c.OffsetX) / c.Zoom
	wy := (float32(screenPt.Y) - halfH - c.OffsetY) / c.Zoom
	return wx, wy
}

// WorldToScreen converts world coordinates to screen pixel coordinates.
func (c *Camera) WorldToScreen(wx, wy float32, screenSize image.Point) (float32, float32) {
	halfW := float32(screenSize.X) * 0.5
	halfH := float32(screenSize.Y) * 0.5

	sx := wx*c.Zoom + halfW + c.OffsetX
	sy := wy*c.Zoom + halfH + c.OffsetY
	return sx, sy
}

// Pan translates the camera by delta screen pixels.
func (c *Camera) Pan(deltaX, deltaY float32) {
	c.OffsetX += deltaX
	c.OffsetY += deltaY
}

// ZoomAt zooms in/out centered at a specific screen anchor point.
func (c *Camera) ZoomAt(screenAnchor image.Point, screenSize image.Point, factor float32) {
	oldZoom := c.Zoom
	newZoom := oldZoom * factor
	if newZoom < MinZoom {
		newZoom = MinZoom
	} else if newZoom > MaxZoom {
		newZoom = MaxZoom
	}
	if newZoom == oldZoom {
		return
	}

	// World point before zoom
	wx, wy := c.ScreenToWorld(screenAnchor, screenSize)

	c.Zoom = newZoom

	// Recompute offset so (wx, wy) remains under screenAnchor
	halfW := float32(screenSize.X) * 0.5
	halfH := float32(screenSize.Y) * 0.5

	c.OffsetX = float32(screenAnchor.X) - halfW - wx*c.Zoom
	c.OffsetY = float32(screenAnchor.Y) - halfH - wy*c.Zoom
}

// ViewportWorldBounds computes the world AABB currently visible in the window,
// with an extra margin for smooth clipping.
func (c *Camera) ViewportWorldBounds(screenSize image.Point, margin float32) (minX, minY, maxX, maxY float32) {
	x0, y0 := c.ScreenToWorld(image.Pt(0, 0), screenSize)
	x1, y1 := c.ScreenToWorld(screenSize, screenSize)

	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}

	return x0 - margin, y0 - margin, x1 + margin, y1 + margin
}
