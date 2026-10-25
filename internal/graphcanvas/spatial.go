package graphcanvas

import (
	"math"

	"gioui.org/op"
)

// Chunk represents a spatial grid partition containing a subset of graph nodes and edges.
// Each chunk maintains its own persistent op.Ops to cache recorded macro operations.
type Chunk struct {
	Col, Row int
	MinX     float32
	MinY     float32
	MaxX     float32
	MaxY     float32

	NodeIndices   []int32
	InternalEdges []int32 // Edge indices where both src and dst belong to this chunk

	// Rendering cache
	Ops        *op.Ops
	Macro      op.CallOp
	MacroValid bool
}

// SpatialGrid partitions the world coordinate space into a uniform 2D grid of chunks.
type SpatialGrid struct {
	MinX, MinY float32
	MaxX, MaxY float32
	Cols, Rows int
	CellW      float32
	CellH      float32

	Chunks    []*Chunk
	NodeChunk []int32
}

// NewSpatialGrid builds a spatial grid indexing the given graph.
func NewSpatialGrid(g *Graph, cols, rows int) *SpatialGrid {
	if cols <= 0 {
		cols = 32
	}
	if rows <= 0 {
		rows = 32
	}

	// Compute bounding box with margin
	var minX, minY, maxX, maxY float32
	if g.NodeCount > 0 {
		minX, maxX = g.X[0], g.X[0]
		minY, maxY = g.Y[0], g.Y[0]
		for i := 1; i < g.NodeCount; i++ {
			if g.X[i] < minX {
				minX = g.X[i]
			}
			if g.X[i] > maxX {
				maxX = g.X[i]
			}
			if g.Y[i] < minY {
				minY = g.Y[i]
			}
			if g.Y[i] > maxY {
				maxY = g.Y[i]
			}
		}
	}
	margin := float32(200.0)
	minX -= margin
	minY -= margin
	maxX += margin
	maxY += margin

	cellW := (maxX - minX) / float32(cols)
	cellH := (maxY - minY) / float32(rows)

	sg := &SpatialGrid{
		MinX:   minX,
		MinY:   minY,
		MaxX:   maxX,
		MaxY:   maxY,
		Cols:   cols,
		Rows:   rows,
		CellW:  cellW,
		CellH:  cellH,
		Chunks: make([]*Chunk, cols*rows),
	}

	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			idx := r*cols + c
			sg.Chunks[idx] = &Chunk{
				Col:   c,
				Row:   r,
				MinX:  minX + float32(c)*cellW,
				MinY:  minY + float32(r)*cellH,
				MaxX:  minX + float32(c+1)*cellW,
				MaxY:  minY + float32(r+1)*cellH,
				Ops:   new(op.Ops),
			}
		}
	}

	// Place nodes into chunks and track which chunk each node belongs to
	sg.NodeChunk = make([]int32, g.NodeCount)
	for i := 0; i < g.NodeCount; i++ {
		c, r := sg.CellCoord(g.X[i], g.Y[i])
		idx := r*cols + c
		sg.Chunks[idx].NodeIndices = append(sg.Chunks[idx].NodeIndices, int32(i))
		sg.NodeChunk[i] = int32(idx)
	}

	// Classify edges
	for i := 0; i < g.EdgeCount; i++ {
		u := g.EdgeSrc[i]
		v := g.EdgeDst[i]
		cu := sg.NodeChunk[u]
		cv := sg.NodeChunk[v]
		if cu == cv {
			sg.Chunks[cu].InternalEdges = append(sg.Chunks[cu].InternalEdges, int32(i))
		}
	}

	return sg
}

// CellCoord maps world coordinates to clamped (col, row).
func (sg *SpatialGrid) CellCoord(x, y float32) (col, row int) {
	c := int((x - sg.MinX) / sg.CellW)
	r := int((y - sg.MinY) / sg.CellH)
	if c < 0 {
		c = 0
	} else if c >= sg.Cols {
		c = sg.Cols - 1
	}
	if r < 0 {
		r = 0
	} else if r >= sg.Rows {
		r = sg.Rows - 1
	}
	return c, r
}

// InvalidateChunk marks a chunk as needing re-recording.
func (sg *SpatialGrid) InvalidateChunk(col, row int) {
	if col < 0 || col >= sg.Cols || row < 0 || row >= sg.Rows {
		return
	}
	idx := row*sg.Cols + col
	sg.Chunks[idx].MacroValid = false
}

