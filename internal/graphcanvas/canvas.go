package graphcanvas

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type LODLevel int

const (
	LODOverview LODLevel = iota
	LODFar
	LODMedium
	LODDetail
)

// Canvas represents the infinite graph visualization widget.
type Canvas struct {
	Graph   *Graph
	Grid    *SpatialGrid
	Camera  *Camera
	Physics *ActiveSubgraph

	// Interaction state
	eventTag        int
	draggingCanvas  bool
	draggingNode    bool
	draggedNodeID   int32
	hoveredNodeID   int32
	lastPointerPos  image.Point
	pressPos        image.Point
	hasDragged      bool
	selectedNodeID  int32
	selectStartTime time.Time

	// LOD tracking
	currentLOD LODLevel
	chunkLOD   []LODLevel
	Overview   *OverviewMap

	// Theme for HUD
	theme *material.Theme

	// Metrics
	fps                float32
	lastFrameTime      time.Time
	lastRenderDuration time.Duration
	visibleChunkCount  int
	visibleNodeCount   int

	// Control buttons
	btnResetView   widget.Clickable
	btnRecluster   widget.Clickable
	btnToggleEdges widget.Clickable
	showEdges      bool
	BgColor        color.NRGBA
}

// NewCanvas creates a new initialized Canvas widget.
func NewCanvas(g *Graph) *Canvas {
	sg := NewSpatialGrid(g, 32, 32)
	overview := BuildOverviewMap(g, sg.MinX, sg.MinY, sg.MaxX, sg.MaxY, 2048, 2048)
	c := &Canvas{
		Graph:         g,
		Grid:          sg,
		Overview:      overview,
		Camera:        NewCamera(),
		Physics:       NewActiveSubgraph(),
		currentLOD:    LODOverview,
		chunkLOD:      make([]LODLevel, len(sg.Chunks)),
		theme:         material.NewTheme(),
		hoveredNodeID:  -1,
		draggedNodeID:  -1,
		selectedNodeID: -1,
		showEdges:      true,
		BgColor:        color.NRGBA{R: 0x0B, G: 0x0D, B: 0x13, A: 0xFF},
	}
	for i := range c.chunkLOD {
		c.chunkLOD[i] = -1 // force initial recording
	}
	return c
}

