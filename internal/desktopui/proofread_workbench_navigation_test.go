package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/workspace"
	"image"
	"testing"
	"time"
)

func TestProofreadingFirstWorkbenchTabOpensList(t *testing.T) {
	a, _ := proofreadingModelFixture(t)
	var ops op.Ops
	var router input.Router
	frame := func() {
		ops.Reset()
		g := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1280, 42))}
		a.processProofreadingNavigation(g)
		a.layoutProofreadingChrome(g)
		router.Frame(&ops)
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(95, 20)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(95, 20)})
	frame()
	frame()
	if !a.ensureProofreadingWorkbench().active {
		t.Fatal("first document workbench tab did not open proofreading list")
	}
}
func TestProofreadingContinueCurrentTaskReturnsToEditor(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	s.projectDocumentID = "book"
	w := a.ensureProofreadingWorkbench()
	w.active = true
	a.resumeProofreadingProject(workspace.ProofreadingProject{ID: "book"})
	if w.active {
		t.Fatal("continue focused the same window but left its workbench visible")
	}
}

// Both tabs must remain reachable after switching to the workbench.
func TestProofreadingManuscriptTabReturnsFromWorkbench(t *testing.T) {
	a, _ := proofreadingModelFixture(t)
	w := a.ensureProofreadingWorkbench()
	w.active = true
	var ops op.Ops
	var router input.Router
	frame := func() {
		ops.Reset()
		g := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1280, 42))}
		a.processProofreadingNavigation(g)
		a.layoutProofreadingChrome(g)
		router.Frame(&ops)
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(300, 20)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(300, 20)})
	frame()
	frame()
	if w.active {
		t.Fatal("manuscript tab cannot return from the workbench")
	}
}
