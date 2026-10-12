package desktopui

import (
	"image"

	"testing"
	"time"

	"gioui.org/io/input"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"gioui.org/widget/material"
)

func TestWordRibbonKeepsHeightAndPictureActionsStayOnOneRow(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 2)
	defer closeDocument()
	state.wordToolbarTab = wordTabPicture
	state.selectedWordImageID = "selected-image"
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{
		Source:      router.Source(),
		Ops:         &ops,
		Now:         time.Now(),
		Constraints: layout.Constraints{Max: image.Pt(1100, 700)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	app.layoutWordFlow(gtx)
	router.Frame(&ops)
	if wordRibbonCategoryHeightDP != 34 || wordRibbonCommandHeightDP != 68 || wordRibbonHeightDP != 102 {
		t.Fatalf("Word ribbon heights = categories %d dp, commands %d dp, total %d dp; want Pen Clpgm 34/68/102 dp", wordRibbonCategoryHeightDP, wordRibbonCommandHeightDP, wordRibbonHeightDP)
	}
	want := map[string]image.Rectangle{
		"替换图片": {}, "删除图片": {}, "调整大小": {}, "向左旋转 90°": {}, "向右旋转 90°": {},
	}
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if _, ok := want[node.Desc.Description]; ok {
				want[node.Desc.Description] = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	rowTop, rowBottom := 0, 0
	for label, bounds := range want {
		if bounds.Empty() {
			t.Errorf("picture action %q has no visible bounds", label)
			continue
		}
		if rowTop == 0 || bounds.Min.Y < rowTop {
			rowTop = bounds.Min.Y
		}
		if bounds.Max.Y > rowBottom {
			rowBottom = bounds.Max.Y
		}
	}
	if rowBottom-rowTop > 48 {
		t.Errorf("picture ribbon actions occupy %d vertical pixels; want one 48 dp row", rowBottom-rowTop)
	}
	firstTop := -1
	for label, bounds := range want {
		if bounds.Empty() {
			continue
		}
		if firstTop < 0 {
			firstTop = bounds.Min.Y
			continue
		}
		if delta := bounds.Min.Y - firstTop; delta < -2 || delta > 2 {
			t.Errorf("picture action %q starts at y=%d; want all actions on one horizontal row near y=%d", label, bounds.Min.Y, firstTop)
		}
	}
}

func TestWordHomeRibbonKeepsCommandGeometryAfterAgentEntry(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 2)
	defer closeDocument()
	state.wordToolbarTab = wordTabHome
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{
		Source:      router.Source(),
		Ops:         &ops,
		Now:         time.Now(),
		Constraints: layout.Constraints{Max: image.Pt(1440, 900)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	app.layoutWordFlow(gtx)
	router.Frame(&ops)
	want := map[string]image.Rectangle{
		"文件":    {Min: image.Pt(22, 2), Max: image.Pt(53, 32)},
		"开始":    {Min: image.Pt(81, 2), Max: image.Pt(112, 32)},
		"AI 助手": {Min: image.Pt(11, 40), Max: image.Pt(51, 96)},
		"粘贴":    {Min: image.Pt(64, 44), Max: image.Pt(108, 92)},
		"剪切":    {Min: image.Pt(114, 44), Max: image.Pt(158, 92)},
		"字体":    {Min: image.Pt(283, 44), Max: image.Pt(327, 92)},
		"加粗":    {Min: image.Pt(433, 44), Max: image.Pt(477, 92)},
		"左对齐":   {Min: image.Pt(802, 44), Max: image.Pt(846, 92)},
		"查找":    {Min: image.Pt(1121, 44), Max: image.Pt(1165, 92)},
		"编辑器":   {Min: image.Pt(1290, 44), Max: image.Pt(1334, 92)},
	}
	found := make(map[string]image.Rectangle, len(want))
	hasHistoryAction := false
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			switch node.Desc.Description {
			case "撤销", "重做":
				hasHistoryAction = true
			}
			if _, ok := want[node.Desc.Description]; ok {
				found[node.Desc.Description] = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	for label, expected := range want {
		bounds := found[label]
		if bounds.Empty() {
			t.Errorf("Clpgm ribbon control %q has no semantic bounds", label)
			continue
		}
		if bounds.Min.X != expected.Min.X || (expected.Max.X > expected.Min.X && bounds.Max.X != expected.Max.X) || bounds.Min.Y != expected.Min.Y || (expected.Max.Y > expected.Min.Y && bounds.Max.Y != expected.Max.Y) {
			t.Errorf("Clpgm ribbon control %q bounds = %v; want %v", label, bounds, expected)
		}
	}
	if hasHistoryAction {
		t.Error("Home ribbon includes undo/redo controls that are absent from Pen node Clpgm")
	}
}