// Layout renders the canvas and handles all pointer interactions.
func (c *Canvas) Layout(gtx layout.Context) layout.Dimensions {
	renderStart := time.Now()

	// Update FPS metric
	if !c.lastFrameTime.IsZero() {
		dt := float32(renderStart.Sub(c.lastFrameTime).Seconds())
		if dt > 0.001 {
			instFPS := 1.0 / dt
			if c.fps == 0 {
				c.fps = instFPS
			} else {
				c.fps = c.fps*0.92 + instFPS*0.08
			}
		}
	}
	c.lastFrameTime = renderStart

	size := gtx.Constraints.Max

	// 1. Process Pointer Events
	c.processEvents(gtx, size)

	// 2. Advance Physics if active
	if c.Physics.Active {
		active := c.Physics.Step(c.Graph, 0.016)
		if active || c.draggingNode || c.Physics.Breathing {
			gtx.Execute(op.InvalidateCmd{})
		} else {
			// Physics cooled down: update overview bitmap & commit back
			if c.Overview != nil {
				for i, nid := range c.Physics.NodeIDs {
					c.Overview.UpdateSingleNode(c.Graph.X[nid], c.Graph.Y[nid], c.Physics.PosX[i], c.Physics.PosY[i], c.Graph.Color[nid])
				}
			}
			c.Physics.Commit(c.Graph, c.Grid)
			gtx.Execute(op.InvalidateCmd{})
		}
	} else if c.selectedNodeID >= 0 {
		gtx.Execute(op.InvalidateCmd{})
	}

	// 3. Register Pointer Hit Area
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &c.eventTag)
	area.Pop()

	// 4. Fill background (Unified seamless deep-black void)
	bgColor := c.BgColor
	if bgColor.A == 0 {
		bgColor = color.NRGBA{R: 0x0B, G: 0x0D, B: 0x13, A: 0xFF}
	}
	paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: size}.Op())

	// 5. Determine Current LOD based on Zoom
	z := c.Camera.Zoom
	var targetLOD LODLevel
	if z < 0.25 {
		targetLOD = LODOverview
	} else if z < 0.60 {
		targetLOD = LODFar
	} else if z < 1.40 {
		targetLOD = LODMedium
	} else {
		targetLOD = LODDetail
	}
	c.currentLOD = targetLOD

	// 6. Query visible chunks
	margin := float32(100.0)
	vx0, vy0, vx1, vy1 := c.Camera.ViewportWorldBounds(size, margin)
	visibleChunks := c.Grid.QueryVisibleChunks(vx0, vy0, vx1, vy1)
	c.visibleChunkCount = len(visibleChunks)

	nodeCount := 0
	for _, ch := range visibleChunks {
		n := len(ch.NodeIndices)
		switch c.currentLOD {
		case LODOverview:
			n = 0
		case LODFar:
			if n > 8 {
				n = 8
			}
		case LODMedium:
			if n > 22 {
				n = 22
			}
		case LODDetail:
			// all nodes
		}
		nodeCount += n
	}
	if c.currentLOD == LODOverview {
		nodeCount = len(c.Graph.HubIDs)
	}
	c.visibleNodeCount = nodeCount

	// 7. Render Static & Dynamic Layers under Camera Affine Transform
	halfW := float32(size.X) * 0.5
	halfH := float32(size.Y) * 0.5

	aff := f32.NewAffine2D(
		c.Camera.Zoom, 0, halfW+c.Camera.OffsetX,
		0, c.Camera.Zoom, halfH+c.Camera.OffsetY,
	)

	trans := op.Affine(aff).Push(gtx.Ops)

	// Layer 1: Static Layer (Hybrid: Bitmap Overview at Low LOD, Vector Chunks at High LOD)
	if c.currentLOD == LODOverview && c.Overview != nil {
		c.Overview.Render(gtx.Ops)

		// Draw only prominent Hub Nodes (150 nodes) as crisp vector circles
		for _, hid := range c.Graph.HubIDs {
			if c.Physics.Active {
				if _, isActive := c.Physics.NodeIndex[hid]; isActive {
					continue
				}
			}
			hx := c.Graph.X[hid]
			hy := c.Graph.Y[hid]
			hr := c.Graph.Radius[hid]
			col := c.Graph.Color[hid]
			rect := image.Rect(int(hx-hr), int(hy-hr), int(hx+hr+0.5), int(hy+hr+0.5))
			paint.FillShape(gtx.Ops, col, clip.Ellipse(rect).Op(gtx.Ops))
		}
	} else {
		c.renderStaticLayer(gtx.Ops, visibleChunks)
	}

	// Layer 2: Dynamic Layer (Active Set, Dragged Node, Active Edges)
	if c.Physics.Active {
		c.renderDynamicLayer(gtx.Ops)
	}

	// Layer 3: Selection Breathing Animation (glowing halo + 1-hop relation links)
	if c.selectedNodeID >= 0 {
		c.renderSelectionBreath(gtx.Ops, gtx.Now)
	}

	// Layer 4: Hover Ring if hovering any non-selected node
	if c.hoveredNodeID >= 0 && c.hoveredNodeID != c.selectedNodeID {
		c.renderHoverHighlight(gtx.Ops, c.hoveredNodeID)
	}

	trans.Pop()

	// 8. Render Labels in Screen Space (Crisp, Non-scaling, Collision-free)
	c.renderLabels(gtx, size)

	// 9. Render HUD Overlay
	c.lastRenderDuration = time.Since(renderStart)
	c.renderHUD(gtx, size)

	return layout.Dimensions{Size: size}
}

func (c *Canvas) findNearestNode(wx, wy, searchRadius float32) (int32, bool) {
	var bestNode int32 = -1
	bestDistSq := searchRadius * searchRadius

	if c.Physics.Active {
		for i, nid := range c.Physics.NodeIDs {
			dx := c.Physics.PosX[i] - wx
			dy := c.Physics.PosY[i] - wy
			distSq := dx*dx + dy*dy
			effectiveRadius := c.Graph.Radius[nid] + 6.0
			if distSq <= (effectiveRadius*effectiveRadius + bestDistSq) {
				d := float32(math.Sqrt(float64(distSq)))
				if d-c.Graph.Radius[nid] < float32(math.Sqrt(float64(bestDistSq))) {
					bestDistSq = distSq
					bestNode = nid
				}
			}
		}
		if bestNode >= 0 {
			return bestNode, true
		}
	}

	return c.Grid.FindNearestNode(c.Graph, wx, wy, searchRadius)
}

