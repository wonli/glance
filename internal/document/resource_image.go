package document

import "image"

// ResourceImage retains logical document geometry independently of raster density.
// Its pixels are immutable after resource publication.
type ResourceImage struct {
	image.Image
	Size image.Point
}

func (i ResourceImage) LogicalSize() image.Point { return i.Size }
func ResourceLogicalSize(i image.Image) image.Point {
	if m, ok := i.(interface{ LogicalSize() image.Point }); ok {
		return m.LogicalSize()
	}
	return i.Bounds().Size()
}
