package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/workspace"
	"image"
	"strings"
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

func TestProofreadingWorkbenchRowDeleteButton(t *testing.T) {
	a, _ := proofreadingModelFixture(t)
	w := a.ensureProofreadingWorkbench()
	p := workspace.ProofreadingProject{ID: "test-row-delete", Title: "测试删除校对项", PageCount: 100}
	w.projects = []workspace.ProofreadingProject{p}

	var ops op.Ops
	g := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(988, 0), Max: image.Pt(988, 800)}}
	a.layoutProofreadingProjectRow(g, p)

	btn := w.button("delete-" + p.ID)
	if btn == nil {
		t.Fatal("expected delete button registered for project row")
	}

	// Click delete button
	btn.Click()
	var ops2 op.Ops
	g2 := layout.Context{Ops: &ops2, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(988, 0), Max: image.Pt(988, 800)}}
	a.layoutProofreadingProjectRow(g2, p)

	if !a.modal.Visible {
		t.Fatal("expected modal dialog visible after clicking delete")
	}
	if a.modal.DialogID != "proofread.workbench.delete" {
		t.Fatalf("expected dialog ID proofread.workbench.delete, got %s", a.modal.DialogID)
	}

	title := a.messageText(a.modal.TitleRef, a.modal.Title)
	if title != "删除校对任务" && title != "Delete Proofreading Project" {
		t.Fatalf("unexpected modal title: %s", title)
	}

	body := a.messageText(a.modal.MessageRef, a.modal.Message)
	if !strings.Contains(body, "不可逆") && !strings.Contains(body, "cannot be undone") {
		t.Fatalf("modal body must state operation is irreversible: %s", body)
	}

	// Verify modal dialog renders in overlay
	var ops3 op.Ops
	g3 := layout.Context{Ops: &ops3, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(988, 600), Max: image.Pt(988, 600)}}
	modalDims := a.layoutModalDialog(g3)
	if modalDims.Size.X == 0 || modalDims.Size.Y == 0 {
		t.Fatalf("modal dialog layout should have non-zero dimensions: %v", modalDims.Size)
	}
}