func (c *Canvas) processEvents(gtx layout.Context, size image.Point) {
	filter := pointer.Filter{
		Target: &c.eventTag,
		Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Move | pointer.Scroll,
		ScrollX: pointer.ScrollRange{Min: -(1 << 20), Max: 1 << 20},
		ScrollY: pointer.ScrollRange{Min: -(1 << 20), Max: 1 << 20},
	}

	for value, ok := gtx.Event(filter); ok; value, ok = gtx.Event(filter) {
		pe, ok := value.(pointer.Event)
		if !ok {
			continue
		}

		posPt := pe.Position.Round()
		wx, wy := c.Camera.ScreenToWorld(posPt, size)

		switch pe.Kind {
		case pointer.Press:
			c.lastPointerPos = posPt
			c.pressPos = posPt
			c.hasDragged = false

			// Check if pressing on a node
			hitRadius := float32(18.0) / c.Camera.Zoom
			if hitRadius < 10.0 {
				hitRadius = 10.0
			}
			nodeID, hit := c.findNearestNode(wx, wy, hitRadius)
			if hit {
				// Hit a node: begin dragging node with local active physics
				c.draggingNode = true
				c.draggedNodeID = nodeID

				// If another node was active, commit it cleanly first
				if c.Physics.Active && c.Physics.DraggedID != nodeID {
					if c.Overview != nil {
						for i, nid := range c.Physics.NodeIDs {
							c.Overview.UpdateSingleNode(c.Graph.X[nid], c.Graph.Y[nid], c.Physics.PosX[i], c.Physics.PosY[i], c.Graph.Color[nid])
						}
					}
					c.Physics.Commit(c.Graph, c.Grid)
				}

				// Anchor initially to node's position to prevent press teleportation
				c.Physics.Activate(c.Graph, nodeID, c.Graph.X[nodeID], c.Graph.Y[nodeID])
				for _, nid := range c.Physics.NodeIDs {
					c.Grid.InvalidateNode(c.Graph, nid)
				}
				gtx.Execute(op.InvalidateCmd{})
			} else {
				// Canvas pan
				c.draggingCanvas = true
			}

		case pointer.Drag:
			pdx := posPt.X - c.pressPos.X
			pdy := posPt.Y - c.pressPos.Y
			if pdx*pdx+pdy*pdy > 16 {
				c.hasDragged = true
			}
			if c.draggingNode && c.Physics.Active {
				c.Physics.UpdateTarget(wx, wy)
				gtx.Execute(op.InvalidateCmd{})
			} else if c.draggingCanvas {
				dx := float32(posPt.X - c.lastPointerPos.X)
				dy := float32(posPt.Y - c.lastPointerPos.Y)
				c.Camera.Pan(dx, dy)
				c.lastPointerPos = posPt
				gtx.Execute(op.InvalidateCmd{})
			}

		case pointer.Move:
			// Hover detection for tooltips/highlights
			hitRadius := float32(14.0) / c.Camera.Zoom
			if hitRadius < 8.0 {
				hitRadius = 8.0
			}
			nid, hit := c.findNearestNode(wx, wy, hitRadius)
			if hit {
				if c.hoveredNodeID != nid {
					c.hoveredNodeID = nid
					gtx.Execute(op.InvalidateCmd{})
				}
			} else {
				if c.hoveredNodeID != -1 {
					c.hoveredNodeID = -1
					gtx.Execute(op.InvalidateCmd{})
				}
			}

		case pointer.Release, pointer.Cancel:
			if !c.hasDragged {
				// Pointer didn't move: this was a CLICK!
				hitRadius := float32(18.0) / c.Camera.Zoom
				if hitRadius < 10.0 {
					hitRadius = 10.0
				}
				nodeID, hit := c.findNearestNode(wx, wy, hitRadius)
				if hit {
					if c.selectedNodeID == nodeID {
						c.selectedNodeID = -1 // toggle off
						c.Physics.StopBreathing()
					} else {
						if c.Physics.Active && c.Physics.DraggedID != nodeID {
							if c.Overview != nil {
								for i, nid := range c.Physics.NodeIDs {
									c.Overview.UpdateSingleNode(c.Graph.X[nid], c.Graph.Y[nid], c.Physics.PosX[i], c.Physics.PosY[i], c.Graph.Color[nid])
								}
							}
							c.Physics.Commit(c.Graph, c.Grid)
						}
						c.selectedNodeID = nodeID
						c.selectStartTime = gtx.Now

						// Activate physical active subgraph with harmonic breathing elasticity!
						c.Physics.Activate(c.Graph, nodeID, c.Graph.X[nodeID], c.Graph.Y[nodeID])
						c.Physics.Breathing = true
						c.Physics.Alpha = 0.5
						for _, nid := range c.Physics.NodeIDs {
							c.Grid.InvalidateNode(c.Graph, nid)
						}
					}
				} else {
					c.selectedNodeID = -1 // click background -> cancel selection and breathing
					c.Physics.StopBreathing()
				}
				gtx.Execute(op.InvalidateCmd{})
			}

			if c.draggingNode {
				c.draggingNode = false
				c.Physics.Release()
				if c.selectedNodeID >= 0 && c.selectedNodeID == c.draggedNodeID {
					// Still selected: keep breathing elasticity alive after drag release
					c.Physics.Breathing = true
					c.Physics.Alpha = 0.5
				}
				gtx.Execute(op.InvalidateCmd{})
			}
			c.draggingCanvas = false

		case pointer.Scroll:
			if pe.Scroll.Y != 0 {
				zoomFactor := float32(1.0 - float64(pe.Scroll.Y)*0.002)
				c.Camera.ZoomAt(posPt, size, zoomFactor)
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}
}

func (c *Canvas) renderStaticLayer(ops *op.Ops, chunks []*Chunk) {
	edgeColor := color.NRGBA{R: 0x47, G: 0x55, B: 0x69, A: 0x66}

	for _, ch := range chunks {
		idx := ch.Row*c.Grid.Cols + ch.Col
		needsRecord := !ch.MacroValid || c.chunkLOD[idx] != c.currentLOD

		if needsRecord {
			ch.Ops.Reset()
			m := op.Record(ch.Ops)

			// 1. Draw Internal Edges
			maxEdges := len(ch.InternalEdges)
			switch c.currentLOD {
			case LODFar:
				if maxEdges > 6 {
					maxEdges = 6
				}
			case LODMedium:
				if maxEdges > 15 {
					maxEdges = 15
				}
			case LODDetail:
				// all edges
			}

			if c.showEdges && len(ch.InternalEdges) > 0 {
				var p clip.Path
				p.Begin(ch.Ops)
				edgeDrawn := 0

				for i := 0; i < maxEdges; i++ {
					e := ch.InternalEdges[i]
					u := c.Graph.EdgeSrc[e]
					v := c.Graph.EdgeDst[e]

					// If active in physics, skip in static recording
					if c.Physics.Active {
						if _, uAct := c.Physics.NodeIndex[u]; uAct {
							continue
						}
						if _, vAct := c.Physics.NodeIndex[v]; vAct {
							continue
						}
					}

					p.MoveTo(f32.Pt(c.Graph.X[u], c.Graph.Y[u]))
					p.LineTo(f32.Pt(c.Graph.X[v], c.Graph.Y[v]))
					edgeDrawn++
				}

				spec := p.End()
				if edgeDrawn > 0 {
					w := float32(1.4)
					if c.currentLOD == LODDetail {
						w = 2.2
					}
					paint.FillShape(ch.Ops, edgeColor, clip.Stroke{Path: spec, Width: w}.Op())
				}
			}

			// 2. Draw Nodes
			// Capped at ~1,000 nodes total on screen for 60-120 FPS
			maxNodes := len(ch.NodeIndices)
			switch c.currentLOD {
			case LODFar:
				if maxNodes > 8 {
					maxNodes = 8
				}
			case LODMedium:
				if maxNodes > 22 {
					maxNodes = 22
				}
			case LODDetail:
				// all nodes
			}

			for i := 0; i < maxNodes; i++ {
				nid := ch.NodeIndices[i]

				// Skip if currently moving in dynamic active set
				if c.Physics.Active {
					if _, isActive := c.Physics.NodeIndex[nid]; isActive {
						continue
					}
				}

				nx := c.Graph.X[nid]
				ny := c.Graph.Y[nid]
				nr := c.Graph.Radius[nid]
				col := c.Graph.Color[nid]

				rect := image.Rect(int(nx-nr), int(ny-nr), int(nx+nr+0.5), int(ny+nr+0.5))
				paint.FillShape(ch.Ops, col, clip.Ellipse(rect).Op(ch.Ops))
			}

			ch.Macro = m.Stop()
			ch.MacroValid = true
			c.chunkLOD[idx] = c.currentLOD
		}

		// Replay the cached macro in ops (gtx.Ops)
		ch.Macro.Add(ops)
	}
}

func (c *Canvas) renderDynamicLayer(ops *op.Ops) {
	sub := c.Physics
	activeEdgeColor := color.NRGBA{R: 0x38, G: 0xBD, B: 0xF8, A: 0x99}

	// 1. Draw Active Dynamic Edges
	if len(sub.ActiveEdges) > 0 {
		var p clip.Path
		p.Begin(ops)

		for _, edge := range sub.ActiveEdges {
			uID, vID := edge.U, edge.V

			var ux, uy, vx, vy float32
			if idxU, uAct := sub.NodeIndex[uID]; uAct {
				ux, uy = sub.PosX[idxU], sub.PosY[idxU]
			} else {
				ux, uy = c.Graph.X[uID], c.Graph.Y[uID]
			}

			if idxV, vAct := sub.NodeIndex[vID]; vAct {
				vx, vy = sub.PosX[idxV], sub.PosY[idxV]
			} else {
				vx, vy = c.Graph.X[vID], c.Graph.Y[vID]
			}

			p.MoveTo(f32.Pt(ux, uy))
			p.LineTo(f32.Pt(vx, vy))
		}
		paint.FillShape(ops, activeEdgeColor, clip.Stroke{Path: p.End(), Width: 1.4}.Op())
	}

	// 2. Draw Active Nodes
	for i, nid := range sub.NodeIDs {
		nx := sub.PosX[i]
		ny := sub.PosY[i]
		nr := c.Graph.Radius[nid]
		col := c.Graph.Color[nid]

		isDragged := (i == 0)

		// Draw halo for active nodes
		haloRadius := nr + 4.0
		if isDragged {
			haloRadius = nr + 8.0
			haloCol := color.NRGBA{R: 0xFF, G: 0xE6, B: 0x00, A: 0x88}
			hRect := image.Rect(int(nx-haloRadius), int(ny-haloRadius), int(nx+haloRadius+0.5), int(ny+haloRadius+0.5))
			paint.FillShape(ops, haloCol, clip.Ellipse(hRect).Op(ops))
		} else {
			haloCol := color.NRGBA{R: 0x38, G: 0xBD, B: 0xF8, A: uint8(float32(0x55) * sub.Alpha)}
			hRect := image.Rect(int(nx-haloRadius), int(ny-haloRadius), int(nx+haloRadius+0.5), int(ny+haloRadius+0.5))
			paint.FillShape(ops, haloCol, clip.Ellipse(hRect).Op(ops))
		}

		// Core node
		nRect := image.Rect(int(nx-nr), int(ny-nr), int(nx+nr+0.5), int(ny+nr+0.5))
		paint.FillShape(ops, col, clip.Ellipse(nRect).Op(ops))
	}
}

func (c *Canvas) renderHoverHighlight(ops *op.Ops, nid int32) {
	nx := c.Graph.X[nid]
	ny := c.Graph.Y[nid]
	if c.Physics.Active {
		if idx, ok := c.Physics.NodeIndex[nid]; ok {
			nx = c.Physics.PosX[idx]
			ny = c.Physics.PosY[idx]
		}
	}
	nr := c.Graph.Radius[nid]

	ringRadius := nr + 6.0
	rect := image.Rect(int(nx-ringRadius), int(ny-ringRadius), int(nx+ringRadius+0.5), int(ny+ringRadius+0.5))
	ringColor := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xCC}

	pathSpec := clip.Ellipse(rect).Path(ops)
	paint.FillShape(ops, ringColor, clip.Stroke{Path: pathSpec, Width: 2.0}.Op())
}

func (c *Canvas) renderSelectionBreath(ops *op.Ops, now time.Time) {
	if c.selectedNodeID < 0 || c.selectedNodeID >= int32(c.Graph.NodeCount) {
		return
	}
	nid := c.selectedNodeID

	var x, y float32
	if c.Physics.Active {
		if idx, ok := c.Physics.NodeIndex[nid]; ok {
			x = c.Physics.PosX[idx]
			y = c.Physics.PosY[idx]
		} else {
			x = c.Graph.X[nid]
			y = c.Graph.Y[nid]
		}
	} else {
		x = c.Graph.X[nid]
		y = c.Graph.Y[nid]
	}

	baseR := c.Graph.Radius[nid]
	elapsed := float32(now.Sub(c.selectStartTime).Seconds())

	// Phase 0..1 smooth breathing cycle (period ~ 2.0 seconds)
	cycle := float32(math.Sin(float64(elapsed) * 3.14)) // -1 to +1
	phase := (cycle + 1.0) * 0.5                      // 0 to 1

	// 1. Draw 1-hop relationship edges with pulsing energy glow
	if len(c.Graph.Adj[nid]) > 0 {
		var p clip.Path
		p.Begin(ops)
		for _, neighbor := range c.Graph.Adj[nid] {
			nx := c.Graph.X[neighbor]
			ny := c.Graph.Y[neighbor]
			if c.Physics.Active {
				if nidx, ok := c.Physics.NodeIndex[neighbor]; ok {
					nx = c.Physics.PosX[nidx]
					ny = c.Physics.PosY[nidx]
				}
			}
			p.MoveTo(f32.Pt(x, y))
			p.LineTo(f32.Pt(nx, ny))
		}
		pulseAlpha := uint8(0x66 + 0x66*phase)
		pulseColor := color.NRGBA{R: 0x38, G: 0xBD, B: 0xF8, A: pulseAlpha}
		paint.FillShape(ops, pulseColor, clip.Stroke{Path: p.End(), Width: 2.0}.Op())
	}

	// 2. Outer breathing aura (expands from baseR+4 to baseR+16, fades as it expands)
	outerR := baseR + 4.0 + phase*12.0
	outerAlpha := uint8(0x10 + (1.0-phase)*0x45)
	outerColor := color.NRGBA{R: 0x38, G: 0xBD, B: 0xF8, A: outerAlpha}
	outerRect := image.Rect(int(x-outerR), int(y-outerR), int(x+outerR+0.5), int(y+outerR+0.5))
	paint.FillShape(ops, outerColor, clip.Ellipse(outerRect).Op(ops))

	// 3. Middle resonant wave (tighter, brighter neon cyan)
	midR := baseR + 2.0 + phase*5.0
	midAlpha := uint8(0x25 + (1.0-phase)*0x55)
	midColor := color.NRGBA{R: 0x00, G: 0xE5, B: 0xFF, A: midAlpha}
	midRect := image.Rect(int(x-midR), int(y-midR), int(x+midR+0.5), int(y+midR+0.5))
	paint.FillShape(ops, midColor, clip.Ellipse(midRect).Op(ops))

	// 4. Inner core anchor ring (sharp white outline hugging node)
	coreR := baseR + 2.5
	coreColor := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xEE}
	coreRect := image.Rect(int(x-coreR), int(y-coreR), int(x+coreR+0.5), int(y+coreR+0.5))
	paint.FillShape(ops, coreColor, clip.Stroke{Path: clip.Ellipse(coreRect).Path(ops), Width: 1.5}.Op())
}

