package graphcanvas

import (
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

func TestBenchmarkGraphGeneration(t *testing.T) {
	start := time.Now()
	g := GenerateBenchmarkGraph(100000, 150, 42)
	dur := time.Since(start)

	if g.NodeCount != 100000 {
		t.Fatalf("expected 100000 nodes, got %d", g.NodeCount)
	}
	if g.EdgeCount < 100000 {
		t.Fatalf("expected at least 100000 edges, got %d", g.EdgeCount)
	}

	t.Logf("Generated 100,000 nodes and %d edges in %v", g.EdgeCount, dur)
}

func TestSpatialGridAndHitTesting(t *testing.T) {
	g := GenerateBenchmarkGraph(100000, 150, 42)

	start := time.Now()
	sg := NewSpatialGrid(g, 32, 32)
	buildDur := time.Since(start)
	t.Logf("Built 32x32 SpatialGrid (1024 chunks) in %v", buildDur)

	// Hit test performance
	hitStart := time.Now()
	testNode := int32(500)
	targetX := g.X[testNode]
	targetY := g.Y[testNode]

	found, ok := sg.FindNearestNode(g, targetX, targetY, 50.0)
	hitDur := time.Since(hitStart)

	if !ok {
		t.Fatalf("expected to find node near (%f, %f)", targetX, targetY)
	}
	t.Logf("Hit test found node %d in %v", found, hitDur)

	// Viewport query
	visible := sg.QueryVisibleChunks(-1000, -1000, 1000, 1000)
	t.Logf("Visible chunks for [-1000, 1000]: %d chunks", len(visible))
	if len(visible) == 0 {
		t.Fatalf("expected visible chunks, got 0")
	}
}

func TestActiveSetPhysicsPerformance(t *testing.T) {
	g := GenerateBenchmarkGraph(100000, 150, 42)
	sg := NewSpatialGrid(g, 32, 32)

	sub := NewActiveSubgraph()
	draggedNode := int32(0) // hub node with high degree

	sub.Activate(g, draggedNode, g.X[draggedNode]+50, g.Y[draggedNode]+50)
	t.Logf("Active set size for hub %d: %d nodes, %d active edges",
		draggedNode, len(sub.NodeIDs), len(sub.ActiveEdges))

	if len(sub.NodeIDs) == 0 {
		t.Fatalf("active set should not be empty")
	}

	// Benchmark 60 physics steps
	start := time.Now()
	stepCount := 0
	for i := 0; i < 60; i++ {
		sub.UpdateTarget(g.X[draggedNode]+float32(i), g.Y[draggedNode]+float32(i))
		active := sub.Step(g, 0.016)
		stepCount++
		if !active {
			break
		}
	}
	totalDur := time.Since(start)
	avgStepDur := totalDur / time.Duration(stepCount)
	t.Logf("Executed %d physics steps in %v (avg %v per step)", stepCount, totalDur, avgStepDur)

	if avgStepDur > 2*time.Millisecond {
		t.Errorf("physics step took too long: %v (budget < 2ms)", avgStepDur)
	}

	// Commit back
	sub.Commit(g, sg)
}

func TestPhysicalBreathingTugBetweenNodes(t *testing.T) {
	g := GenerateBenchmarkGraph(1000, 50, 42)
	sub := NewActiveSubgraph()
	hub := g.HubIDs[0]

	sub.Activate(g, hub, g.X[hub], g.Y[hub])
	sub.Breathing = true
	sub.Alpha = 0.5

	// Run 30 steps (approx 0.5s of breathing expansion)
	for i := 0; i < 30; i++ {
		sub.Step(g, 0.016)
	}

	// Verify that neighbor nodes gently breathe with subtle micro-motion
	maxDisplacement := float32(0)
	for i := 1; i < len(sub.NodeIDs); i++ {
		dx := sub.PosX[i] - sub.OrigX[i]
		dy := sub.PosY[i] - sub.OrigY[i]
		disp := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if disp > maxDisplacement {
			maxDisplacement = disp
		}
	}

	t.Logf("Breathing subtle micro-motion maximum node displacement: %.2f world units", maxDisplacement)
	if maxDisplacement < 0.2 {
		t.Fatalf("expected subtle micro-motion, got too small: %.4f", maxDisplacement)
	}
	if maxDisplacement > 4.0 {
		t.Fatalf("breathing displacement too exaggerated! expected <= 4.0 units, got %.2f", maxDisplacement)
	}

	// Now stop breathing and step until cool
	sub.Breathing = false
	for i := 0; i < 120; i++ {
		if !sub.Step(g, 0.016) {
			break
		}
	}
	t.Logf("Physical breathing smoothly cooled down to rest with alpha=%.4f", sub.Alpha)
}

func TestClickDeselectRetainsClickability(t *testing.T) {
	g := GenerateBenchmarkGraph(1000, 50, 42)
	sg := NewSpatialGrid(g, 32, 32)
	sub := NewActiveSubgraph()
	hub := g.HubIDs[0]

	// 1. Initial hit test before any interaction
	origX := g.X[hub]
	origY := g.Y[hub]
	nid, ok := sg.FindNearestNode(g, origX, origY, 20.0)
	if !ok || nid != hub {
		t.Fatalf("initial hit test failed: expected %d, got %d (ok=%v)", hub, nid, ok)
	}

	// 2. User clicks hub to start breathing (UserDragged = false)
	sub.Activate(g, hub, origX, origY)
	sub.Breathing = true
	sub.Alpha = 0.5

	// Run 25 steps with breathing oscillation active
	for i := 0; i < 25; i++ {
		sub.Step(g, 0.016)
	}

	// Check that neighbors oscillated away from resting positions
	oscillated := false
	for i := 1; i < len(sub.NodeIDs); i++ {
		if sub.PosX[i] != sub.OrigX[i] || sub.PosY[i] != sub.OrigY[i] {
			oscillated = true
			break
		}
	}
	if !oscillated {
		t.Fatalf("expected neighbor nodes to oscillate during breathing")
	}

	// 3. User clicks background to lose focus -> StopBreathing settles nodes in place at point B
	sub.StopBreathing()
	for i := 0; i < 120; i++ {
		if !sub.Step(g, 0.016) {
			break
		}
	}

	expectedFinalX := sub.PosX[0]
	expectedFinalY := sub.PosY[0]

	// 4. Commit back to graph and spatial grid
	sub.Commit(g, sg)

	// 5. Verify hub node stays at its settled position B (does NOT snap back rigidly to A)
	if g.X[hub] != expectedFinalX || g.Y[hub] != expectedFinalY {
		t.Fatalf("hub node failed to stay at point B: (%f, %f) vs expected (%f, %f)",
			g.X[hub], g.Y[hub], expectedFinalX, expectedFinalY)
	}

	// 6. Verify hit test on hub node at point B succeeds 100%
	foundHub, okHub := sg.FindNearestNode(g, g.X[hub], g.Y[hub], 20.0)
	if !okHub || foundHub != hub {
		t.Fatalf("hub node became unclickable at point B! expected %d, got %d (ok=%v)", hub, foundHub, okHub)
	}

	// 7. Verify all neighbor nodes remain 100% clickable at their settled positions with spatial grid intact
	for _, neighbor := range g.Adj[hub] {
		nx := g.X[neighbor]
		ny := g.Y[neighbor]
		foundNeighbor, okNeighbor := sg.FindNearestNode(g, nx, ny, 20.0)
		if !okNeighbor || foundNeighbor != neighbor {
			t.Fatalf("neighbor node %d became unclickable! found=%d ok=%v", neighbor, foundNeighbor, okNeighbor)
		}
	}
	t.Log("Successfully verified: clicking and deselecting stays at point B with 100% clickability and zero snap-back")
}

func TestDragCrossChunkRetainsClickability(t *testing.T) {
	g := GenerateBenchmarkGraph(1000, 50, 42)
	sg := NewSpatialGrid(g, 32, 32)
	sub := NewActiveSubgraph()
	nodeID := int32(10)

	oldX := g.X[nodeID]
	oldY := g.Y[nodeID]
	oldChunkIdx := sg.NodeChunk[nodeID]

	// Drag node 500 units away into another chunk
	newX := oldX + 500.0
	newY := oldY + 500.0

	sub.Activate(g, nodeID, oldX, oldY)
	sub.UpdateTarget(newX, newY)
	if !sub.UserDragged {
		t.Fatalf("expected UserDragged to be true after UpdateTarget")
	}

	// Step once so PosX[0] updates
	sub.Step(g, 0.016)
	sub.Release()
	sub.Commit(g, sg)

	newChunkIdx := sg.NodeChunk[nodeID]
	if newChunkIdx == oldChunkIdx {
		t.Fatalf("expected node to move to a different chunk, stayed in %d", oldChunkIdx)
	}

	// Check that old chunk does not contain nodeID
	for _, id := range sg.Chunks[oldChunkIdx].NodeIndices {
		if id == nodeID {
			t.Fatalf("old chunk %d still contains moved node %d", oldChunkIdx, nodeID)
		}
	}

	// Check that new chunk DOES contain nodeID
	hasNode := false
	for _, id := range sg.Chunks[newChunkIdx].NodeIndices {
		if id == nodeID {
			hasNode = true
			break
		}
	}
	if !hasNode {
		t.Fatalf("new chunk %d does NOT contain moved node %d", newChunkIdx, nodeID)
	}

	// Check that hit testing at new position finds the node!
	foundNode, ok := sg.FindNearestNode(g, newX, newY, 20.0)
	if !ok || foundNode != nodeID {
		t.Fatalf("hit test at new position failed: expected %d, got %d (ok=%v)", nodeID, foundNode, ok)
	}

	t.Logf("Successfully verified cross-chunk drag: node %d migrated from chunk %d to %d and remains clickable",
		nodeID, oldChunkIdx, newChunkIdx)
}

func TestChunkMacroRecordingAndReplay(t *testing.T) {
	g := GenerateBenchmarkGraph(10000, 50, 42)
	sg := NewSpatialGrid(g, 16, 16)

	// Test recording into persistent ops
	ch := sg.Chunks[0]
	m := op.Record(ch.Ops)

	// Record a couple of test shapes
	rect := image.Rect(0, 0, 50, 50)
	paint.FillShape(ch.Ops, color.NRGBA{R: 255, A: 255}, clip.Rect(rect).Op())
	ch.Macro = m.Stop()
	ch.MacroValid = true

	// Now replay into a mock gtx
	frameOps := new(op.Ops)
	gtx := layout.Context{
		Ops: frameOps,
		Constraints: layout.Exact(image.Pt(800, 600)),
	}

	// Push transform
	aff := f32.Affine2D{}.Offset(f32.Pt(400, 300)).Scale(f32.Pt(0, 0), f32.Pt(0.5, 0.5))
	trans := op.Affine(aff).Push(gtx.Ops)
	ch.Macro.Add(gtx.Ops)
	trans.Pop()

	t.Log("Successfully recorded and replayed chunk macro under op.Affine")
}

func BenchmarkCanvasStaticFrame(b *testing.B) {
	g := GenerateBenchmarkGraph(100000, 150, 42)
	canvas := NewCanvas(g)

	size := image.Pt(1920, 1080)
	frameOps := new(op.Ops)

	// Warmup initial recording
	gtx := layout.Context{
		Ops:         frameOps,
		Constraints: layout.Exact(size),
	}
	canvas.Layout(gtx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frameOps.Reset()
		// Pan camera slightly
		canvas.Camera.Pan(0.5, 0.5)
		gtx.Ops = frameOps
		canvas.Layout(gtx)
	}
}

func BenchmarkCanvasActiveDragFrame(b *testing.B) {
	g := GenerateBenchmarkGraph(100000, 150, 42)
	canvas := NewCanvas(g)

	size := image.Pt(1920, 1080)
	frameOps := new(op.Ops)

	// Activate physics on node 0
	canvas.Physics.Activate(g, 0, g.X[0]+50, g.Y[0]+50)
	canvas.draggingNode = true

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frameOps.Reset()
		canvas.Physics.UpdateTarget(g.X[0]+float32(i%100), g.Y[0]+float32(i%100))
		gtx := layout.Context{
			Ops:         frameOps,
			Constraints: layout.Exact(size),
		}
		canvas.Layout(gtx)
	}
}

