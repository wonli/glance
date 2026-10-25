package desktopui

import (
	"image"
	"image/color"
	"math"
	"sort"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

type proofreadingCircuitRoute struct {
	points []image.Point
}
type proofreadingCircuitCache struct {
	gutter image.Rectangle
	pitch  int
	pairs  [][2]image.Point
	routes []proofreadingCircuitRoute
}

// Allocate rectilinear tracks in the inter-page gutter. Long routes are placed
// first; the lane search penalizes crossings and shared tracks before length.
// Reversed endpoint order can force a crossing; wires intersect naturally.
func proofreadingCircuitRoutes(connections []proofreadingOCRConnection, gutter image.Rectangle, pitch int) []proofreadingCircuitRoute {
	routes := make([]proofreadingCircuitRoute, len(connections))
	if len(connections) == 0 || gutter.Dx() < 1 {
		return routes
	}
	pitch = max(1, pitch)
	count := max(2, min(80, gutter.Dx()/pitch+1))
	var lanes []int
	for i := 0; i < count; i++ {
		lanes = append(lanes, gutter.Min.X+(gutter.Dx()*i)/(count-1))
	}
	order := make([]int, len(connections))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := connections[order[i]], connections[order[j]]
		da, db := proofreadingCircuitAbs(a.to.Y-a.from.Y), proofreadingCircuitAbs(b.to.Y-b.from.Y)
		if da != db {
			return da > db
		}
		if a.from.Y != b.from.Y {
			return a.from.Y < b.from.Y
		}
		return a.to.Y < b.to.Y
	})
	var placed []int
	for _, index := range order {
		c := connections[index]
		bestScore := int64(math.MaxInt64)
		var best []image.Point
		for laneIndex := range lanes {
			lane := lanes[laneIndex]
			if c.to.Y > c.from.Y {
				lane = lanes[len(lanes)-laneIndex-1]
			}
			candidate := simplifyProofreadingCircuit([]image.Point{c.from, image.Pt(lane, c.from.Y), image.Pt(lane, c.to.Y), c.to})
			score := int64(proofreadingCircuitLength(candidate))
			for _, prior := range placed {
				for i := 1; i < len(candidate); i++ {
					for j := 1; j < len(routes[prior].points); j++ {
						_, cross, overlap := proofreadingCircuitIntersection(candidate[i-1], candidate[i], routes[prior].points[j-1], routes[prior].points[j])
						if cross {
							score += 1_000_000
						}
						if overlap > 0 && c.from != connections[prior].from && c.to != connections[prior].to {
							score += 10_000_000 + int64(overlap)*100_000
						}
					}
				}
			}
			if score < bestScore {
				bestScore, best = score, candidate
			}
		}
		routes[index].points = best
		placed = append(placed, index)
	}
	return routes
}

func proofreadingCircuitAbs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
func proofreadingCircuitLength(points []image.Point) int {
	n := 0
	for i := 1; i < len(points); i++ {
		n += proofreadingCircuitAbs(points[i].X-points[i-1].X) + proofreadingCircuitAbs(points[i].Y-points[i-1].Y)
	}
	return n
}
func simplifyProofreadingCircuit(points []image.Point) []image.Point {
	result := make([]image.Point, 0, len(points))
	for _, point := range points {
		if len(result) > 0 && point == result[len(result)-1] {
			continue
		}
		for len(result) > 1 {
			a, b := result[len(result)-2], result[len(result)-1]
			if (a.X == b.X && b.X == point.X) || (a.Y == b.Y && b.Y == point.Y) {
				result = result[:len(result)-1]
			} else {
				break
			}
		}
		result = append(result, point)
	}
	return result
}
func proofreadingCircuitIntersection(a, b, c, d image.Point) (image.Point, bool, int) {
	ah, ch := a.Y == b.Y, c.Y == d.Y
	if ah == ch {
		if ah && a.Y == c.Y {
			return image.Point{}, false, max(0, min(max(a.X, b.X), max(c.X, d.X))-max(min(a.X, b.X), min(c.X, d.X)))
		}
		if !ah && a.X == c.X {
			return image.Point{}, false, max(0, min(max(a.Y, b.Y), max(c.Y, d.Y))-max(min(a.Y, b.Y), min(c.Y, d.Y)))
		}
		return image.Point{}, false, 0
	}
	if !ah {
		a, b, c, d = c, d, a, b
	}
	point := image.Pt(c.X, a.Y)
	cross := point.X > min(a.X, b.X) && point.X < max(a.X, b.X) && point.Y > min(c.Y, d.Y) && point.Y < max(c.Y, d.Y)
	return point, cross, 0
}

func (s *proofreadingOCRStepState) circuitRoutes(connections []proofreadingOCRConnection, gutter image.Rectangle, pitch int) []proofreadingCircuitRoute {
	cache := &s.circuitCache
	same := cache.gutter == gutter && cache.pitch == pitch && len(cache.pairs) == len(connections)
	if same {
		for i, c := range connections {
			if cache.pairs[i] != ([2]image.Point{c.from, c.to}) {
				same = false
				break
			}
		}
	}
	if same {
		return cache.routes
	}
	cache.gutter, cache.pitch = gutter, pitch
	cache.pairs = make([][2]image.Point, len(connections))
	for i, c := range connections {
		cache.pairs[i] = [2]image.Point{c.from, c.to}
	}
	cache.routes = proofreadingCircuitRoutes(connections, gutter, pitch)
	return cache.routes
}

func proofreadingCircuitPath(gtx layout.Context, points []image.Point) clip.PathSpec {
	var path clip.Path
	path.Begin(gtx.Ops)
	for i, point := range points {
		p := f32.Pt(float32(point.X), float32(point.Y))
		if i == 0 {
			path.MoveTo(p)
		} else {
			path.LineTo(p)
		}
	}
	return path.End()
}
func drawProofreadingCircuitRoute(gtx layout.Context, route proofreadingCircuitRoute, startInk, endInk color.NRGBA, active bool) {
	if len(route.points) < 2 {
		return
	}
	spec := proofreadingCircuitPath(gtx, route.points)
	width := float32(max(1, gtx.Dp(1.5)))
	if active {
		width = float32(max(2, gtx.Dp(2.5)))
	}
	shape := clip.Stroke{Path: spec, Width: width}.Op().Push(gtx.Ops)
	first, last := route.points[0], route.points[len(route.points)-1]
	paint.LinearGradientOp{Stop1: f32.Pt(float32(first.X), float32(first.Y)), Color1: startInk, Stop2: f32.Pt(float32(last.X), float32(last.Y)), Color2: endInk}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	shape.Pop()
}
func proofreadingCircuitPoint(points []image.Point, progress float64) image.Point {
	remaining := float64(proofreadingCircuitLength(points)) * max(0, min(1, progress))
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		length := float64(proofreadingCircuitAbs(b.X-a.X) + proofreadingCircuitAbs(b.Y-a.Y))
		if remaining <= length && length > 0 {
			return image.Pt(int(math.Round(float64(a.X)+float64(b.X-a.X)*remaining/length)), int(math.Round(float64(a.Y)+float64(b.Y-a.Y)*remaining/length)))
		}
		remaining -= length
	}
	if len(points) > 0 {
		return points[len(points)-1]
	}
	return image.Point{}
}