func (c *Canvas) renderLabels(gtx layout.Context, size image.Point) {
	labelBg := color.NRGBA{R: 0x14, G: 0x17, B: 0x22, A: 0xEE}
	labelBorder := color.NRGBA{R: 0x3B, G: 0x47, B: 0x58, A: 0xFF}
	labelTextColor := color.NRGBA{R: 0xF1, G: 0xF5, B: 0xF9, A: 0xFF}

	placedRects := make([]image.Rectangle, 0, 48)

	// 1. Reserve HUD card area in top-left to avoid drawing labels behind or over HUD
	placedRects = append(placedRects, image.Rect(0, 0, 430, 260))

	collidesWithPlaced := func(r image.Rectangle) bool {
		for _, p := range placedRects {
			if r.Overlaps(p) {
				return true
			}
		}
		return false
	}

	drawPillLabel := func(sx, sy int, text string, highlight bool, isPriority bool) bool {
		lbl := material.Label(c.theme, unit.Sp(10.5), text)
		lbl.Color = labelTextColor
		if highlight {
			lbl.Font.Weight = 700
		}

		// Reset Min constraints so label measures natural text dimensions
		cgtx := gtx
		cgtx.Constraints.Min = image.Point{}

		macro := op.Record(gtx.Ops)
		dims := lbl.Layout(cgtx)
		call := macro.Stop()

		padH := gtx.Dp(6)
		padV := gtx.Dp(3)
		pillW := dims.Size.X + padH*2
		pillH := dims.Size.Y + padV*2

		// Candidate positions: 1. Right, 2. Left, 3. Top, 4. Bottom
		type candidatePos struct {
			x, y int
		}
		candidates := []candidatePos{
			{x: sx + 12, y: sy - pillH/2},
			{x: sx - pillW - 12, y: sy - pillH/2},
			{x: sx - pillW/2, y: sy - pillH - 8},
			{x: sx - pillW/2, y: sy + 10},
		}

		bestX, bestY := candidates[0].x, candidates[0].y
		foundPlacement := false

		for _, cand := range candidates {
			cx := cand.x
			cy := cand.y

			// Clamp strictly within screen boundaries
			if cx < 10 {
				cx = 10
			} else if cx > size.X-pillW-10 {
				cx = size.X - pillW - 10
			}
			if cy < 10 {
				cy = 10
			} else if cy > size.Y-pillH-10 {
				cy = size.Y - pillH - 10
			}

			// AABB with 4px safety buffer around the pill
			candRect := image.Rect(cx-4, cy-3, cx+pillW+4, cy+pillH+3)
			if !collidesWithPlaced(candRect) {
				bestX, bestY = cx, cy
				foundPlacement = true
				break
			}
		}

		// If all candidates collide, only force placement if priority (e.g. tooltip / active drag)
		if !foundPlacement {
			if !isPriority {
				return false // declutter: omit label to prevent overlapping clutter
			}
			// Priority: fallback to right position clamped
			cx := candidates[0].x
			cy := candidates[0].y
			if cx < 10 {
				cx = 10
			} else if cx > size.X-pillW-10 {
				cx = size.X - pillW - 10
			}
			if cy < 10 {
				cy = 10
			} else if cy > size.Y-pillH-10 {
				cy = size.Y - pillH - 10
			}
			bestX, bestY = cx, cy
		}

		placedRects = append(placedRects, image.Rect(bestX-2, bestY-2, bestX+pillW+2, bestY+pillH+2))

		off := op.Offset(image.Pt(bestX, bestY)).Push(gtx.Ops)

		border := labelBorder
		bg := labelBg
		if highlight {
			border = color.NRGBA{R: 0x38, G: 0xBD, B: 0xF8, A: 0xFF}
			bg = color.NRGBA{R: 0x0F, G: 0x17, B: 0x2A, A: 0xF8}
		}

		// 1. Draw rounded outer border (zero stroke artifacts)
		paint.FillShape(gtx.Ops, border, clip.UniformRRect(image.Rect(0, 0, pillW, pillH), gtx.Dp(4)).Op(gtx.Ops))

		// 2. Draw rounded inner background (inset 1px)
		paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rect(1, 1, pillW-1, pillH-1), gtx.Dp(3)).Op(gtx.Ops))

		// 3. Draw text label centered inside padding
		textOff := op.Offset(image.Pt(padH, padV)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		textOff.Pop()

		off.Pop()
		return true
	}

	// 0. Priority 0: Selected Node Breathing Focus Label (Always displayed with top priority)
	if c.selectedNodeID >= 0 {
		nid := c.selectedNodeID
		var wx, wy float32
		if c.Physics.Active {
			if idx, ok := c.Physics.NodeIndex[nid]; ok {
				wx = c.Physics.PosX[idx]
				wy = c.Physics.PosY[idx]
			} else {
				wx = c.Graph.X[nid]
				wy = c.Graph.Y[nid]
			}
		} else {
			wx = c.Graph.X[nid]
			wy = c.Graph.Y[nid]
		}
		sx, sy := c.Camera.WorldToScreen(wx, wy, size)
		label := c.Graph.Label[nid]
		if label == "" {
			label = fmt.Sprintf("知识实体 #%d", nid)
		}
		txt := fmt.Sprintf("✦ 聚焦选中: %s [关系数: %d]", label, len(c.Graph.Adj[nid]))
		drawPillLabel(int(sx), int(sy), txt, true, true)
	}

	// 1. Priority 1: Hovered Node Tooltip (High contrast, always placed first)
	if c.hoveredNodeID >= 0 && c.hoveredNodeID != c.selectedNodeID {
		nid := c.hoveredNodeID
		wx := c.Graph.X[nid]
		wy := c.Graph.Y[nid]
		if c.Physics.Active {
			if idx, ok := c.Physics.NodeIndex[nid]; ok {
				wx = c.Physics.PosX[idx]
				wy = c.Physics.PosY[idx]
			}
		}
		sx, sy := c.Camera.WorldToScreen(wx, wy, size)
		label := c.Graph.Label[nid]
		if label == "" {
			label = fmt.Sprintf("知识实体 #%d", nid)
		}
		tooltip := fmt.Sprintf("%s  [关系度数: %d]", label, len(c.Graph.Adj[nid]))
		drawPillLabel(int(sx), int(sy), tooltip, true, true)
	}

	// 2. Priority 1: Active/Dragged Node Labels (always visible)
	if c.Physics.Active {
		for i, nid := range c.Physics.NodeIDs {
			if i >= 8 {
				break
			}
			sx, sy := c.Camera.WorldToScreen(c.Physics.PosX[i], c.Physics.PosY[i], size)
			label := c.Graph.Label[nid]
			if label == "" {
				label = fmt.Sprintf("实体 #%d", nid)
			}
			txt := label
			if i == 0 {
				txt = fmt.Sprintf("★ 正在拖拽: %s", label)
			} else {
				txt = fmt.Sprintf("• 弹性联动: %s", label)
			}
			drawPillLabel(int(sx), int(sy), txt, i == 0, true)
		}
	}

	// 3. Priority 2: Hub Labels (only when zoom >= 0.20, collision-avoided)
	if c.Camera.Zoom >= 0.20 {
		labelCount := 0
		for _, hid := range c.Graph.HubIDs {
			if labelCount >= 35 {
				break
			}
			sx, sy := c.Camera.WorldToScreen(c.Graph.X[hid], c.Graph.Y[hid], size)
			if sx >= -40 && sx <= float32(size.X)+40 && sy >= -20 && sy <= float32(size.Y)+20 {
				label := c.Graph.Label[hid]
				if label == "" {
					label = fmt.Sprintf("【领域-%d】核心", c.Graph.Cluster[hid]+1)
				}
				if drawPillLabel(int(sx), int(sy), label, false, false) {
					labelCount++
				}
			}
		}
	}
}

