package desktopui

import (
	"image"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"

	"glance/internal/document"
)

const presentationMinimumObjectSizePT = 8.0

// presentationTransform is the committed or preview geometry of one slide
// object. Pointer gestures keep this value in view state until release.
type presentationTransform struct {
	Bounds      document.PresentationRect
	RotationDeg float64
}

type presentationTransformHandle uint8

const (
	presentationHandleTopLeft presentationTransformHandle = iota
	presentationHandleTop
	presentationHandleTopRight
	presentationHandleRight
	presentationHandleBottomRight
	presentationHandleBottom
	presentationHandleBottomLeft
	presentationHandleLeft
)

// presentationDevicePointToSlide converts one normalized canvas point from
// physical device pixels into slide points. The canvas origin is expressed in
// the same pixel coordinate space as the point.
func presentationDevicePointToSlide(point, canvasOrigin f32.Point, canvasScale float64) document.PresentationPoint {
	if canvasScale <= 0 || math.IsNaN(canvasScale) || math.IsInf(canvasScale, 0) {
		canvasScale = 1
	}
	return document.PresentationPoint{
		X: float64(point.X-canvasOrigin.X) / canvasScale,
		Y: float64(point.Y-canvasOrigin.Y) / canvasScale,
	}
}

// presentationMovePreview returns the preview geometry for an unconstrained
// move. It deliberately allows the object to cross slide edges.
func presentationMovePreview(original presentationTransform, press, current, canvasOrigin f32.Point, canvasScale float64) presentationTransform {
	start := presentationDevicePointToSlide(press, canvasOrigin, canvasScale)
	end := presentationDevicePointToSlide(current, canvasOrigin, canvasScale)
	return presentationMovePreviewPoints(original, start, end)
}

func presentationMovePreviewPoints(original presentationTransform, start, end document.PresentationPoint) presentationTransform {
	original.Bounds.X += end.X - start.X
	original.Bounds.Y += end.Y - start.Y
	return original
}

// presentationInverseRotatePoint maps a slide-space point into an object's
// unrotated local coordinate space around its center.
func presentationInverseRotatePoint(point, center document.PresentationPoint, rotationDeg float64) document.PresentationPoint {
	angle := presentationNormalizeAngle(rotationDeg) * math.Pi / 180
	dx, dy := point.X-center.X, point.Y-center.Y
	cosine, sine := math.Cos(angle), math.Sin(angle)
	return document.PresentationPoint{
		X: center.X + dx*cosine + dy*sine,
		Y: center.Y - dx*sine + dy*cosine,
	}
}

// presentationResizePreview applies a pointer drag in the object's local
// coordinate space and keeps the opposite edge or corner anchored.
func presentationResizePreview(original presentationTransform, handle presentationTransformHandle, press, current, canvasOrigin f32.Point, canvasScale float64, aspectRatioLocked bool) presentationTransform {
	bounds := original.Bounds
	if bounds.W < presentationMinimumObjectSizePT || bounds.H < presentationMinimumObjectSizePT || canvasScale <= 0 {
		return original
	}
	startSlide := presentationDevicePointToSlide(press, canvasOrigin, canvasScale)
	currentSlide := presentationDevicePointToSlide(current, canvasOrigin, canvasScale)
	return presentationResizePreviewPoints(original, handle, startSlide, currentSlide, aspectRatioLocked)
}

func presentationResizePreviewPoints(original presentationTransform, handle presentationTransformHandle, startSlide, currentSlide document.PresentationPoint, aspectRatioLocked bool) presentationTransform {
	bounds := original.Bounds
	center := document.PresentationPoint{X: bounds.X + bounds.W/2, Y: bounds.Y + bounds.H/2}
	startLocal := presentationInverseRotatePoint(startSlide, center, original.RotationDeg)
	currentLocal := presentationInverseRotatePoint(currentSlide, center, original.RotationDeg)
	deltaX, deltaY := currentLocal.X-startLocal.X, currentLocal.Y-startLocal.Y
	left, right, top, bottom, ok := presentationResizeHandleSides(handle)
	if !ok {
		return original
	}
	if aspectRatioLocked {
		return presentationAspectLockedResize(original, deltaX, deltaY, left, right, top, bottom)
	}
	if left {
		newWidth := bounds.W - deltaX
		if newWidth < presentationMinimumObjectSizePT {
			bounds.X = bounds.X + bounds.W - presentationMinimumObjectSizePT
			bounds.W = presentationMinimumObjectSizePT
		} else {
			bounds.X += deltaX
			bounds.W = newWidth
		}
	} else if right {
		bounds.W = math.Max(presentationMinimumObjectSizePT, bounds.W+deltaX)
	}
	if top {
		newHeight := bounds.H - deltaY
		if newHeight < presentationMinimumObjectSizePT {
			bounds.Y = bounds.Y + bounds.H - presentationMinimumObjectSizePT
			bounds.H = presentationMinimumObjectSizePT
		} else {
			bounds.Y += deltaY
			bounds.H = newHeight
		}
	} else if bottom {
		bounds.H = math.Max(presentationMinimumObjectSizePT, bounds.H+deltaY)
	}
	original.Bounds = bounds
	return original
}

