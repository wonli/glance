package wordlayout

import (
	"math"
	"sort"

	"glance/internal/document"
)

type floatingObstacle struct {
	pageIndex     int
	columnIndex   int
	bounds        Rect
	polygon       []document.WordImageWrapPoint
	leftDistance  float64
	rightDistance float64
}

type horizontalInterval struct {
	left  float64
	right float64
}

func validWordImageFloating(image *document.WordImage) bool {
	if image == nil || image.Floating == nil || image.WidthEMU <= 0 || image.HeightEMU <= 0 || image.RotationDegrees != 0 {
		return false
	}
	floating := image.Floating
	if floating.WrapType != "tight" || floating.WrapDistanceLeftEMU < 0 || floating.WrapDistanceRightEMU < 0 || len(floating.WrapPolygon) < 3 || len(floating.WrapPolygon) > 256 {
		return false
	}
	if !finite(emuToPoints(floating.XOffsetEMU)) || !finite(emuToPoints(floating.YOffsetEMU)) || !finite(emuToPoints(floating.WrapDistanceLeftEMU)) || !finite(emuToPoints(floating.WrapDistanceRightEMU)) {
		return false
	}
	area := 0.0
	for index, point := range floating.WrapPolygon {
		if !finite(point.X) || !finite(point.Y) || math.Abs(point.X) > 1e4 || math.Abs(point.Y) > 1e4 {
			return false
		}
		next := floating.WrapPolygon[(index+1)%len(floating.WrapPolygon)]
		area += point.X*next.Y - next.X*point.Y
	}
	return math.Abs(area) > 1e-12
}

func (c *layoutCursor) placeFloatingImages(images []preparedFloatingImage, area Rect, paragraph document.WordParagraphLayout, textTop float64) {
	if c.pageIndex < 0 || c.pageIndex >= len(c.result.Pages) {
		return
	}
	xAnchor := area.XPT + emuToPoints(paragraph.LeftIndentEMU)
	for _, image := range images {
		bounds := Rect{
			XPT: xAnchor + emuToPoints(image.geometry.XOffsetEMU), YPT: textTop + emuToPoints(image.geometry.YOffsetEMU),
			WidthPT: image.width, HeightPT: image.height,
		}
		c.result.Pages[c.pageIndex].Images = append(c.result.Pages[c.pageIndex].Images, ImagePlacement{ImageID: image.id, Bounds: bounds, RotationDegrees: image.rotation})
		c.floating = append(c.floating, floatingObstacle{
			pageIndex: c.pageIndex, columnIndex: c.currentCol, bounds: bounds,
			polygon:      append([]document.WordImageWrapPoint(nil), image.geometry.WrapPolygon...),
			leftDistance: emuToPoints(image.geometry.WrapDistanceLeftEMU), rightDistance: emuToPoints(image.geometry.WrapDistanceRightEMU),
		})
		c.used = true
	}
}

func (c *layoutCursor) paragraphLineIntervals(area Rect, paragraph document.WordParagraphLayout, firstLine bool, y, lineHeight float64) ([]horizontalInterval, float64) {
	base := paragraphBaseInterval(area, paragraph, firstLine)
	intervals := []horizontalInterval{base}
	var nearestBottom float64
	intersected := false
	for _, obstacle := range c.floating {
		if obstacle.pageIndex != c.pageIndex || obstacle.columnIndex != c.currentCol {
			continue
		}
		leftEdge, rightEdge, bottom, intersects := floatingPolygonBand(obstacle, y, y+lineHeight)
		if !intersects {
			continue
		}
		intersected = true
		leftEdge -= obstacle.leftDistance
		rightEdge += obstacle.rightDistance
		intervals = subtractHorizontalInterval(intervals, leftEdge, rightEdge)
		if bottom > y+epsilonPT && (nearestBottom == 0 || bottom < nearestBottom) {
			nearestBottom = bottom
		}
	}
	if len(c.floating) == 0 || !intersected {
		return []horizontalInterval{base}, 0
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].left < intervals[j].left })
	usable := intervals[:0]
	for _, interval := range intervals {
		if interval.right-interval.left > epsilonPT {
			usable = append(usable, interval)
		}
	}
	if len(usable) > 0 {
		return usable, nearestBottom
	}
	return nil, nearestBottom
}

