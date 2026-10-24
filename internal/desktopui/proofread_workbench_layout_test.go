package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/workspace"
	"image"
	"testing"
)

func TestProofreadingWorkbenchRowKeepsNaturalHeight(t *testing.T) {
	a, _ := proofreadingModelFixture(t)
	var ops op.Ops
	g := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(988, 0), Max: image.Pt(988, 800)}}
	d := a.layoutProofreadingProjectRow(g, workspace.ProofreadingProject{ID: "x", Title: "书稿", PageCount: 500})
	if d.Size.Y > 140 {
		t.Fatalf("row expanded to viewport height: %v", d.Size)
	}
}
