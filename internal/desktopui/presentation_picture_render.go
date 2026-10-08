package desktopui

import (
	"image"
	"image/draw"
	"math"

	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"glance/internal/document"
)

// drawPresentationPicture renders a cropped source into the fixed object frame.
// Callers apply object position and rotation around this local-coordinate draw.
func drawPresentationPicture(gtx layout.Context, elem document.PresentationElement, size image.Point) layout.Dimensions {
	if elem.Kind != document.ElementKindPicture || elem.Image == nil || size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{}
	}
	sourceBounds := presentationPictureSourceRect(elem.Image.Bounds(), elem.Crop)
	if sourceBounds.Empty() {
		return layout.Dimensions{}
	}
	source := presentationPictureSubImage(elem.Image, sourceBounds)
	gtx.Constraints = layout.Exact(size)
	return widget.Image{
		Src:      paint.NewImageOp(source),
		Fit:      widget.Fill,
		Position: layout.Center,
	}.Layout(gtx)
}

func presentationPictureSourceRect(bounds image.Rectangle, crop document.PresentationCrop) image.Rectangle {
	if bounds.Empty() || !presentationPictureCropValid(crop) {
		return bounds
	}
	left := int(math.Round(float64(bounds.Dx()) * float64(crop.Left) / 100000))
	top := int(math.Round(float64(bounds.Dy()) * float64(crop.Top) / 100000))
	right := int(math.Round(float64(bounds.Dx()) * float64(crop.Right) / 100000))
	bottom := int(math.Round(float64(bounds.Dy()) * float64(crop.Bottom) / 100000))
	left = min(left, bounds.Dx()-1)
	top = min(top, bounds.Dy()-1)
	if left+right >= bounds.Dx() {
		right = bounds.Dx() - left - 1
	}
	if top+bottom >= bounds.Dy() {
		bottom = bounds.Dy() - top - 1
	}
	return image.Rect(bounds.Min.X+left, bounds.Min.Y+top, bounds.Max.X-right, bounds.Max.Y-bottom)
}

func presentationPictureCropValid(crop document.PresentationCrop) bool {
	for _, value := range []int{crop.Left, crop.Top, crop.Right, crop.Bottom} {
		if value < 0 || value > 100000 {
			return false
		}
	}
	return crop.Left+crop.Right < 100000 && crop.Top+crop.Bottom < 100000
}

func presentationPictureSubImage(source image.Image, bounds image.Rectangle) image.Image {
	if subImage, ok := source.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return subImage.SubImage(bounds)
	}
	result := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(result, result.Bounds(), source, bounds.Min, draw.Src)
	return result
}
