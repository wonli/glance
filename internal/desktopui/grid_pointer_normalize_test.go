package desktopui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestSpreadsheetWindowsDragTracksNativeCursor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selection.csv")
	if err := os.WriteFile(path, []byte("A,B,C,D\n1,2,3,4\n5,6,7,8\n9,10,11,12\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := newAppWithRuntime(new(app.Window), newDesktopRuntime())
	t.Cleanup(func() { a.ensureDesktopRuntime().shutdown(); close(a.shutdown) })
	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeLoadedDocument(loaded)
	if err := a.installDocument(loaded); err != nil {
		t.Fatal(err)
	}
	var router input.Router
	var ops op.Ops
	origin := image.Pt(23, 256)
	render := func() {
		ops.Reset()
		stack := op.Offset(origin).Push(&ops)
		a.layoutVirtualGrid(layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(1000, 600)}})
		stack.Pop()
		router.Frame(&ops)
	}
	render()
	// Allow asynchronous column geometry to settle before pressing.
	for i := 0; i < 20; i++ {
		time.Sleep(time.Millisecond)
		render()
	}
	v := a.virtualGrid
	point := func(row, col int) f32.Point {
		r := editingCellRect(v, fmt.Sprintf("%c%d", 'A'+col-1, row), int(gridRowHeader), int(gridColumnHeader))
		return f32.Pt(float32(r.Min.X+5+origin.X), float32(r.Min.Y+5+origin.Y))
	}
	start := point(2, 2)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: start})
	render()
	end := point(4, 4)
	a.wordPointer.sample = wordPointerSample{valid: true, client: end, origin: origin}
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: end.Sub(f32.Pt(160, 70))})
	render()
	if a.selectedCell != "D4" {
		t.Fatalf("live drag selected %s; want D4", a.selectedCell)
	}
	if a.selection.Anchor != (CellPosition{Row: 1, Column: 1}) {
		t.Fatalf("drag moved anchor: %+v", a.selection.Anchor)
	}
	// Reverse across the anchor while the button remains down.
	end = point(1, 1)
	a.wordPointer.sample.client = end
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: end.Add(f32.Pt(160, 70))})
	render()
	if a.selectedCell != "A1" {
		t.Fatalf("reverse drag selected %s; want A1", a.selectedCell)
	}
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: end})
	render()
	if a.selectionDragging || a.selectedCell != "A1" {
		t.Fatalf("release: dragging=%v cell=%s", a.selectionDragging, a.selectedCell)
	}
}
