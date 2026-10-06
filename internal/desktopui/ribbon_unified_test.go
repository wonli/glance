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

	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
)

func TestRibbonHeightAndCommandStripUnified(t *testing.T) {
	// Verify height constants are unified across formats:
	// Word: 34 dp categories + 68 dp command strip = 102 dp total
	// Excel: 34 dp categories + 68 dp command strip = 102 dp total
	// JSON: the same 68 dp command strip as Excel, without category tabs
	if wordRibbonCategoryHeightDP != 34 || wordRibbonCommandHeightDP != 68 || wordRibbonHeightDP != 102 {
		t.Fatalf("Word ribbon heights mismatch: category=%d, command=%d, total=%d; want 34/68/102",
			wordRibbonCategoryHeightDP, wordRibbonCommandHeightDP, wordRibbonHeightDP)
	}

	if jsonToolbarHeightDP != 68 {
		t.Fatalf("JSON toolbar height = %d dp, want 68 dp (Excel command strip)", jsonToolbarHeightDP)
	}

	if toolbarButtonWidth != 44 || toolbarButtonHeight != 48 {
		t.Fatalf("Word ribbon button geometry = %v x %v, want 44x48 dp (kiBim)", toolbarButtonWidth, toolbarButtonHeight)
	}

	if excelToolbarButtonWidth != 48 || excelToolbarButtonHeight != 48 {
		t.Fatalf("Excel ribbon button geometry = %v x %v, want 48x48 dp (nmShb)", excelToolbarButtonWidth, excelToolbarButtonHeight)
	}

	if toolbarItemGap != 6 {
		t.Fatalf("Word ribbon button gap = %v dp, want 6 dp (kiBim)", toolbarItemGap)
	}

	if excelToolbarItemGap != 3 {
		t.Fatalf("Excel ribbon button gap = %v dp, want 3 dp (nmShb)", excelToolbarItemGap)
	}

	app := New(nil, "")
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:    ops,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Max: image.Pt(1440, 900),
		},
	}

	// Test Excel toolbar height
	book := &toolbarTestBook{editable: true}
	state := newDocumentSessionState("")
	state.book = book
	app.documentSessionState = state
	app.book = book
	dimsExcel := app.layoutSpreadsheetToolbar(gtx)
	if dimsExcel.Size.Y != 102 {
		t.Fatalf("Excel ribbon height = %d px, want 102 px", dimsExcel.Size.Y)
	}

	// Test JSON toolbar height
	jsonDoc := jsondoc.NewBlank()
	stateJSON := newDocumentSessionState("")
	stateJSON.jsonDoc = jsonDoc
	app.documentSessionState = stateJSON
	app.jsonDoc = jsonDoc
	dimsJSON := app.layoutJSONToolbar(gtx)
	if dimsJSON.Size.Y != 68 {
		t.Fatalf("JSON ribbon toolbar height = %d px, want 68 px", dimsJSON.Size.Y)
	}
}

func TestRibbonButtonsTopIconBottomTextLayout(t *testing.T) {
	theme := material.NewTheme()
	ops := new(op.Ops)
	router := new(input.Router)
	gtx := layout.Context{
		Source: router.Source(),
		Ops:    ops,
		Now:    time.Now(),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Max: image.Pt(200, 200),
		},
	}

	// Render a standalone toolbarIconButton
	dims := toolbarIconButton(gtx, theme, nil, nil, toolbarIconSize, "测试", "测试按钮", true, false, contentSurface)
	if dims.Size.X != 44 || dims.Size.Y != 48 {
		t.Fatalf("toolbarIconButton dims = %v, want 44x48", dims.Size)
	}
	router.Frame(ops)

	foundDescription := false
	var checkSemantics func([]input.SemanticNode)
	checkSemantics = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "测试按钮" {
				foundDescription = true
				if node.Desc.Bounds.Empty() {
					t.Fatalf("semantic node has empty bounds")
				}
			}
			checkSemantics(node.Children)
		}
	}
	checkSemantics(router.AppendSemantics(nil))
	if !foundDescription {
		t.Fatalf("toolbarIconButton description not found in semantic tree")
	}
}

