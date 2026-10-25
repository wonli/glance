package graphcanvas

import (
	"math"
)

const (
	MaxActiveNodes = 36
	SpringK        = float32(0.04)
	AnchorK        = float32(0.12)
	Damping        = float32(0.85)
)

// ActiveEdge represents a spring connection with its natural rest length.
type ActiveEdge struct {
	U, V       int32
	RestLength float32
}

// ActiveSubgraph manages the local physics simulation for dragged nodes
// and their 1-2 hop neighborhood.
type ActiveSubgraph struct {
	Active      bool
	Breathing   bool    // Continuous harmonic breathing between connected nodes
	BreathTime  float32 // Accumulated time for breathing rhythm
	Alpha       float32 // Cooling factor (1.0 -> 0.0)
	DraggedID   int32
	UserDragged bool    // True only if user actually dragged the node
	TargetX     float32 // Target cursor world X
	TargetY     float32 // Target cursor world Y

	NodeIDs    []int32
	NodeIndex  map[int32]int
	PosX       []float32
	PosY       []float32
	OrigX      []float32
	OrigY      []float32
	VelX       []float32
	VelY       []float32
	Pinned     []bool

	// ActiveEdges contains elastic spring connections
	ActiveEdges []ActiveEdge
}

// NewActiveSubgraph initializes an empty active subgraph.
func NewActiveSubgraph() *ActiveSubgraph {
	return &ActiveSubgraph{
		NodeIndex: make(map[int32]int),
	}
}

// Activate selects the dragged node and its immediate 1-hop (and select 2-hop) neighbors.
func (sub *ActiveSubgraph) Activate(g *Graph, draggedNode int32, cursorX, cursorY float32) {
	sub.Active = true
	sub.Breathing = false
	sub.UserDragged = false
	sub.Alpha = 1.0
	sub.BreathTime = 0
	sub.DraggedID = draggedNode
	sub.TargetX = g.X[draggedNode]
	sub.TargetY = g.Y[draggedNode]

	sub.NodeIDs = sub.NodeIDs[:0]
	clear(sub.NodeIndex)

	addNode := func(id int32, pinned bool) {
		if _, exists := sub.NodeIndex[id]; exists {
			return
		}
		if len(sub.NodeIDs) >= MaxActiveNodes {
			return
		}
		idx := len(sub.NodeIDs)
		sub.NodeIndex[id] = idx
		sub.NodeIDs = append(sub.NodeIDs, id)
	}

	// 1. Dragged node is first and pinned
	addNode(draggedNode, true)

	// 2. 1-hop neighbors
	neighbors := g.Adj[draggedNode]
	for _, n := range neighbors {
		addNode(n, false)
	}

	// 3. 2-hop neighbors if active set is small
	if len(sub.NodeIDs) < 80 {
		for _, n1 := range neighbors {
			for _, n2 := range g.Adj[n1] {
				if len(sub.NodeIDs) >= MaxActiveNodes {
					break
				}
				addNode(n2, false)
			}
			if len(sub.NodeIDs) >= MaxActiveNodes {
				break
			}
		}
	}

	n := len(sub.NodeIDs)
	sub.ensureCapacity(n)

	for i, id := range sub.NodeIDs {
		sub.PosX[i] = g.X[id]
		sub.PosY[i] = g.Y[id]
		sub.OrigX[i] = g.X[id]
		sub.OrigY[i] = g.Y[id]
		sub.VelX[i] = 0
		sub.VelY[i] = 0
		sub.Pinned[i] = (id == draggedNode)
	}

	// Build active edges: incident to dragged node first, capped at 45 edges
	sub.ActiveEdges = sub.ActiveEdges[:0]
	edgeSet := make(map[uint64]struct{})

	// 1. Direct edges of dragged node
	for _, neighbor := range g.Adj[draggedNode] {
		if _, isActive := sub.NodeIndex[neighbor]; isActive {
			u, v := draggedNode, neighbor
			if u > v {
				u, v = v, u
			}
			key := (uint64(u) << 32) | uint64(v)
			edgeSet[key] = struct{}{}
			dx := g.X[v] - g.X[u]
			dy := g.Y[v] - g.Y[u]
			rest := float32(math.Sqrt(float64(dx*dx + dy*dy)))
			if rest < 5.0 {
				rest = 5.0
			}
			sub.ActiveEdges = append(sub.ActiveEdges, ActiveEdge{U: u, V: v, RestLength: rest})
		}
	}

	// 2. Inter-neighbor edges (up to 45 total)
	for _, id := range sub.NodeIDs[1:] {
		if len(sub.ActiveEdges) >= 45 {
			break
		}
		for _, neighbor := range g.Adj[id] {
			if len(sub.ActiveEdges) >= 45 {
				break
			}
			if _, isActive := sub.NodeIndex[neighbor]; isActive {
				u, v := id, neighbor
				if u > v {
					u, v = v, u
				}
				key := (uint64(u) << 32) | uint64(v)
				if _, seen := edgeSet[key]; !seen {
					edgeSet[key] = struct{}{}
					dx := g.X[v] - g.X[u]
					dy := g.Y[v] - g.Y[u]
					rest := float32(math.Sqrt(float64(dx*dx + dy*dy)))
					if rest < 5.0 {
						rest = 5.0
					}
					sub.ActiveEdges = append(sub.ActiveEdges, ActiveEdge{U: u, V: v, RestLength: rest})
				}
			}
		}
	}
}

