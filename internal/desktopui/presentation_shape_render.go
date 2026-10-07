package desktopui

import (
	"image"
	"image/color"
	"math"
	"sort"
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"glance/internal/document"
)

type presentationPaintPath struct {
	path   clip.PathSpec
	fill   bool
	stroke bool
}

func presentationElementPaths(gtx layout.Context, elem document.PresentationElement, size image.Point) []presentationPaintPath {
	if len(elem.Paths) > 0 {
		paths := make([]presentationPaintPath, 0, len(elem.Paths))
		for _, source := range elem.Paths {
			path, ok := presentationCustomPath(gtx, source, size)
			if !ok {
				return presentationPresetPaths(gtx, elem.ShapeType, size)
			}
			paths = append(paths, presentationPaintPath{
				path:   path,
				fill:   !strings.EqualFold(source.Fill, "none"),
				stroke: !strings.EqualFold(source.Stroke, "none"),
			})
		}
		if len(paths) > 0 {
			return paths
		}
	}
	return presentationPresetPaths(gtx, elem.ShapeType, size)
}

func presentationPresetPaths(gtx layout.Context, shapeType string, size image.Point) []presentationPaintPath {
	bounds := image.Rectangle{Max: size}
	var path clip.PathSpec
	switch strings.ToLower(shapeType) {
	case "ellipse", "oval":
		path = clip.Ellipse(bounds).Path(gtx.Ops)
	case "roundrect", "roundrectangle":
		radius := min(size.X, size.Y) / 8
		path = clip.UniformRRect(bounds, max(1, radius)).Path(gtx.Ops)
	case "line", "lineinv":
		var line clip.Path
		line.Begin(gtx.Ops)
		if strings.EqualFold(shapeType, "lineInv") {
			line.MoveTo(f32.Pt(0, float32(size.Y)))
			line.LineTo(f32.Pt(float32(size.X), 0))
		} else {
			line.MoveTo(f32.Pt(0, 0))
			line.LineTo(f32.Pt(float32(size.X), float32(size.Y)))
		}
		path = line.End()
		return []presentationPaintPath{{path: path, stroke: true}}
	default:
		path = clip.Rect(bounds).Path()
	}
	return []presentationPaintPath{{path: path, fill: true, stroke: true}}
}

func presentationCustomPath(gtx layout.Context, source document.PresentationPath, size image.Point) (clip.PathSpec, bool) {
	if source.Width <= 0 || source.Height <= 0 || size.X <= 0 || size.Y <= 0 {
		return clip.PathSpec{}, false
	}
	scaleX := float64(size.X) / source.Width
	scaleY := float64(size.Y) / source.Height
	point := func(p document.PresentationPoint) f32.Point {
		return f32.Pt(float32(p.X*scaleX), float32(p.Y*scaleY))
	}

	hasSegment := false
	for _, command := range source.Commands {
		switch command.Kind {
		case "moveTo":
			if len(command.Points) != 1 {
				return clip.PathSpec{}, false
			}
		case "lnTo":
			if len(command.Points) != 1 {
				return clip.PathSpec{}, false
			}
			hasSegment = true
		case "cubicBezTo":
			if len(command.Points) != 3 {
				return clip.PathSpec{}, false
			}
			hasSegment = true
		case "quadBezTo":
			if len(command.Points) != 2 {
				return clip.PathSpec{}, false
			}
			hasSegment = true
		case "close":
		default:
			return clip.PathSpec{}, false
		}
	}
	if !hasSegment {
		return clip.PathSpec{}, false
	}
	var path clip.Path
	path.Begin(gtx.Ops)
	for _, command := range source.Commands {
		switch command.Kind {
		case "moveTo":
			path.MoveTo(point(command.Points[0]))
		case "lnTo":
			path.LineTo(point(command.Points[0]))
		case "cubicBezTo":
			path.CubeTo(point(command.Points[0]), point(command.Points[1]), point(command.Points[2]))
		case "quadBezTo":
			path.QuadTo(point(command.Points[0]), point(command.Points[1]))
		case "close":
			path.Close()
		}
	}
	return path.End(), true
}

func drawPresentationElementShape(gtx layout.Context, elem document.PresentationElement, size image.Point, scale float64) {
	paths := presentationElementPaths(gtx, elem, size)
	fillColor := parsePresentationHexColor(elem.FillColorHex, color.NRGBA{})
	borderColor := parsePresentationHexColor(elem.BorderColor, pptxBorderColor)
	for _, path := range paths {
		if path.fill {
			switch {
			case elem.FillGradient != nil:
				if !drawPresentationLinearGradient(gtx, path.path, size, elem.FillGradient) {
					if gradientColor, ok := presentationGradientFallbackColor(elem.FillGradient); ok {
						paint.FillShape(gtx.Ops, gradientColor, clip.Outline{Path: path.path}.Op())
					}
				}
			case fillColor.A > 0:
				paint.FillShape(gtx.Ops, fillColor, clip.Outline{Path: path.path}.Op())
			}
		}
		if path.stroke && elem.BorderColor != "" {
			width := float32(max(1.0, elem.BorderWidth*scale))
			paint.FillShape(gtx.Ops, borderColor, clip.Stroke{Path: path.path, Width: width}.Op())
		}
	}
}