func TestWordAndExcelAndJSONRibbonButtonsRenderUnifiedLabels(t *testing.T) {
	// Verify that Word home ribbon includes labels on controls
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("test.docx")
	state.word = word
	state.wordToolbarTab = wordTabHome
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	ops := new(op.Ops)
	gtx := layout.Context{
		Source: router.Source(),
		Ops:    ops,
		Now:    time.Now(),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Max: image.Pt(1440, 900),
		},
	}
	app.layoutWordRibbon(gtx)
	router.Frame(ops)

	wantedWordLabels := []string{"粘贴", "剪切", "复制", "加粗", "查找"}
	foundWordLabels := make(map[string]bool)
	var visitWord func([]input.SemanticNode)
	visitWord = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			for _, label := range wantedWordLabels {
				if node.Desc.Description == label {
					foundWordLabels[label] = true
				}
			}
			visitWord(node.Children)
		}
	}
	visitWord(router.AppendSemantics(nil))
	for _, label := range wantedWordLabels {
		if !foundWordLabels[label] {
			t.Errorf("Word ribbon missing label %q in semantic tree", label)
		}
	}

	// Verify that Excel home ribbon includes labels on controls
	book := &toolbarTestBook{editable: true}
	stateExcel := newDocumentSessionState("test.xlsx")
	stateExcel.book = book
	stateExcel.toolbarTab = toolbarTabHome
	appExcel := New(nil, "")
	appExcel.documentSessionState = stateExcel
	appExcel.book = book
	appExcel.theme = material.NewTheme()
	opsExcel := new(op.Ops)
	routerExcel := new(input.Router)
	gtxExcel := layout.Context{
		Source: routerExcel.Source(),
		Ops:    opsExcel,
		Now:    time.Now(),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Max: image.Pt(1440, 900),
		},
	}
	appExcel.layoutSpreadsheetToolbar(gtxExcel)
	routerExcel.Frame(opsExcel)

	wantedExcelLabels := []string{"剪切", "复制", "粘贴", "加粗", "排序"}
	foundExcelLabels := make(map[string]bool)
	var visitExcel func([]input.SemanticNode)
	visitExcel = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			for _, label := range wantedExcelLabels {
				if node.Desc.Description == label {
					foundExcelLabels[label] = true
				}
			}
			visitExcel(node.Children)
		}
	}
	visitExcel(routerExcel.AppendSemantics(nil))
	for _, label := range wantedExcelLabels {
		if !foundExcelLabels[label] {
			t.Errorf("Excel ribbon missing label %q in semantic tree", label)
		}
	}

	// Verify that JSON toolbar includes labels on controls
	jsonDoc := jsondoc.NewBlank()
	stateJSON := newDocumentSessionState("test.json")
	stateJSON.jsonDoc = jsonDoc
	appJSON := New(nil, "")
	appJSON.documentSessionState = stateJSON
	appJSON.jsonDoc = jsonDoc
	appJSON.theme = material.NewTheme()
	opsJSON := new(op.Ops)
	routerJSON := new(input.Router)
	gtxJSON := layout.Context{
		Source: routerJSON.Source(),
		Ops:    opsJSON,
		Now:    time.Now(),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Max: image.Pt(1440, 900),
		},
	}
	appJSON.layoutJSONToolbar(gtxJSON)
	routerJSON.Frame(opsJSON)

	wantedJSONLabels := []string{"保存", "另存为", "导入", "格式化", "压缩", "排序", "校验"}
	foundJSONLabels := make(map[string]bool)
	var visitJSON func([]input.SemanticNode)
	visitJSON = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			for _, label := range wantedJSONLabels {
				if node.Desc.Description == label {
					foundJSONLabels[label] = true
					if node.Desc.Bounds.Size() != image.Pt(48, 48) {
						t.Errorf("JSON %s button bounds=%v, want Excel 48x48", label, node.Desc.Bounds)
					}
				}
			}
			visitJSON(node.Children)
		}
	}
	visitJSON(routerJSON.AppendSemantics(nil))
	for _, label := range wantedJSONLabels {
		if !foundJSONLabels[label] {
			t.Errorf("JSON ribbon missing label %q in semantic tree", label)
		}
	}
}