func (sub *ActiveSubgraph) ensureCapacity(n int) {
	if cap(sub.PosX) < n {
		sub.PosX = make([]float32, n)
		sub.PosY = make([]float32, n)
		sub.OrigX = make([]float32, n)
		sub.OrigY = make([]float32, n)
		sub.VelX = make([]float32, n)
		sub.VelY = make([]float32, n)
		sub.Pinned = make([]bool, n)
	} else {
		sub.PosX = sub.PosX[:n]
		sub.PosY = sub.PosY[:n]
		sub.OrigX = sub.OrigX[:n]
		sub.OrigY = sub.OrigY[:n]
		sub.VelX = sub.VelX[:n]
		sub.VelY = sub.VelY[:n]
		sub.Pinned = sub.Pinned[:n]
	}
}

// UpdateTarget sets the cursor position for the pinned dragged node.
func (sub *ActiveSubgraph) UpdateTarget(cursorX, cursorY float32) {
	sub.TargetX = cursorX
	sub.TargetY = cursorY
	sub.UserDragged = true
	sub.Alpha = 1.0 // Re-heat during drag
}

// Step advances the physics simulation by one tick.
// Returns true if simulation is still active, false if cooled to rest.
func (sub *ActiveSubgraph) Step(g *Graph, dt float32) bool {
	if !sub.Active {
		return false
	}
	if !sub.Breathing && sub.Alpha < 0.005 {
		return false
	}

	n := len(sub.NodeIDs)
	if n == 0 {
		return false
	}

	if sub.Breathing {
		sub.BreathTime += dt
		if sub.Alpha < 0.5 {
			sub.Alpha = 0.5 // Maintain gentle elasticity during breathing
		}
	}

	// Update dragged/focused node position directly
	if sub.Pinned[0] {
		sub.PosX[0] = sub.TargetX
		sub.PosY[0] = sub.TargetY
		sub.VelX[0] = 0
		sub.VelY[0] = 0
	}

	// Calculate spring forces along active edges with breathing oscillation
	for _, edge := range sub.ActiveEdges {
		uID, vID := edge.U, edge.V

		var ux, uy, vx, vy float32
		idxU, uIsActive := sub.NodeIndex[uID]
		if uIsActive {
			ux, uy = sub.PosX[idxU], sub.PosY[idxU]
		} else {
			ux, uy = g.X[uID], g.Y[uID]
		}

		idxV, vIsActive := sub.NodeIndex[vID]
		if vIsActive {
			vx, vy = sub.PosX[idxV], sub.PosY[idxV]
		} else {
			vx, vy = g.X[vID], g.Y[vID]
		}

		dx := vx - ux
		dy := vy - uy
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if dist < 1e-4 {
			continue
		}

		// Dynamic harmonic breathing rest length:
		// Nodes gently breathe with subtle organic micro-motion (max ±2.5 units)
		rest := edge.RestLength
		if sub.Breathing {
			phaseOffset := float64(edge.RestLength) * 0.008
			breath := float32(math.Sin(float64(sub.BreathTime*1.8) + phaseOffset))
			delta := edge.RestLength * 0.035 * breath
			if delta > 2.5 {
				delta = 2.5
			} else if delta < -2.5 {
				delta = -2.5
			}
			rest = edge.RestLength + delta
		}

		// Hooke's spring force:
		force := (dist - rest) * SpringK * sub.Alpha
		fx := (dx / dist) * force
		fy := (dy / dist) * force

		if uIsActive && !sub.Pinned[idxU] {
			sub.VelX[idxU] += fx
			sub.VelY[idxU] += fy
		}
		if vIsActive && !sub.Pinned[idxV] {
			sub.VelX[idxV] -= fx
			sub.VelY[idxV] -= fy
		}
	}

	// Apply anchor force, damping, and integrate positions
	var maxV float32
	for i := 0; i < n; i++ {
		if sub.Pinned[i] {
			continue
		}
		// Anchor spring towards original rest position
		anchorFx := (sub.OrigX[i] - sub.PosX[i]) * AnchorK * sub.Alpha
		anchorFy := (sub.OrigY[i] - sub.PosY[i]) * AnchorK * sub.Alpha

		sub.VelX[i] = (sub.VelX[i] + anchorFx) * Damping
		sub.VelY[i] = (sub.VelY[i] + anchorFy) * Damping

		sub.PosX[i] += sub.VelX[i] * dt
		sub.PosY[i] += sub.VelY[i] * dt

		v := sub.VelX[i]*sub.VelX[i] + sub.VelY[i]*sub.VelY[i]
		if v > maxV {
			maxV = v
		}
	}

	// Cool down if not breathing
	if !sub.Breathing {
		sub.Alpha *= 0.88
		if sub.Alpha < 0.005 || maxV < 0.01 {
			sub.Alpha = 0.0
			return false
		}
	}
	return true
}

