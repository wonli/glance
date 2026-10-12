package desktopui

import (
	"image"
	"runtime"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/agent/provider"
)

func TestAgentModelClickFillsIDInSameFrame(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := New(nil, "")
	a.agentUI.settingsTab = "deepseek"
	a.agentUI.providerInput.SetText("deepseek")
	a.agentUI.modelInput.SetText("old-model")
	a.agentUI.modelLookup.models = []provider.ModelInfo{{ID: "deepseek-flash", Name: "DeepSeek-V4.1-Flash"}}
	var router input.Router
	var ops op.Ops
	size := image.Pt(700, 600)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: size}}
		a.layoutAgentSettingsPageFields(gtx)
		router.Frame(&ops)
	}
	frame()
	var bounds image.Rectangle
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, n := range nodes {
			if n.Desc.Description == "DeepSeek-V4.1-Flash" || n.Desc.Label == "DeepSeek-V4.1-Flash" {
				bounds = n.Desc.Bounds
			}
			visit(n.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	if bounds.Empty() {
		t.Fatal("model option has no clickable bounds")
	}
	if bounds.Max.X > size.X {
		t.Fatalf("model option extends outside visible list: %v", bounds)
	}
	pos := f32.Pt(float32(bounds.Min.X+20), float32(bounds.Min.Y+10))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 4, Buttons: pointer.ButtonPrimary, Position: pos})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 4, Position: pos})
	frame()
	if a.agentUI.modelInput.Text() != "deepseek-flash" {
		t.Fatalf("clicked model did not fill API ID: %q", a.agentUI.modelInput.Text())
	}
	clicked := renderPresentationTestOps(t, &ops, size)
	frame()
	stable := renderPresentationTestOps(t, &ops, size)
	changed := 0
	changedBounds := image.Rectangle{}
	// Compare the form above the option row; native click ink animates below it.
	for y := 0; y < bounds.Min.Y-30; y++ {
		for x := 0; x < size.X; x++ {
			if clicked.RGBAAt(x, y) != stable.RGBAAt(x, y) {
				changed++
				changedBounds = changedBounds.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if changed > 0 {
		t.Fatalf("model click retains old field until another frame: %d changed pixels in %v", changed, changedBounds)
	}
}