// MoveNode updates the spatial index and graph position when a node changes position.
func (sg *SpatialGrid) MoveNode(g *Graph, nodeID int32, newX, newY float32) {
	if nodeID < 0 || int(nodeID) >= g.NodeCount {
		return
	}
	oldIdx := int(sg.NodeChunk[nodeID])
	newC, newR := sg.CellCoord(newX, newY)
	newIdx := newR*sg.Cols + newC

	// Update node position in graph
	g.X[nodeID] = newX
	g.Y[nodeID] = newY

	sg.Chunks[oldIdx].MacroValid = false
	if oldIdx == newIdx {
		return
	}

	sg.Chunks[newIdx].MacroValid = false

	// Remove from oldChunk.NodeIndices
	oldNodes := sg.Chunks[oldIdx].NodeIndices
	for i, nid := range oldNodes {
		if nid == nodeID {
			last := len(oldNodes) - 1
			oldNodes[i] = oldNodes[last]
			sg.Chunks[oldIdx].NodeIndices = oldNodes[:last]
			break
		}
	}

	// Append to newChunk.NodeIndices
	sg.Chunks[newIdx].NodeIndices = append(sg.Chunks[newIdx].NodeIndices, nodeID)
	sg.NodeChunk[nodeID] = int32(newIdx)

	// Clean up oldChunk.InternalEdges
	oldInternal := sg.Chunks[oldIdx].InternalEdges
	nKeep := 0
	for _, e := range oldInternal {
		u := g.EdgeSrc[e]
		v := g.EdgeDst[e]
		if sg.NodeChunk[u] == int32(oldIdx) && sg.NodeChunk[v] == int32(oldIdx) {
			oldInternal[nKeep] = e
			nKeep++
		}
	}
	sg.Chunks[oldIdx].InternalEdges = oldInternal[:nKeep]
}

// InvalidateNode invalidates the chunk containing node i.
func (sg *SpatialGrid) InvalidateNode(g *Graph, nodeID int32) {
	if nodeID < 0 || int(nodeID) >= g.NodeCount {
		return
	}
	if int(nodeID) < len(sg.NodeChunk) {
		idx := int(sg.NodeChunk[nodeID])
		if idx >= 0 && idx < len(sg.Chunks) {
			sg.Chunks[idx].MacroValid = false
			return
		}
	}
	c, r := sg.CellCoord(g.X[nodeID], g.Y[nodeID])
	sg.InvalidateChunk(c, r)
}

// InvalidateAll marks all chunks dirty.
func (sg *SpatialGrid) InvalidateAll() {
	for _, ch := range sg.Chunks {
		ch.MacroValid = false
	}
}

// QueryVisibleChunks returns all chunks intersecting the given world AABB.
func (sg *SpatialGrid) QueryVisibleChunks(vx0, vy0, vx1, vy1 float32) []*Chunk {
	c0, r0 := sg.CellCoord(vx0, vy0)
	c1, r1 := sg.CellCoord(vx1, vy1)

	if c0 > c1 {
		c0, c1 = c1, c0
	}
	if r0 > r1 {
		r0, r1 = r1, r0
	}

	capacity := (c1 - c0 + 1) * (r1 - r0 + 1)
	res := make([]*Chunk, 0, capacity)

	for r := r0; r <= r1; r++ {
		rowOffset := r * sg.Cols
		for c := c0; c <= c1; c++ {
			res = append(res, sg.Chunks[rowOffset+c])
		}
	}
	return res
}

// FindNearestNode searches for the closest node within searchRadius from (wx, wy).
// Returns nodeID and true if found, or -1 and false.
func (sg *SpatialGrid) FindNearestNode(g *Graph, wx, wy, searchRadius float32) (int32, bool) {
	c0, r0 := sg.CellCoord(wx-searchRadius, wy-searchRadius)
	c1, r1 := sg.CellCoord(wx+searchRadius, wy+searchRadius)

	var bestNode int32 = -1
	bestDistSq := searchRadius * searchRadius

	for r := r0; r <= r1; r++ {
		for c := c0; c <= c1; c++ {
			ch := sg.Chunks[r*sg.Cols+c]
			for _, nid := range ch.NodeIndices {
				dx := g.X[nid] - wx
				dy := g.Y[nid] - wy
				distSq := dx*dx + dy*dy
				effectiveRadius := g.Radius[nid] + 6.0 // add hit-padding
				if distSq <= (effectiveRadius*effectiveRadius + bestDistSq) {
					// Actual distance from edge of node
					d := float32(math.Sqrt(float64(distSq)))
					if d-g.Radius[nid] < float32(math.Sqrt(float64(bestDistSq))) {
						bestDistSq = distSq
						bestNode = nid
					}
				}
			}
		}
	}

	return bestNode, bestNode >= 0
}