func paragraphBaseInterval(area Rect, paragraph document.WordParagraphLayout, firstLine bool) horizontalInterval {
	left := emuToPoints(paragraph.LeftIndentEMU)
	right := emuToPoints(paragraph.RightIndentEMU)
	firstOffset := 0.0
	if firstLine {
		firstOffset = emuToPoints(paragraph.FirstLineIndentEMU - paragraph.HangingIndentEMU)
	}
	base := horizontalInterval{
		left:  area.XPT + left + firstOffset,
		right: area.XPT + area.WidthPT - right,
	}
	if base.right < base.left {
		base.right = base.left
	}
	return base
}

func floatingPolygonBand(obstacle floatingObstacle, top, bottom float64) (float64, float64, float64, bool) {
	shapeTop := obstacle.bounds.YPT
	shapeBottom := shapeTop + obstacle.bounds.HeightPT
	if bottom <= shapeTop+epsilonPT || top >= shapeBottom-epsilonPT || obstacle.bounds.WidthPT <= 0 || obstacle.bounds.HeightPT <= 0 {
		return 0, 0, shapeBottom, false
	}
	bandTop := math.Max(top, shapeTop)
	bandBottom := math.Min(bottom, shapeBottom)
	normalizedTop := (bandTop - shapeTop) / obstacle.bounds.HeightPT
	normalizedBottom := (bandBottom - shapeTop) / obstacle.bounds.HeightPT
	minX, maxX := math.Inf(1), math.Inf(-1)
	maxShapeY := math.Inf(-1)
	include := func(point document.WordImageWrapPoint) {
		x := obstacle.bounds.XPT + point.X*obstacle.bounds.WidthPT
		y := obstacle.bounds.YPT + point.Y*obstacle.bounds.HeightPT
		if y >= bandTop-epsilonPT && y <= bandBottom+epsilonPT {
			minX = math.Min(minX, x)
			maxX = math.Max(maxX, x)
		}
		maxShapeY = math.Max(maxShapeY, point.Y)
	}
	for index, start := range obstacle.polygon {
		end := obstacle.polygon[(index+1)%len(obstacle.polygon)]
		include(start)
		if math.Abs(end.Y-start.Y) <= epsilonPT {
			continue
		}
		for _, scanY := range []float64{normalizedTop, normalizedBottom} {
			if scanY < math.Min(start.Y, end.Y)-epsilonPT || scanY > math.Max(start.Y, end.Y)+epsilonPT {
				continue
			}
			fraction := (scanY - start.Y) / (end.Y - start.Y)
			if fraction >= -epsilonPT && fraction <= 1+epsilonPT {
				x := start.X + fraction*(end.X-start.X)
				minX = math.Min(minX, obstacle.bounds.XPT+x*obstacle.bounds.WidthPT)
				maxX = math.Max(maxX, obstacle.bounds.XPT+x*obstacle.bounds.WidthPT)
			}
		}
		if end.Y >= bandTop-epsilonPT && end.Y <= bandBottom+epsilonPT {
			x := obstacle.bounds.XPT + end.X*obstacle.bounds.WidthPT
			minX = math.Min(minX, x)
			maxX = math.Max(maxX, x)
		}
	}
	if math.IsInf(minX, 1) || math.IsInf(maxX, -1) {
		return 0, 0, shapeTop + maxShapeY*obstacle.bounds.HeightPT, false
	}
	return minX, maxX, shapeTop + maxShapeY*obstacle.bounds.HeightPT, true
}

func subtractHorizontalInterval(intervals []horizontalInterval, left, right float64) []horizontalInterval {
	if right <= left+epsilonPT {
		return intervals
	}
	result := make([]horizontalInterval, 0, len(intervals)+1)
	for _, interval := range intervals {
		if right <= interval.left+epsilonPT || left >= interval.right-epsilonPT {
			result = append(result, interval)
			continue
		}
		if left > interval.left+epsilonPT {
			result = append(result, horizontalInterval{left: interval.left, right: math.Min(left, interval.right)})
		}
		if right < interval.right-epsilonPT {
			result = append(result, horizontalInterval{left: math.Max(right, interval.left), right: interval.right})
		}
	}
	return result
}