func TestWordRibbonSubmenuScrollableAndLeftAligned(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("test.docx")
	state.word = word
	state.wordToolbarTab = wordTabHome
	app := &App{documentSessionState: state, theme: material.NewTheme()}

	// Test 1: left alignment at normal width
	router := new(input.Router)
	ops := new(op.Ops)
	gtx := layout.Context{
		Source: router.Source(),
		Ops:    ops,
		Now:    time.Now(),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Max: image.Pt(1440, 900),
		},
	}
	app.layoutWordRibbon(gtx)
	router.Frame(ops)

	var pasteBounds image.Rectangle
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "粘贴" {
				pasteBounds = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	if pasteBounds.Min.X != 8 {
		t.Errorf("Word Ribbon '粘贴' Min.X = %d; want 8 (left aligned with 8dp inset)", pasteBounds.Min.X)
	}

	// Test 2: narrow width scrolling capability via wordToolbarActionList
	narrowOps := new(op.Ops)
	narrowRouter := new(input.Router)
	narrowGtx := layout.Context{
		Source: narrowRouter.Source(),
		Ops:    narrowOps,
		Now:    time.Now(),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Max: image.Pt(400, 900),
		},
	}
	// Simulate scrolling to item index 4 ("字体")
	state.wordToolbarActionList.List.Position.First = 4
	app.layoutWordRibbon(narrowGtx)
	narrowRouter.Frame(narrowOps)

	var fontBounds image.Rectangle
	visitNarrow := func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "字体" {
				fontBounds = node.Desc.Bounds
			}
		}
	}
	visitNarrow(narrowRouter.AppendSemantics(nil))
	if fontBounds.Empty() {
		t.Error("Word Ribbon did not render '字体' after scrolling horizontally")
	}
	if fontBounds.Min.X != 8 && fontBounds.Min.X != 10 {
		t.Errorf("scrolled '字体' Min.X = %d; want 8 or 10 (scrolled to start)", fontBounds.Min.X)
	}
}

func TestWordRibbonShortMenusStayLeftAligned(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	for _, tab := range []wordToolbarTab{wordTabInsert, wordTabDesign, wordTabLayout, wordTabReview, wordTabView, wordTabPicture} {
		for _, width := range []int{1440, 2000} {
			state := newDocumentSessionState("test.docx")
			state.word = word
			state.wordToolbarTab = tab
			state.selectedWordImageID = "test-image"
			app := &App{documentSessionState: state, theme: material.NewTheme()}
			router := new(input.Router)
			ops := new(op.Ops)
			gtx := layout.Context{
				Source: router.Source(), Ops: ops, Now: time.Now(),
				Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
				Constraints: layout.Constraints{Max: image.Pt(width, 900)},
			}
			app.layoutWordRibbon(gtx)
			router.Frame(ops)
			firstLabel := wordToolbarActions(tab)[0].label
			var bounds image.Rectangle
			var visit func([]input.SemanticNode)
			visit = func(nodes []input.SemanticNode) {
				for _, node := range nodes {
					if node.Desc.Description == firstLabel {
						bounds = node.Desc.Bounds
					}
					visit(node.Children)
				}
			}
			visit(router.AppendSemantics(nil))
			if bounds.Empty() || bounds.Min.X != 8 {
				t.Errorf("tab %v at width %d: first command %q bounds = %v; want left edge 8", tab, width, firstLabel, bounds)
			}
		}
	}
}