func drawPresentationLinearGradient(gtx layout.Context, shape clip.PathSpec, size image.Point, gradient *document.PresentationGradient) bool {
	if gradient == nil || gradient.PathType != "" || len(gradient.Stops) < 2 || size.X <= 0 || size.Y <= 0 {
		return false
	}
	stops := presentationSortedGradientStops(gradient.Stops)
	if len(stops) < 2 {
		return false
	}
	for _, stop := range stops {
		if strings.TrimSpace(stop.ColorHex) == "" {
			return false
		}
	}
	pathClip := clip.Outline{Path: shape}.Op().Push(gtx.Ops)
	defer pathClip.Pop()

	center := f32.Pt(float32(size.X)/2, float32(size.Y)/2)
	angle := gradient.AngleDeg * math.Pi / 180
	dx, dy := math.Cos(angle), math.Sin(angle)
	length := math.Abs(dx)*float64(size.X)/2 + math.Abs(dy)*float64(size.Y)/2
	perpX, perpY := -dy, dx
	perp := math.Abs(perpX)*float64(size.X)/2 + math.Abs(perpY)*float64(size.Y)/2
	position := func(t float64) f32.Point {
		distance := (t - 0.5) * 2 * length
		return f32.Pt(center.X+float32(dx*distance), center.Y+float32(dy*distance))
	}
	band := func(from, to float64) clip.Op {
		start, end := position(from), position(to)
		var path clip.Path
		path.Begin(gtx.Ops)
		path.MoveTo(f32.Pt(start.X+float32(perpX*perp), start.Y+float32(perpY*perp)))
		path.LineTo(f32.Pt(end.X+float32(perpX*perp), end.Y+float32(perpY*perp)))
		path.LineTo(f32.Pt(end.X-float32(perpX*perp), end.Y-float32(perpY*perp)))
		path.LineTo(f32.Pt(start.X-float32(perpX*perp), start.Y-float32(perpY*perp)))
		path.Close()
		return clip.Outline{Path: path.End()}.Op()
	}

	for i := 0; i+1 < len(stops); i++ {
		from, to := stops[i], stops[i+1]
		if to.Position <= from.Position {
			continue
		}
		fromT, toT := from.Position/100000, to.Position/100000
		clipFrom, clipTo := fromT, toT
		seamOverlap := 1.25 / (2 * length)
		if i > 0 {
			clipFrom -= seamOverlap
		}
		if i+1 < len(stops)-1 {
			clipTo += seamOverlap
		}
		stripe := band(clipFrom, clipTo).Push(gtx.Ops)
		paint.LinearGradientOp{
			Stop1:  position(fromT),
			Color1: presentationGradientColor(from),
			Stop2:  position(toT),
			Color2: presentationGradientColor(to),
		}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		stripe.Pop()
	}
	return true
}

func presentationSortedGradientStops(source []document.PresentationGradientStop) []document.PresentationGradientStop {
	stops := append([]document.PresentationGradientStop(nil), source...)
	for i := range stops {
		if stops[i].Position < 0 {
			if len(stops) == 1 {
				stops[i].Position = 0
			} else {
				stops[i].Position = float64(i) * 100000 / float64(len(stops)-1)
			}
		}
		stops[i].Position = min(100000, max(0, stops[i].Position))
	}
	sort.SliceStable(stops, func(i, j int) bool { return stops[i].Position < stops[j].Position })
	if len(stops) == 0 {
		return nil
	}
	if stops[0].Position > 0 {
		stops = append([]document.PresentationGradientStop{stops[0]}, stops...)
		stops[0].Position = 0
	}
	if stops[len(stops)-1].Position < 100000 {
		last := stops[len(stops)-1]
		last.Position = 100000
		stops = append(stops, last)
	}
	return stops
}

func presentationGradientColor(stop document.PresentationGradientStop) color.NRGBA {
	col := parsePresentationHexColor(stop.ColorHex, color.NRGBA{})
	alpha := stop.Alpha
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	col.A = uint8(math.Round(alpha * 255))
	return col
}

func presentationGradientFallbackColor(gradient *document.PresentationGradient) (color.NRGBA, bool) {
	if gradient == nil || len(gradient.Stops) == 0 {
		return color.NRGBA{}, false
	}
	stops := presentationSortedGradientStops(gradient.Stops)
	for _, stop := range stops {
		if strings.TrimSpace(stop.ColorHex) != "" {
			return presentationGradientColor(stop), true
		}
	}
	return color.NRGBA{}, false
}
