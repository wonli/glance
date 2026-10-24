package desktopui

import (
	"image"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/proofhistory"
)

func TestProofreadingTextHistoryRendersChangesAndRawDisclosure(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		change := proofhistory.PageChange{Entry: proofhistory.Entry{Hash: "abcdef0123456789"}, TextAvailable: true, Diff: strings.Repeat("+ raw json\n", 30), TextChanges: []proofhistory.TextChange{
			{Kind: proofhistory.TextUnchanged, Text: "正文："},
			{Kind: proofhistory.TextDeleted, Text: strings.Repeat("删除", 70)},
			{Kind: proofhistory.TextAdded, Text: "新增文字"},
			{Kind: proofhistory.TextUnchanged, Text: "。"},
		}}
		var ops op.Ops
		var router input.Router
		frame := func() (layout.Dimensions, *image.RGBA) {
			ops.Reset()
			g := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(int(500*scale), int(2000*scale))}}
			dims := a.layoutProofreadingHistoryCommit(g, change)
			shot := renderPresentationTestOps(t, &ops, image.Pt(int(500*scale), int(2000*scale)))
			router.Frame(&ops)
			return dims, shot
		}
		collapsed, shot := frame()
		view := a.ensureProofreadingWorkbench().historyTextViews[change.Entry.Hash]
		if view == nil || view.rawOpen || strings.Contains(view.text, "raw json") {
			t.Fatal("raw JSON shown by default")
		}
		red, green := 0, 0
		for y := 0; y < collapsed.Size.Y; y++ {
			for x := 0; x < shot.Bounds().Dx(); x++ {
				p := shot.RGBAAt(x, y)
				if p.R > 140 && p.R > p.G+30 && p.R > p.B+20 {
					red++
				}
				if p.G > 70 && p.G > p.R+20 && p.G > p.B {
					green++
				}
			}
		}
		if red < int(100*scale) || green < int(10*scale) {
			t.Fatalf("%gx missing deletion/addition colors: red=%d green=%d", scale, red, green)
		}
		regions := view.selectable.Regions(3, 143, nil)
		if len(regions) < 2 {
			t.Fatal("deleted text did not wrap")
		}
		for _, region := range regions {
			y := region.Bounds.Min.Y + region.Bounds.Dy()/2 + int(97*scale)
			// Card body origin is (16dp,91dp), followed by text inset (8dp,6dp).
			x := region.Bounds.Min.X + int(24*scale)
			p := shot.RGBAAt(x, y)
			if p.R <= p.G+30 {
				t.Fatalf("%gx missing wrapped strike at %d,%d: %v", scale, x, y, p)
			}
		}
		click := func() {
			point := f32.Pt(40*scale, float32(collapsed.Size.Y)-25*scale)
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
		}
		click()
		frame()
		expanded, _ := frame()
		if !view.rawOpen || expanded.Size.Y <= collapsed.Size.Y+int(100*scale) {
			t.Fatalf("%gx raw disclosure failed: open=%v collapsed=%d expanded=%d", scale, view.rawOpen, collapsed.Size.Y, expanded.Size.Y)
		}
		click()
		frame()
		frame()
		if view.rawOpen {
			t.Fatal("raw data did not collapse")
		}
		// Selection remains available through the parent commit header even
		// though the body contains a selectable and a disclosure button.
		a.ensureProofreadingWorkbench().selectedCommit = "other"
		point := f32.Pt(40*scale, 20*scale)
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
		frame()
		frame()
		if a.ensureProofreadingWorkbench().selectedCommit != change.Entry.Hash {
			t.Fatal("commit header could not select the historical version")
		}
	}
}

func TestProofreadingHistoryContentOmitsUnchangedPageLines(t *testing.T) {
	a := New(nil, "")
	change := proofhistory.PageChange{Entry: proofhistory.Entry{Hash: "line-change"}, TextAvailable: true, TextChanges: proofhistory.CompareText("第一行不变\n春眠不觉晓\n最后一行不变", "第一行不变\n春眠不觉晓新增\n最后一行不变")}
	var ops op.Ops
	g := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(600, 1000)}}
	a.layoutProofreadingHistoryContent(g, change)
	view := a.ensureProofreadingWorkbench().historyTextViews[change.Entry.Hash]
	if view.text != "春眠不觉晓新增\n" {
		t.Fatalf("history repeated unchanged page content: %q", view.text)
	}
}
