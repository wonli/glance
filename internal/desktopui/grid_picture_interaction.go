package desktopui

import (
	"fmt"
	"image"
	"math"

	"gioui.org/f32"
	"glance/internal/document"
)

const (
	gridPicturePlaceholderWidth  = 128
	gridPicturePlaceholderHeight = 96
)

type gridPictureGeometry struct {
	X, Y, Width, Height float64
}

func gridPictureContentGeometry(v *gridViewport, picture document.SpreadsheetImage) gridPictureGeometry {
	if v == nil || v.Scale <= 0 || picture.Row < 1 || picture.Column < 1 {
		return gridPictureGeometry{}
	}
	x, errX := v.Columns.Boundary(picture.Column - 1)
	y, errY := v.Rows.Boundary(picture.Row - 1)
	if errX != nil || errY != nil {
		return gridPictureGeometry{}
	}
	width, height := picture.WidthPx, picture.HeightPx
	if width < 1 {
		width = gridPicturePlaceholderWidth
	}
	if height < 1 {
		height = gridPicturePlaceholderHeight
	}
	return gridPictureGeometry{
		X:      x + float64(picture.OffsetX)*v.Scale,
		Y:      y + float64(picture.OffsetY)*v.Scale,
		Width:  float64(width) * v.Scale,
		Height: float64(height) * v.Scale,
	}
}

func gridPictureScreenRect(v *gridViewport, geometry gridPictureGeometry, rowHeader, columnHeader int) image.Rectangle {
	if v == nil || geometry.Width <= 0 || geometry.Height <= 0 {
		return image.Rectangle{}
	}
	columnSplit, rowSplit := v.paneSplits()
	left := int(math.Round(geometry.X + float64(rowHeader)))
	right := int(math.Round(geometry.X + geometry.Width + float64(rowHeader)))
	top := int(math.Round(geometry.Y + float64(columnHeader)))
	bottom := int(math.Round(geometry.Y + geometry.Height + float64(columnHeader)))
	if columnSplit == 0 || geometry.X >= v.frozenColumnBoundary() {
		left -= int(math.Round(v.X))
		right -= int(math.Round(v.X))
	}
	if rowSplit == 0 || geometry.Y >= v.frozenRowBoundary() {
		top -= int(math.Round(v.Y))
		bottom -= int(math.Round(v.Y))
	}
	if right <= left {
		right = left + 1
	}
	if bottom <= top {
		bottom = top + 1
	}
	return image.Rect(left, top, right, bottom)
}

func gridPictureScreenRectForCell(v *gridViewport, geometry gridPictureGeometry, row, column, rowHeader, columnHeader int) image.Rectangle {
	if v == nil || geometry.Width <= 0 || geometry.Height <= 0 {
		return image.Rectangle{}
	}
	columnSplit, rowSplit := v.paneSplits()
	frozenColumn := columnSplit > 0 && column <= columnSplit
	frozenRow := rowSplit > 0 && row <= rowSplit
	left := geometry.X + float64(rowHeader)
	right := geometry.X + geometry.Width + float64(rowHeader)
	top := geometry.Y + float64(columnHeader)
	bottom := geometry.Y + geometry.Height + float64(columnHeader)
	if !frozenColumn {
		left -= v.X
		right -= v.X
	}
	if !frozenRow {
		top -= v.Y
		bottom -= v.Y
	}
	leftPixel, topPixel := int(math.Round(left)), int(math.Round(top))
	rightPixel, bottomPixel := int(math.Round(right)), int(math.Round(bottom))
	if rightPixel <= leftPixel {
		rightPixel = leftPixel + 1
	}
	if bottomPixel <= topPixel {
		bottomPixel = topPixel + 1
	}
	return image.Rect(leftPixel, topPixel, rightPixel, bottomPixel)
}

func gridPicturePaneClip(v *gridViewport, row, column int, size image.Point, rowHeader, columnHeader int) image.Rectangle {
	viewport := image.Rect(rowHeader, columnHeader, size.X, size.Y)
	if v == nil {
		return viewport
	}
	columnSplit, rowSplit := v.paneSplits()
	frozenWidth, frozenHeight := v.frozenViewportExtent(float64(viewport.Dx()), float64(viewport.Dy()))
	if columnSplit > 0 {
		if column <= columnSplit {
			viewport.Max.X = rowHeader + int(frozenWidth)
		} else {
			viewport.Min.X = rowHeader + int(frozenWidth)
		}
	}
	if rowSplit > 0 {
		if row <= rowSplit {
			viewport.Max.Y = columnHeader + int(frozenHeight)
		} else {
			viewport.Min.Y = columnHeader + int(frozenHeight)
		}
	}
	return viewport
}