func presentationAspectLockedResize(original presentationTransform, deltaX, deltaY float64, left, right, top, bottom bool) presentationTransform {
	bounds := original.Bounds
	var scaleX, scaleY float64
	if left || right {
		signed := deltaX
		if left {
			signed = -signed
		}
		scaleX = (bounds.W + signed) / bounds.W
	}
	if top || bottom {
		signed := deltaY
		if top {
			signed = -signed
		}
		scaleY = (bounds.H + signed) / bounds.H
	}
	scale := 1.0
	switch {
	case (left || right) && (top || bottom):
		if math.Abs(scaleX-1) >= math.Abs(scaleY-1) {
			scale = scaleX
		} else {
			scale = scaleY
		}
	case left || right:
		scale = scaleX
	case top || bottom:
		scale = scaleY
	default:
		return original
	}
	minimumScale := math.Max(presentationMinimumObjectSizePT/bounds.W, presentationMinimumObjectSizePT/bounds.H)
	scale = math.Max(minimumScale, scale)
	newWidth, newHeight := bounds.W*scale, bounds.H*scale
	if left {
		bounds.X += bounds.W - newWidth
	} else if !right {
		bounds.X += (bounds.W - newWidth) / 2
	}
	if top {
		bounds.Y += bounds.H - newHeight
	} else if !bottom {
		bounds.Y += (bounds.H - newHeight) / 2
	}
	bounds.W, bounds.H = newWidth, newHeight
	original.Bounds = bounds
	return original
}

func presentationResizeHandleSides(handle presentationTransformHandle) (left, right, top, bottom, ok bool) {
	switch handle {
	case presentationHandleTopLeft:
		return true, false, true, false, true
	case presentationHandleTop:
		return false, false, true, false, true
	case presentationHandleTopRight:
		return false, true, true, false, true
	case presentationHandleRight:
		return false, true, false, false, true
	case presentationHandleBottomRight:
		return false, true, false, true, true
	case presentationHandleBottom:
		return false, false, false, true, true
	case presentationHandleBottomLeft:
		return true, false, false, true, true
	case presentationHandleLeft:
		return true, false, false, false, true
	default:
		return false, false, false, false, false
	}
}

// presentationRotatePreview applies the clockwise angular difference between
// the initial and current pointer positions around the object center.
func presentationRotatePreview(original presentationTransform, press, current, canvasOrigin f32.Point, canvasScale float64) presentationTransform {
	if canvasScale <= 0 {
		return original
	}
	start := presentationDevicePointToSlide(press, canvasOrigin, canvasScale)
	end := presentationDevicePointToSlide(current, canvasOrigin, canvasScale)
	return presentationRotatePreviewPoints(original, start, end)
}

func presentationRotatePreviewPoints(original presentationTransform, start, end document.PresentationPoint) presentationTransform {
	center := document.PresentationPoint{X: original.Bounds.X + original.Bounds.W/2, Y: original.Bounds.Y + original.Bounds.H/2}
	startAngle := math.Atan2(start.Y-center.Y, start.X-center.X) * 180 / math.Pi
	endAngle := math.Atan2(end.Y-center.Y, end.X-center.X) * 180 / math.Pi
	delta := endAngle - startAngle
	for delta > 180 {
		delta -= 360
	}
	for delta <= -180 {
		delta += 360
	}
	original.RotationDeg = presentationNormalizeAngle(original.RotationDeg + delta)
	return original
}

func presentationNormalizeAngle(angle float64) float64 {
	angle = math.Mod(angle, 360)
	if angle < 0 {
		angle += 360
	}
	if angle == 360 || angle == 0 {
		return 0
	}
	return angle
}

// pushPresentationElementRotation applies the same local-space rotation to
// object content, text, and canvas selection chrome in every presentation
// preview surface.
func pushPresentationElementRotation(gtx layout.Context, bounds image.Rectangle, rotationDeg float64) op.TransformStack {
	center := f32.Pt(float32(bounds.Min.X+bounds.Max.X)/2, float32(bounds.Min.Y+bounds.Max.Y)/2)
	angle := float32(presentationNormalizeAngle(rotationDeg) * math.Pi / 180)
	return op.Affine(f32.AffineId().Rotate(center, angle)).Push(gtx.Ops)
}