// Release unpins the dragged node and lets the active set settle.
func (sub *ActiveSubgraph) Release() {
	if len(sub.Pinned) > 0 {
		sub.Pinned[0] = false
	}
	if len(sub.OrigX) > 0 {
		sub.OrigX[0] = sub.PosX[0]
		sub.OrigY[0] = sub.PosY[0]
	}
	sub.Alpha = 0.8 // Soft settle
}

// StopBreathing smoothly stops breathing oscillation and anchors nodes where they currently are
// so they settle naturally in place without snapping back to their old positions.
func (sub *ActiveSubgraph) StopBreathing() {
	if !sub.Active {
		return
	}
	sub.Breathing = false
	for i := 0; i < len(sub.NodeIDs); i++ {
		sub.OrigX[i] = sub.PosX[i]
		sub.OrigY[i] = sub.PosY[i]
	}
	sub.Alpha = 0.35 // Quick, smooth damping to rest
}

// Commit writes simulated positions back to the main graph model.
func (sub *ActiveSubgraph) Commit(g *Graph, sg *SpatialGrid) {
	if !sub.Active {
		return
	}

	// All nodes stay permanently at their current relaxed/simulated positions!
	for i, id := range sub.NodeIDs {
		newX := sub.PosX[i]
		newY := sub.PosY[i]
		if sg != nil {
			sg.MoveNode(g, id, newX, newY)
		} else {
			g.X[id] = newX
			g.Y[id] = newY
		}
	}

	sub.Active = false
	sub.Breathing = false
	sub.UserDragged = false
	clear(sub.NodeIndex)
}
