package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestOfficeRibbonDropdownRecordingBoundary(t *testing.T) {
	for _, kind := range []officeRibbonMenuKind{officeRibbonMenuWord, officeRibbonMenuSpreadsheet} {
		t.Run(string(rune('0'+kind)), func(t *testing.T) {
			app := New(nil, "")
			state := newDocumentSessionState("")
			app.documentSessionState = state
			state.wordToolbarTab = wordTabFile
			state.toolbarTab = toolbarTabFile
			state.officeRibbonMenu = officeRibbonDropdownState{kind: kind, wordAction: "print", sheetSlot: toolbarActionOpenFile}
			var ops op.Ops
			router := new(input.Router)
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}, Constraints: layout.Constraints{Max: image.Pt(2560, 1600)}}
			app.layoutOfficeRibbonDropdown(gtx)
			router.Frame(&ops)
			if state.officeRibbonMenu.bounds.Empty() {
				t.Fatal("open dropdown has no bounds")
			}
			wantX := gtx.Dp(12)
			if kind == officeRibbonMenuWord {
				wantX = gtx.Dp(12 + 84*6)
			}
			if state.officeRibbonMenu.bounds.Min.X != wantX {
				t.Errorf("File menu x=%d; want %d", state.officeRibbonMenu.bounds.Min.X, wantX)
			}
		})
	}
}

func TestPenRibbonFileCommandGeometry(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 2)
	defer closeDocument()
	app := New(nil, "")
	app.documentSessionState = state
	for _, scale := range []float32{1, 2} {
		state.wordToolbarTab = wordTabFile
		var ops op.Ops
		router := new(input.Router)
		metric := unit.Metric{PxPerDp: scale, PxPerSp: scale}
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(metric.Dp(1440), metric.Dp(900))}}
		dims := app.layoutWordRibbon(gtx)
		if dims.Size.Y != metric.Dp(122) {
			t.Errorf("ribbon height = %v, want 34dp categories + 88dp commands", dims.Size.Y)
		}
		router.Frame(&ops)
		want := map[string]image.Rectangle{"新建": image.Rect(12, 44, 92, 112), "打开": image.Rect(96, 44, 176, 112), "最近使用": image.Rect(180, 44, 260, 112), "保存": image.Rect(264, 44, 344, 112)}
		var visit func([]input.SemanticNode)
		visit = func(nodes []input.SemanticNode) {
			for _, node := range nodes {
				if rect, ok := want[node.Desc.Description]; ok {
					expected := image.Rect(metric.Dp(unit.Dp(rect.Min.X)), metric.Dp(unit.Dp(rect.Min.Y)), metric.Dp(unit.Dp(rect.Max.X)), metric.Dp(unit.Dp(rect.Max.Y)))
					if node.Desc.Bounds != expected {
						t.Errorf("%s bounds = %v, want %v", node.Desc.Description, node.Desc.Bounds, expected)
					}
					delete(want, node.Desc.Description)
				}
				visit(node.Children)
			}
		}
		visit(router.AppendSemantics(nil))
		if len(want) > 0 {
			t.Errorf("missing commands: %v", want)
		}
	}
}

func TestPenRibbonOverviewCommandSets(t *testing.T) {
	if got := len(wordToolbarActions(wordTabReferences)); got != 8 {
		t.Errorf("Pen reference commands = %d, want 8", got)
	}
	if got := len(wordToolbarActions(wordTabView)); got != 12 {
		t.Errorf("Pen view commands = %d, want 12", got)
	}
}