func gridPictureContentPointer(v *gridViewport, position f32.Point, rowHeader, columnHeader int, windowSize ...image.Point) f32.Point {
	if v == nil {
		return f32.Point{}
	}
	x, y := float64(position.X)-float64(rowHeader), float64(position.Y)-float64(columnHeader)
	columnSplit, rowSplit := v.paneSplits()
	width, height := float64(rowHeader)+float64(position.X), float64(columnHeader)+float64(position.Y)
	if len(windowSize) > 0 {
		width = float64(windowSize[0].X - rowHeader)
		height = float64(windowSize[0].Y - columnHeader)
	}
	frozenWidth, frozenHeight := v.frozenViewportExtent(width, height)
	if columnSplit == 0 || x >= frozenWidth {
		x = v.frozenColumnBoundary() + v.X + x - frozenWidth
	}
	if rowSplit == 0 || y >= frozenHeight {
		y = v.frozenRowBoundary() + v.Y + y - frozenHeight
	}
	return f32.Pt(float32(x), float32(y))
}

func gridPictureClipRect(rect, viewport image.Rectangle) image.Rectangle {
	return rect.Intersect(viewport)
}

type gridPictureHit struct {
	ImageID      string
	ResizeHandle bool
}

func gridPictureHitAt(v *gridViewport, pictures []document.SpreadsheetImage, selectedID string, point image.Point, viewport image.Rectangle, rowHeader, columnHeader, handleSize int) gridPictureHit {
	if !point.In(viewport) {
		return gridPictureHit{}
	}
	for _, picture := range pictures {
		if picture.ID != selectedID || !picture.Editable {
			continue
		}
		paneClip := gridPicturePaneClip(v, picture.Row, picture.Column, viewport.Max, rowHeader, columnHeader)
		bounds := gridPictureClipRect(gridPictureScreenRectForCell(v, gridPictureContentGeometry(v, picture), picture.Row, picture.Column, rowHeader, columnHeader), paneClip)
		if bounds.Empty() {
			continue
		}
		if point.In(gridPictureClipRect(gridPictureHandleRect(bounds, handleSize), paneClip)) {
			return gridPictureHit{ImageID: picture.ID, ResizeHandle: true}
		}
		break
	}
	for index := len(pictures) - 1; index >= 0; index-- {
		picture := pictures[index]
		paneClip := gridPicturePaneClip(v, picture.Row, picture.Column, viewport.Max, rowHeader, columnHeader)
		bounds := gridPictureClipRect(gridPictureScreenRectForCell(v, gridPictureContentGeometry(v, picture), picture.Row, picture.Column, rowHeader, columnHeader), paneClip)
		if !bounds.Empty() && point.In(bounds) {
			return gridPictureHit{ImageID: picture.ID}
		}
	}
	return gridPictureHit{}
}

func gridPictureHandleRect(bounds image.Rectangle, size int) image.Rectangle {
	if bounds.Empty() || size <= 0 {
		return image.Rectangle{}
	}
	half := size / 2
	return image.Rect(bounds.Max.X-half, bounds.Max.Y-half, bounds.Max.X-half+size, bounds.Max.Y-half+size)
}

type gridPictureGestureMode uint8

const (
	gridPictureGestureNone gridPictureGestureMode = iota
	gridPictureGestureMove
	gridPictureGestureResize
)

type gridPictureGesture struct {
	Mode         gridPictureGestureMode
	ImageID      string
	Original     gridPictureGeometry
	Base         gridPictureGeometry
	Preview      gridPictureGeometry
	StartPointer f32.Point
}

func (gesture *gridPictureGesture) begin(id string, mode gridPictureGestureMode, geometry gridPictureGeometry, pointer f32.Point) bool {
	gesture.cancel()
	if id == "" || (mode != gridPictureGestureMove && mode != gridPictureGestureResize) || geometry.Width <= 0 || geometry.Height <= 0 {
		return false
	}
	gesture.Mode = mode
	gesture.ImageID = id
	gesture.Original = geometry
	gesture.Base = geometry
	gesture.Preview = geometry
	gesture.StartPointer = pointer
	return true
}

