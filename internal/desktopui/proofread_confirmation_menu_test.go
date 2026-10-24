package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"image"
	"runtime"
	"testing"
	"time"
)

func TestProofreadingConfirmationMenuStates(t *testing.T) {
	for _, tc := range []struct {
		status, label string
		enabled       bool
	}{
		{"proofread.history.unreviewed", "proofread.menu.reviewed", true},
		{"proofread.history.saved", "proofread.menu.reviewed", true},
		{"proofread.history.modified", "proofread.menu.confirm_changes", true},
		{"proofread.history.saving", "proofread.menu.saving", false},
		{"proofread.history.failed", "proofread.menu.retry_confirmation", true},
	} {
		label, enabled := proofreadingConfirmationMenuAction(tc.status)
		if label != tc.label || enabled != tc.enabled {
			t.Fatalf("%s: label=%s enabled=%v", tc.status, label, enabled)
		}
	}
}

func TestProofreadingMenuDetectsEditsAndReconfirmation(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	a.confirmProofreadingPage(0)
	if err := s.controller.Model.ReplacePage(1, "ab改d"); err != nil {
		t.Fatal(err)
	}
	a.refreshProofreadingModelView()
	if status := a.proofreadingPageStatusKey(0); status != "proofread.history.modified" {
		t.Fatalf("edited status=%s", status)
	}
	a.confirmProofreadingPage(0)
	if status := a.proofreadingPageStatusKey(0); status == "proofread.history.modified" {
		t.Fatal("reconfirmation did not capture edited body")
	}
}

func TestProofreadingConfirmationMenuVisualAndClick(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, s := proofreadingModelFixture(t)
		a.confirmProofreadingPage(0)
		if err := s.controller.Model.ReplacePage(1, "ab改d"); err != nil {
			t.Fatal(err)
		}
		a.refreshProofreadingModelView()
		s.xPageMenuOpen = true
		var ops op.Ops
		var router input.Router
		frame := func() *image.RGBA {
			ops.Reset()
			g := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(289*scale), int(75*scale)))}
			a.processProofreadingMenuEvents(g)
			a.layoutProofreadingPageMenuRows(g)
			shot := renderPresentationTestOps(t, &ops, g.Constraints.Max)
			router.Frame(&ops)
			return shot
		}
		shot := frame()
		// Pending confirmation is communicated by the existing button, with
		// no extra hint row or reserved blank area below it.
		p := shot.RGBAAt(int(90*scale), int(44*scale))
		if p.R < 240 || p.B >= p.G {
			t.Fatalf("%gx missing pending-confirmation button fill: %v", scale, p)
		}

		point := f32.Pt(125*scale, 55*scale)
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
		frame()
		frame()
		if status := a.proofreadingPageStatusKey(0); status == "proofread.history.modified" {
			t.Fatal("Confirm changes click did not reconfirm current body")
		}
	}
}