func (c *Canvas) renderHUD(gtx layout.Context, size image.Point) {
	// Top-Left Info Panel
	layout.NW.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			cardBg := color.NRGBA{R: 0x1A, G: 0x1D, B: 0x24, A: 0xEE}
			cardBorder := color.NRGBA{R: 0x33, G: 0x3A, B: 0x48, A: 0xFF}

			return widget.Border{
				Color:        cardBorder,
				Width:        unit.Dp(1),
				CornerRadius: unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						r := clip.UniformRRect(image.Rect(0, 0, gtx.Constraints.Min.X, gtx.Constraints.Min.Y), gtx.Dp(8))
						paint.FillShape(gtx.Ops, cardBg, r.Op(gtx.Ops))
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									title := material.Label(c.theme, unit.Sp(13), "十万节点无限画布知识图谱 (Gio UI)")
									title.Color = color.NRGBA{R: 0xF3, G: 0xF4, B: 0xF6, A: 0xFF}
									title.Font.Weight = 700
									return title.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									var fpsText string
									var fpsColor color.NRGBA
									if c.Physics.Active || c.draggingNode || c.draggingCanvas || c.selectedNodeID >= 0 {
										fpsText = fmt.Sprintf("实时帧率: %.1f FPS  (渲染耗时: %.2f ms)", c.fps, float32(c.lastRenderDuration.Microseconds())/1000.0)
										if c.fps >= 50 {
											fpsColor = color.NRGBA{R: 0x34, G: 0xD3, B: 0x99, A: 0xFF} // Green
										} else {
											fpsColor = color.NRGBA{R: 0xFB, G: 0xBF, B: 0x24, A: 0xFF} // Amber
										}
									} else {
										fpsText = fmt.Sprintf("运行状态: 事件响应就绪 (按需渲染 0 CPU) | 单帧: %.2f ms", float32(c.lastRenderDuration.Microseconds())/1000.0)
										fpsColor = color.NRGBA{R: 0x38, G: 0xBD, B: 0xF8, A: 0xFF} // Sky blue
									}
									lbl := material.Label(c.theme, unit.Sp(11), fpsText)
									lbl.Color = fpsColor
									return lbl.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									stats := fmt.Sprintf("总实体节点: %d | 知识关系边数: %d", c.Graph.NodeCount, c.Graph.EdgeCount)
									lbl := material.Label(c.theme, unit.Sp(11), stats)
									lbl.Color = color.NRGBA{R: 0x9C, G: 0xA3, B: 0xAF, A: 0xFF}
									return lbl.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									lodStr := "中景星群 (22实体/分块，全屏 ~1,100)"
									switch c.currentLOD {
									case LODOverview:
										lodStr = "宏观全景 (16MB 位图烘焙 + 150 领域核心)"
									case LODFar:
										lodStr = "远景星宿 (8实体/分块，全屏 ~800)"
									case LODMedium:
										lodStr = "中景星群 (22实体/分块，全屏 ~1,100)"
									case LODDetail:
										lodStr = "特写微观 (100% 完整实体与拓扑连线)"
									}
									chText := fmt.Sprintf("当前可见块: %d / %d (视口实体: ~%d) | 呈现层级: %s",
										c.visibleChunkCount, len(c.Grid.Chunks), c.visibleNodeCount, lodStr)
									lbl := material.Label(c.theme, unit.Sp(11), chText)
									lbl.Color = color.NRGBA{R: 0x9C, G: 0xA3, B: 0xAF, A: 0xFF}
									return lbl.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									var physText string
									var physColor color.NRGBA
									if c.Physics.Active {
										physText = fmt.Sprintf("局部弹性网络: %d 实体, %d 关系弹性拉伸 (阻尼衰减: %.2f)",
											len(c.Physics.NodeIDs), len(c.Physics.ActiveEdges), c.Physics.Alpha)
										physColor = color.NRGBA{R: 0x38, G: 0xBD, B: 0xF8, A: 0xFF}
									} else if c.selectedNodeID >= 0 {
										nid := c.selectedNodeID
										label := c.Graph.Label[nid]
										if label == "" {
											label = fmt.Sprintf("知识实体 #%d", nid)
										}
										physText = fmt.Sprintf("✦ 聚焦实体: %s | 1跳关联关系: %d 条 | 持续呼吸律动中 (点击空白取消)", label, len(c.Graph.Adj[nid]))
										physColor = color.NRGBA{R: 0x00, G: 0xE5, B: 0xFF, A: 0xFF}
									} else {
										physText = "物理引擎: 静止就绪 (点击实体维持呼吸律动，拖拽触发布局弹性)"
										physColor = color.NRGBA{R: 0x6B, G: 0x72, B: 0x80, A: 0xFF}
									}
									lbl := material.Label(c.theme, unit.Sp(11), physText)
									lbl.Color = physColor
									return lbl.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									tips := "操作提示: 点击实体维持呼吸聚焦与拓扑高亮 | 拖拽实体产生弹性布局 | 滚轮缩放 | 拖动画布平移"
									lbl := material.Label(c.theme, unit.Sp(10), tips)
									lbl.Color = color.NRGBA{R: 0xD1, G: 0xD5, B: 0xDB, A: 0xFF}
									return lbl.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceStart}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if c.btnResetView.Clicked(gtx) {
												c.Camera.OffsetX = 0
												c.Camera.OffsetY = 0
												c.Camera.Zoom = 0.10
												gtx.Execute(op.InvalidateCmd{})
											}
											btn := material.Button(c.theme, &c.btnResetView, "重置视口")
											btn.TextSize = unit.Sp(11)
											btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}
											return btn.Layout(gtx)
										}),
										layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if c.btnToggleEdges.Clicked(gtx) {
												c.showEdges = !c.showEdges
												c.Grid.InvalidateAll()
												gtx.Execute(op.InvalidateCmd{})
											}
											txt := "隐藏连线"
											if !c.showEdges {
												txt = "显示连线"
											}
											btn := material.Button(c.theme, &c.btnToggleEdges, txt)
											btn.TextSize = unit.Sp(11)
											btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}
											return btn.Layout(gtx)
										}),
									)
								}),
							)
						})
					}),
				)
			})
		})
	})
}