func (gesture *gridPictureGesture) update(pointer f32.Point) {
	if gesture == nil || gesture.Mode == gridPictureGestureNone {
		return
	}
	dx, dy := float64(pointer.X-gesture.StartPointer.X), float64(pointer.Y-gesture.StartPointer.Y)
	switch gesture.Mode {
	case gridPictureGestureMove:
		gesture.Preview.X = gesture.Base.X + dx
		gesture.Preview.Y = gesture.Base.Y + dy
	case gridPictureGestureResize:
		width := math.Max(1, gesture.Base.Width+dx)
		ratio := gesture.Base.Height / gesture.Base.Width
		gesture.Preview.Width = width
		gesture.Preview.Height = math.Max(1, width*ratio)
	}
}

// rebase establishes a new pointer origin after viewport scroll changes while
// retaining the current preview geometry.
func (gesture *gridPictureGesture) rebase(pointer f32.Point) {
	if gesture == nil || gesture.Mode == gridPictureGestureNone {
		return
	}
	gesture.Base = gesture.Preview
	gesture.StartPointer = pointer
}

func (gesture *gridPictureGesture) finish() (gridPictureGeometry, bool) {
	if gesture == nil || gesture.Mode == gridPictureGestureNone {
		return gridPictureGeometry{}, false
	}
	preview := gesture.Preview
	changed := math.Abs(preview.X-gesture.Original.X) > 0.01 || math.Abs(preview.Y-gesture.Original.Y) > 0.01 || math.Abs(preview.Width-gesture.Original.Width) > 0.01 || math.Abs(preview.Height-gesture.Original.Height) > 0.01
	gesture.cancel()
	return preview, changed
}

func (gesture *gridPictureGesture) cancel() {
	if gesture == nil {
		return
	}
	*gesture = gridPictureGesture{}
}

type gridPicturePlacementTarget struct {
	Sheet       string
	Row, Column int
	OffsetX     int
	OffsetY     int
	WidthPx     int
	HeightPx    int
}

func gridPicturePlacementAt(v *gridViewport, picture document.SpreadsheetImage, geometry gridPictureGeometry) (gridPicturePlacementTarget, error) {
	if v == nil || !finite(v.Scale) || v.Scale <= 0 || geometry.X < 0 || geometry.Y < 0 || geometry.Width <= 0 || geometry.Height <= 0 {
		return gridPicturePlacementTarget{}, fmt.Errorf("图片位置或尺寸无效")
	}
	columnIndex, rowIndex := v.Columns.IndexAt(geometry.X), v.Rows.IndexAt(geometry.Y)
	if rowIndex < 0 || columnIndex < 0 {
		return gridPicturePlacementTarget{}, fmt.Errorf("图片位置超出工作表范围")
	}
	columnEdge, errColumn := v.Columns.Boundary(columnIndex)
	rowEdge, errRow := v.Rows.Boundary(rowIndex)
	if errColumn != nil || errRow != nil {
		return gridPicturePlacementTarget{}, fmt.Errorf("计算图片锚点失败")
	}
	target := gridPicturePlacementTarget{
		Sheet:    picture.Sheet,
		Row:      rowIndex + 1,
		Column:   columnIndex + 1,
		OffsetX:  max(0, int(math.Round((geometry.X-columnEdge)/v.Scale))),
		OffsetY:  max(0, int(math.Round((geometry.Y-rowEdge)/v.Scale))),
		WidthPx:  max(1, int(math.Round(geometry.Width/v.Scale))),
		HeightPx: max(1, int(math.Round(geometry.Height/v.Scale))),
	}
	if target.Row > xlsxRows || target.Column > xlsxColumns {
		return gridPicturePlacementTarget{}, fmt.Errorf("图片位置超出工作表范围")
	}
	return target, nil
}

func (geometry gridPictureGeometry) valid() bool {
	return finite(geometry.X) && finite(geometry.Y) && finite(geometry.Width) && finite(geometry.Height) && geometry.Width > 0 && geometry.Height > 0
}
