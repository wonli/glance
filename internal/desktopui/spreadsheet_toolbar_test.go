package desktopui

import (
	"image"
	"image/color"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

type toolbarTestBook struct{ editable bool }

func (*toolbarTestBook) SheetNames() []string                              { return []string{"Sheet1"} }
func (*toolbarTestBook) Range(string, int, int) ([][]document.Cell, error) { return nil, nil }
func (*toolbarTestBook) SetCellText(string, string, string) error          { return nil }
func (*toolbarTestBook) Save() error                                       { return nil }
func (*toolbarTestBook) Dirty() bool                                       { return false }
func (b *toolbarTestBook) Editable() bool                                  { return b.editable }
func (*toolbarTestBook) Close() error                                      { return nil }
func (*toolbarTestBook) Window(string, int, int, int, int) (document.SheetWindow, error) {
	return document.SheetWindow{}, nil
}
func (*toolbarTestBook) SetRowHeight(string, int, float64) error   { return nil }
func (*toolbarTestBook) SetColumnWidth(string, int, float64) error { return nil }

func TestSpreadsheetToolbarVisibilityRequiresEditableGridReader(t *testing.T) {
	state := newDocumentSessionState("")
	app := &App{documentSessionState: state}
	app.book = &toolbarTestBook{editable: true}
	if !app.spreadsheetToolbarVisible() {
		t.Fatal("editable grid reader should show the toolbar")
	}
	app.book = &toolbarTestBook{editable: false}
	if app.spreadsheetToolbarVisible() {
		t.Fatal("read-only workbook should not show editing controls")
	}
}

func TestSpreadsheetToolbarActionsCoverHomeAndDataGroups(t *testing.T) {
	state := newDocumentSessionState("")
	wantCategories := []string{"文件", "开始", "插入", "页面布局", "公式", "数据", "审阅", "视图"}
	gotCategories := toolbarCategoryLabelsForState(state)
	if !equalStrings(gotCategories, wantCategories) {
		t.Fatalf("Excel categories = %v, want %v", gotCategories, wantCategories)
	}
	wantActions := map[spreadsheetToolbarTab][]string{
		toolbarTabFile:       {"打开", "保存", "另存为", "打印", "导出", "共享"},
		toolbarTabHome:       {"剪切", "复制", "粘贴", "格式刷", "字体", "加粗", "斜体", "下划线", "左对齐", "居中", "右对齐", "自动换行", "百分比", "数字格式", "插入", "删除", "条件格式", "表格格式", "求和", "排序", "筛选", "查找", "撤销", "重做"},
		toolbarTabInsert:     {"表格", "数据透视表", "图片", "形状", "柱状图", "折线图", "饼图", "链接", "文本框"},
		toolbarTabPageLayout: {"页边距", "纸张方向", "纸张大小", "打印区域", "网格线", "标题行"},
		toolbarTabFormula:    {"插入函数", "自动求和", "最近使用", "逻辑函数", "文本函数", "日期时间", "名称管理器", "重新计算"},
		toolbarTabData:       {"刷新全部", "排序", "筛选", "分列", "快速填充", "删除重复项", "数据验证"},
		toolbarTabReview:     {"拼写检查", "翻译", "新建批注", "删除批注", "保护工作表", "保护工作簿"},
		toolbarTabView:       {"普通视图", "分页预览", "标尺", "网格线", "放大", "100%显示", "冻结窗格"},
	}
	for tab, want := range wantActions {
		actions := toolbarActionsForTab(tab)
		got := make([]string, len(actions))
		for i, action := range actions {
			got[i] = action.label
		}
		if !equalStrings(got, want) {
			t.Errorf("tab %q actions = %v, want %v", gotCategories[tab], got, want)
		}
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestSpreadsheetToolbarActionsHaveIconGlyphsAndAccessibleLabels(t *testing.T) {
	var actions []spreadsheetToolbarAction
	for tab := toolbarTabFile; tab <= toolbarTabView; tab++ {
		actions = append(actions, toolbarActionsForTab(tab)...)
	}
	actions = append(actions, pictureToolbarActions...)
	for _, action := range actions {
		if action.icon == "" {
			t.Errorf("toolbar action %q has no icon", action.label)
		}
		if action.label == "" {
			t.Errorf("toolbar action slot %d has no accessible label", action.slot)
		}
		if _, ok := toolbarIconData(action.icon); !ok {
			t.Errorf("toolbar action %q references missing icon %q", action.label, action.icon)
		}
	}
}

func TestPenExcelScreenIconsAreBundled(t *testing.T) {
	for _, name := range []string{"minus", "plus", "menu", "list", "grip-horizontal", "settings-2", "eraser", "house"} {
		if _, err := renderSVGIcon("excel", name, 24, nil); err != nil {
			t.Errorf("Pen Excel icon %q is unavailable: %v", name, err)
		}
	}
}

func TestExcelRibbonActionsUseExcelIconAssets(t *testing.T) {
	for tab := toolbarTabFile; tab <= toolbarTabView; tab++ {
		for _, action := range toolbarActionsForTab(tab) {
			if action.icon == "" {
				t.Errorf("tab %d action %q has no Excel icon", tab, action.label)
				continue
			}
			if _, ok := toolbarIconData(action.icon); !ok {
				t.Errorf("tab %d action %q does not resolve through Excel icon assets: %q", tab, action.label, action.icon)
			}
		}
	}
}

func TestExcelInsertActionClickShowsMissingNotice(t *testing.T) {
	book := &toolbarTestBook{editable: true}
	state := newDocumentSessionState("")
	state.book = book
	state.toolbarTab = toolbarTabInsert
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{
		Source:      router.Source(),
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(800, 600)},
	}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx.Source = router.Source()
		app.handleSpreadsheetToolbarEvents(gtx)
		app.layoutSpreadsheetToolbar(gtx)
		router.Frame(&ops)
	}

	frame()
	point := f32.Pt(74, 58) // 2nd Insert action, "数据透视表" (unimplemented).
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
	frame()
	if want := "“数据透视表”功能暂未实现"; app.status != want {
		t.Fatalf("status after clicking an unavailable action = %q, want %q", app.status, want)
	}
}

func TestExcelReviewActionClickShowsMissingNotice(t *testing.T) {
	book := &toolbarTestBook{editable: true}
	state := newDocumentSessionState("")
	state.book = book
	state.toolbarTab = toolbarTabReview
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{
		Source:      router.Source(),
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(800, 600)},
	}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx.Source = router.Source()
		app.handleSpreadsheetToolbarEvents(gtx)
		app.layoutSpreadsheetToolbar(gtx)
		router.Frame(&ops)
	}

	frame()
	point := f32.Pt(34, 58) // 1st Review action, "拼写检查" (unimplemented).
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
	frame()
	if want := "“拼写检查”功能暂未实现"; app.status != want {
		t.Fatalf("status after clicking Excel Review action = %q, want %q", app.status, want)
	}
}


func TestUnimplementedExcelTabsHaveMissingFeatureMessages(t *testing.T) {
	for _, tab := range []spreadsheetToolbarTab{toolbarTabFile, toolbarTabHome, toolbarTabInsert, toolbarTabPageLayout, toolbarTabFormula, toolbarTabData, toolbarTabReview, toolbarTabView} {
		if name, ok := unimplementedSpreadsheetToolbarTab(tab); ok {
			t.Errorf("tab %d reports its category missing (%q) even though its buttons are listed", tab, name)
		}
	}
}

func TestToolbarIconTintWritesRequestedColorIntoPixels(t *testing.T) {
	mask := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	mask.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 85})
	want := color.NRGBA{R: 0, G: 153, B: 255, A: 85}

	tinted := tintToolbarIcon(mask, color.NRGBA{R: want.R, G: want.G, B: want.B, A: 255})
	got := color.NRGBAModel.Convert(tinted.At(0, 0)).(color.NRGBA)
	if got != want {
		t.Fatalf("tinted icon pixel=%v; want %v", got, want)
	}
}

func TestToolbarIconsUseOfficeStyleMonochromeColors(t *testing.T) {
	enabledColor := color.NRGBA{R: 75, G: 83, B: 94, A: 255}
	checks := []struct {
		slot int
		icon string
	}{
		{slot: 0, icon: "undo"},
		{slot: 2, icon: "copy"},
		{slot: 8, icon: "paint-bucket"},
		{slot: 13, icon: "row-delete"},
	}
	for _, check := range checks {
		if got := toolbarIconColor(true); got != enabledColor {
			t.Errorf("enabled toolbar icon color for slot %d=%v; want %v", check.slot, got, enabledColor)
			continue
		}
		icon, ok := tintedToolbarIcon(check.icon, enabledColor)
		if !ok {
			t.Errorf("toolbar icon %q was not loaded", check.icon)
			continue
		}
		near := func(a, b uint8) bool {
			if a > b {
				return a-b <= 6
			}
			return b-a <= 6
		}
		foundGlyph := false
		for y := icon.Bounds().Min.Y; y < icon.Bounds().Max.Y && !foundGlyph; y++ {
			for x := icon.Bounds().Min.X; x < icon.Bounds().Max.X; x++ {
				pixel := color.NRGBAModel.Convert(icon.At(x, y)).(color.NRGBA)
				if pixel.A == 0 {
					continue
				}
				foundGlyph = true
				if !near(pixel.R, enabledColor.R) || !near(pixel.G, enabledColor.G) || !near(pixel.B, enabledColor.B) {
					t.Errorf("toolbar icon %q pixel RGB=(%d,%d,%d); want (%d,%d,%d)", check.icon, pixel.R, pixel.G, pixel.B, enabledColor.R, enabledColor.G, enabledColor.B)
				}
				break
			}
		}
		if !foundGlyph {
			t.Errorf("toolbar icon %q has no visible pixels", check.icon)
		}
	}
	if got := toolbarIconColor(false); got != disabledToolbarText || int(got.B)-int(got.R) > 20 {
		t.Errorf("disabled toolbar icon should be light gray, got %v", got)
	}
}
func TestToolbarPickerPanelTracksItsScrolledActionAndStaysInViewport(t *testing.T) {
	actions := make([]spreadsheetToolbarAction, 18)
	for i := range actions {
		actions[i] = spreadsheetToolbarAction{slot: i, group: "字体"}
	}
	for i := 8; i < len(actions); i++ {
		actions[i].group = "格式"
	}
	position := layout.Position{First: 7, Offset: 8}
	if got := toolbarPickerPanelX(10, actions, position, 40, 48, 800, 260); got != 120 {
		t.Fatalf("panel x=%d; want 120px under the scrolled action", got)
	}
	if got := toolbarPickerPanelX(17, actions, layout.Position{}, 40, 48, 300, 260); got != 40 {
		t.Fatalf("panel x=%d; want 40px after clamping to the viewport", got)
	}
}

func TestSpreadsheetToolbarHeightDoesNotChangeWhenPickerOpens(t *testing.T) {
	book := &toolbarTestBook{editable: true}
	state := newDocumentSessionState("")
	state.book = book
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	app.book = book

	heightForPicker := func(picker spreadsheetToolbarPicker) int {
		state.toolbarPicker = picker
		var ops op.Ops
		gtx := layout.Context{
			Ops:         &ops,
			Constraints: layout.Constraints{Max: image.Pt(800, 600)},
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		}
		return app.layoutSpreadsheetToolbar(gtx).Size.Y
	}

	if closed, open := heightForPicker(pickerNone), heightForPicker(pickerAlignment); closed != open {
		t.Fatalf("toolbar height closed=%d open=%d; submenu must float without resizing it", closed, open)
	}
}

func TestToolbarStyleUsesActiveCellAndMarksMixedSelection(t *testing.T) {
	state := newDocumentSessionState("")
	state.sheet = "Sheet1"
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 1}}
	state.selectionSet = true
	state.virtualGrid = newGridViewport()
	state.virtualGrid.WindowSheet = "Sheet1"
	state.virtualGrid.WindowRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2}
	state.virtualGrid.Window = document.SheetWindow{StartRow: 1, StartColumn: 1, Cells: [][]document.Cell{{{Style: document.CellStyle{Bold: false, Italic: true}}, {Style: document.CellStyle{Bold: true, Italic: true}}}}}
	app := &App{documentSessionState: state}
	if got := app.selectedCellStyle(); !got.Bold {
		t.Fatal("toolbar should reflect the active cell's bold style")
	}
	if !app.selectionFormatMixed(4) {
		t.Fatal("bold should be marked mixed across the selection")
	}
	if app.selectionFormatMixed(5) {
		t.Fatal("uniform italic should not be marked mixed")
	}
}

type formattingTestBook struct {
	toolbarTestBook
	lastPatch document.CellFormatPatch
	lastSheet string
	lastRange document.CellRange
}

func (f *formattingTestBook) SetCellFormats(sheet string, bounds document.CellRange, patch document.CellFormatPatch) error {
	f.lastSheet = sheet
	f.lastRange = bounds
	f.lastPatch = patch
	return nil
}

func TestSpreadsheetToolbarClickAppliesFormat(t *testing.T) {
	fake := &formattingTestBook{toolbarTestBook: toolbarTestBook{editable: true}}
	state := newDocumentSessionState("")
	state.book = fake
	state.sheet = "Sheet1"
	state.selectionSet = true
	state.selection = CellSelection{Active: CellPosition{Row: 0, Column: 0}, Anchor: CellPosition{Row: 0, Column: 0}}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionWrapText) {
		t.Fatal("toolbarActionWrapText should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionAlignLeft) {
		t.Fatal("toolbarActionAlignLeft should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionAlignRight) {
		t.Fatal("toolbarActionAlignRight should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionPercentFormat) {
		t.Fatal("toolbarActionPercentFormat should be enabled")
	}

	patch, ok := formatPatchForToolbarAction(toolbarActionWrapText, document.CellStyle{})
	if !ok || patch.WrapText == nil || !*patch.WrapText {
		t.Fatalf("unexpected patch: %+v", patch)
	}
	app.applyCellFormat(patch)

	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("applyCellFormat error: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for formatResult")
	}

	if fake.lastPatch.WrapText == nil || !*fake.lastPatch.WrapText {
		t.Fatalf("formatter did not receive WrapText patch: %+v", fake.lastPatch)
	}
}

func TestSpreadsheetFormatPainterTogglesAndCancels(t *testing.T) {
	fake := &formattingTestBook{toolbarTestBook: toolbarTestBook{editable: true}}
	state := newDocumentSessionState("")
	state.book = fake
	state.sheet = "Sheet1"
	state.selectionSet = true
	state.selection = CellSelection{Active: CellPosition{Row: 0, Column: 0}, Anchor: CellPosition{Row: 0, Column: 0}}
	state.virtualGrid = &gridViewport{
		Window: document.SheetWindow{
			StartRow:    1,
			StartColumn: 1,
			Cells: [][]document.Cell{
				{{Style: document.CellStyle{Bold: true, FontFamily: "Arial", FontSize: 12}}},
			},
		},
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionFormatPainter) {
		t.Fatal("format painter should be enabled when selection is set")
	}

	app.toggleSpreadsheetFormatPainter()
	if !state.excelFormatPainterActive || state.excelFormatPainterPatch == nil {
		t.Fatal("format painter should be active after toggle")
	}
	if state.excelFormatPainterPatch.Bold == nil || !*state.excelFormatPainterPatch.Bold {
		t.Fatalf("captured patch should have Bold=true, got %+v", state.excelFormatPainterPatch)
	}

	app.cancelSpreadsheetFormatPainter()
	if state.excelFormatPainterActive || state.excelFormatPainterPatch != nil {
		t.Fatal("format painter should be inactive after cancel")
	}

	app.toggleSpreadsheetFormatPainter()
	if !state.excelFormatPainterActive {
		t.Fatal("format painter should be active again")
	}
	app.toggleSpreadsheetFormatPainter()
	if state.excelFormatPainterActive {
		t.Fatal("format painter should be inactive after second toggle")
	}
}

func TestSpreadsheetFormatPainterAppliesToTargetSelection(t *testing.T) {
	fake := &formattingTestBook{toolbarTestBook: toolbarTestBook{editable: true}}
	state := newDocumentSessionState("")
	state.book = fake
	state.sheet = "Sheet1"
	state.selectionSet = true
	state.selection = CellSelection{Active: CellPosition{Row: 0, Column: 0}, Anchor: CellPosition{Row: 0, Column: 0}}
	state.virtualGrid = &gridViewport{
		Window: document.SheetWindow{
			StartRow:    1,
			StartColumn: 1,
			Cells: [][]document.Cell{
				{
					{Style: document.CellStyle{Bold: true, Italic: true, FontFamily: "Consolas", FontSize: 14}},
					{Style: document.CellStyle{}},
				},
				{
					{Style: document.CellStyle{}},
					{Style: document.CellStyle{}},
				},
			},
		},
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	app.toggleSpreadsheetFormatPainter()
	if !state.excelFormatPainterActive {
		t.Fatal("format painter should be active")
	}

	// Move selection to target B2 (Row 1, Column 1)
	state.selection = CellSelection{Active: CellPosition{Row: 1, Column: 1}, Anchor: CellPosition{Row: 1, Column: 1}}
	app.applySpreadsheetFormatPainter()

	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("applySpreadsheetFormatPainter error: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for formatResult")
	}

	if state.excelFormatPainterActive {
		t.Fatal("format painter should be deactivated after application")
	}
	if fake.lastRange != (document.CellRange{StartRow: 2, StartColumn: 2, EndRow: 2, EndColumn: 2}) {
		t.Fatalf("target range mismatch: got %+v", fake.lastRange)
	}
	if fake.lastPatch.Bold == nil || !*fake.lastPatch.Bold {
		t.Fatalf("target patch bold want true, got %+v", fake.lastPatch.Bold)
	}
	if fake.lastPatch.FontFamily == nil || *fake.lastPatch.FontFamily != "Consolas" {
		t.Fatalf("target patch font family want Consolas, got %+v", fake.lastPatch.FontFamily)
	}
}

func TestSpreadsheetFontFamilyDialog(t *testing.T) {
	fake := &formattingTestBook{toolbarTestBook: toolbarTestBook{editable: true}}
	state := newDocumentSessionState("")
	state.book = fake
	state.sheet = "Sheet1"
	state.selectionSet = true
	state.selection = CellSelection{Active: CellPosition{Row: 0, Column: 0}, Anchor: CellPosition{Row: 0, Column: 0}}
	state.virtualGrid = &gridViewport{
		Window: document.SheetWindow{
			StartRow:    1,
			StartColumn: 1,
			Cells: [][]document.Cell{
				{{Style: document.CellStyle{FontFamily: "宋体"}}},
			},
		},
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionFontFamily) {
		t.Fatal("font family action should be enabled")
	}

	app.showSpreadsheetFontFamilyDialog()
	if !app.modal.Visible {
		t.Fatal("font family modal dialog should be visible")
	}

	app.modal.Input.SetText("Microsoft YaHei")
	app.modal.OnAction("confirm")

	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("applyCellFormat error: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for formatResult")
	}

	if fake.lastPatch.FontFamily == nil || *fake.lastPatch.FontFamily != "Microsoft YaHei" {
		t.Fatalf("expected FontFamily 'Microsoft YaHei', got %+v", fake.lastPatch)
	}
}

func TestSpreadsheetZoomInAndZoom100(t *testing.T) {
	fake := &formattingTestBook{toolbarTestBook: toolbarTestBook{editable: true}}
	state := newDocumentSessionState("")
	state.book = fake
	state.sheet = "Sheet1"
	v := newGridViewport()
	state.virtualGrid = v
	app := &App{documentSessionState: state, theme: material.NewTheme()}

	if !app.toolbarActionEnabled(toolbarActionZoomIn) {
		t.Fatal("zoom-in should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionZoom100) {
		t.Fatal("zoom-100 should be enabled")
	}

	// Trigger zoom-in
	newScale := v.Scale + 0.15
	if newScale > 2.5 {
		newScale = 2.5
	}
	v.setScale(newScale)
	if v.Scale != 1.15 {
		t.Fatalf("expected scale 1.15, got %v", v.Scale)
	}

	// Trigger zoom-100
	v.setScale(1.0)
	if v.Scale != 1.0 {
		t.Fatalf("expected scale 1.0, got %v", v.Scale)
	}
}

func TestSpreadsheetPageLayoutDialogsAndCommands(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = wb.SheetNames()[0]
	v := newGridViewport()
	state.virtualGrid = v
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionLayoutMargins) {
		t.Fatal("layout-margins should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionLayoutOrientation) {
		t.Fatal("layout-orientation should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionLayoutSize) {
		t.Fatal("layout-size should be enabled")
	}

	// 1. Orientation Dialog
	app.showSpreadsheetOrientationDialog()
	if !app.modal.Visible || app.modal.Title != "纸张方向" {
		t.Fatalf("expected modal dialog for orientation, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("landscape")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("orientation command failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for orientation formatResult")
	}
	pageLayout, err := wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.Orientation != "landscape" {
		t.Fatalf("expected landscape orientation, got %+v (err=%v)", pageLayout, err)
	}

	// 2. Margins Dialog
	app.showSpreadsheetMarginsDialog()
	if !app.modal.Visible || app.modal.Title != "页边距设置" {
		t.Fatalf("expected modal dialog for margins, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("narrow")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("margins command failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for margins formatResult")
	}
	pageLayout, err = wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.MarginLeft != 0.25 {
		t.Fatalf("expected narrow margin 0.25, got %+v (err=%v)", pageLayout, err)
	}

	// 3. Page Size Dialog
	app.showSpreadsheetPageSizeDialog()
	if !app.modal.Visible || app.modal.Title != "纸张大小设置" {
		t.Fatalf("expected modal dialog for page size, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("letter")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("page size command failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for page size formatResult")
	}
	pageLayout, err = wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.PaperSize != 1 {
		t.Fatalf("expected paper size 1 (Letter), got %+v (err=%v)", pageLayout, err)
	}

	// 4. Print Area Dialog and Commands
	if !app.toolbarActionEnabled(toolbarActionPrintArea) {
		t.Fatal("print-area should be enabled")
	}
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 9, Column: 3},
	}
	app.showSpreadsheetPrintAreaDialog()
	if !app.modal.Visible || app.modal.Title != "打印区域设置" {
		t.Fatalf("expected modal dialog for print area, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("set")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("set print area failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for print area formatResult")
	}
	pageLayout, err = wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.PrintArea != "$A$1:$D$10" {
		t.Fatalf("expected print area $A$1:$D$10, got %+v (err=%v)", pageLayout, err)
	}

	// Clear print area
	app.showSpreadsheetPrintAreaDialog()
	if !app.modal.Visible {
		t.Fatal("expected modal dialog for print area")
	}
	app.modal.OnAction("clear")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("clear print area failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for clear print area formatResult")
	}
	pageLayout, err = wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.PrintArea != "" {
		t.Fatalf("expected empty print area after clear, got %+v (err=%v)", pageLayout, err)
	}

	// 5. Print Titles / Header Row Dialog
	if !app.toolbarActionEnabled(toolbarActionHeaderRow) {
		t.Fatal("header-row / print-titles should be enabled")
	}
	app.showSpreadsheetPrintTitlesDialog()
	if !app.modal.Visible || app.modal.Title != "打印标题设置" {
		t.Fatalf("expected modal dialog for print titles, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("set")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("set print titles failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for print titles formatResult")
	}
	pageLayout, err = wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.PrintTitles != "$1:$10" {
		t.Fatalf("expected print titles $1:$10, got %+v (err=%v)", pageLayout, err)
	}

	// Clear print titles
	app.showSpreadsheetPrintTitlesDialog()
	if !app.modal.Visible {
		t.Fatal("expected modal dialog for print titles")
	}
	app.modal.OnAction("clear")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("clear print titles failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for clear print titles formatResult")
	}
	pageLayout, err = wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.PrintTitles != "" {
		t.Fatalf("expected empty print titles after clear, got %+v (err=%v)", pageLayout, err)
	}

	// 6. View Gridlines Toggle
	if !app.toolbarActionEnabled(toolbarActionViewGridlines) {
		t.Fatal("view-gridlines should be enabled")
	}
	if state.spreadsheetGridlinesHidden {
		t.Fatal("expected gridlines initially visible")
	}
	state.toolbarTab = toolbarTabView
	state.toolbarActionButtons[toolbarActionViewGridlines].Click()
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	app.handleSpreadsheetToolbarEvents(gtx)
	if !state.spreadsheetGridlinesHidden {
		t.Fatal("expected gridlines to be hidden after toggle")
	}
	if app.status != "已隐藏网格线" {
		t.Fatalf("expected status '已隐藏网格线', got %q", app.status)
	}
	state.toolbarActionButtons[toolbarActionViewGridlines].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	if state.spreadsheetGridlinesHidden {
		t.Fatal("expected gridlines to be visible after second toggle")
	}
	if app.status != "已显示网格线" {
		t.Fatalf("expected status '已显示网格线', got %q", app.status)
	}

	// 7. Layout Gridlines Toggle
	if !app.toolbarActionEnabled(toolbarActionLayoutGridlines) {
		t.Fatal("layout-gridlines should be enabled")
	}
	state.toolbarTab = toolbarTabPageLayout
	state.toolbarActionButtons[toolbarActionLayoutGridlines].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	if !state.spreadsheetGridlinesHidden {
		t.Fatal("expected gridlines to be hidden after layout gridlines toggle")
	}
	gridlines, err := wb.GetSheetGridlines(state.sheet)
	if err != nil || gridlines {
		t.Fatalf("expected workbook gridlines to be false, got %v (err=%v)", gridlines, err)
	}
	state.toolbarActionButtons[toolbarActionLayoutGridlines].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	if state.spreadsheetGridlinesHidden {
		t.Fatal("expected gridlines to be visible after second layout toggle")
	}
	gridlines, err = wb.GetSheetGridlines(state.sheet)
	if err != nil || !gridlines {
		t.Fatalf("expected workbook gridlines to be true, got %v (err=%v)", gridlines, err)
	}
}

func TestSpreadsheetLinkDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = wb.SheetNames()[0]
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 1, Column: 1},
		Active: CellPosition{Row: 1, Column: 1}, // B2
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionInsertLink) {
		t.Fatal("toolbarActionInsertLink should be enabled")
	}

	// 1. Show link dialog on empty cell B2
	app.showSpreadsheetLinkDialog()
	if !app.modal.Visible || app.modal.Title != "插入链接" {
		t.Fatalf("expected modal dialog for link, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("https://example.com")
	app.modal.SecondInput.SetText("Example")
	app.modal.OnAction("confirm")

	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert link failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert link formatResult")
	}

	link, ok, err := wb.GetCellHyperlink(state.sheet, "B2")
	if err != nil || !ok || link.Target != "https://example.com" {
		t.Fatalf("expected hyperlink https://example.com on B2, got ok=%v target=%q (err=%v)", ok, link.Target, err)
	}

	// 2. Reopen link dialog on B2 (now has link) and remove link
	app.showSpreadsheetLinkDialog()
	if !app.modal.Visible {
		t.Fatal("expected modal dialog for link")
	}
	app.modal.OnAction("remove")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("remove link failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for remove link formatResult")
	}

	link, ok, err = wb.GetCellHyperlink(state.sheet, "B2")
	if err != nil || ok {
		t.Fatalf("expected no hyperlink after remove, got ok=%v (err=%v)", ok, err)
	}
}

func TestSpreadsheetFreezePanesDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = wb.SheetNames()[0]
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 2, Column: 1},
		Active: CellPosition{Row: 2, Column: 1}, // B3
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionFreezePanes) {
		t.Fatal("toolbarActionFreezePanes should be enabled")
	}

	// 1. Freeze top row
	app.showSpreadsheetFreezePanesDialog()
	if !app.modal.Visible || app.modal.Title != "冻结窗格" {
		t.Fatalf("expected modal dialog for freeze panes, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("freeze_row")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("freeze top row failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for freeze top row formatResult")
	}

	panes, err := wb.GetSheetPanes(state.sheet)
	if err != nil || !panes.Frozen || panes.YSplit != 1 || panes.XSplit != 0 {
		t.Fatalf("expected frozen top row (YSplit=1, XSplit=0), got %+v (err=%v)", panes, err)
	}

	// 2. Unfreeze panes
	app.showSpreadsheetFreezePanesDialog()
	if !app.modal.Visible {
		t.Fatal("expected modal dialog for freeze panes")
	}
	app.modal.OnAction("unfreeze")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("unfreeze panes failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for unfreeze formatResult")
	}

	panes, err = wb.GetSheetPanes(state.sheet)
	if err != nil || panes.Frozen {
		t.Fatalf("expected unfrozen panes, got %+v (err=%v)", panes, err)
	}
}

func TestSpreadsheetRemoveDuplicatesDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	_ = wb.SetCellText(sheet, "A1", "Alice")
	_ = wb.SetCellText(sheet, "B1", "100")
	_ = wb.SetCellText(sheet, "A2", "Bob")
	_ = wb.SetCellText(sheet, "B2", "200")
	_ = wb.SetCellText(sheet, "A3", "Alice")
	_ = wb.SetCellText(sheet, "B3", "100") // exact duplicate of row 1

	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = sheet
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 2, Column: 1}, // A1:B3 (3 rows, 2 columns)
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionRemoveDuplicates) {
		t.Fatal("toolbarActionRemoveDuplicates should be enabled")
	}

	app.showSpreadsheetRemoveDuplicatesDialog()
	if !app.modal.Visible || app.modal.Title != "删除重复项" {
		t.Fatalf("expected modal dialog for remove duplicates, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}

	// Deduplicate across all columns
	app.modal.OnAction("all")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("remove duplicates failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for remove duplicates formatResult")
	}

	// Verify row 3 was removed, leaving 2 rows
	cells, err := wb.Range(sheet, 10, 10)
	if err != nil {
		t.Fatalf("failed to read range: %v", err)
	}
	var rowCount int
	for _, row := range cells {
		if len(row) > 0 && row[0].Value != "" {
			rowCount++
		}
	}
	if rowCount != 2 {
		t.Fatalf("expected 2 remaining rows, got %d", rowCount)
	}

	// Verify Undo restores row 3
	if applied, err := wb.Undo(); err != nil || !applied {
		t.Fatalf("Undo failed: applied=%v, err=%v", applied, err)
	}
	cells, err = wb.Range(sheet, 10, 10)
	if err != nil {
		t.Fatalf("failed to read range: %v", err)
	}
	rowCount = 0
	for _, row := range cells {
		if len(row) > 0 && row[0].Value != "" {
			rowCount++
		}
	}
	if rowCount != 3 {
		t.Fatalf("expected 3 rows restored after Undo, got %d", rowCount)
	}
}

func TestSpreadsheetTextToColumnsDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]

	_ = wb.SetCellText(sheet, "A1", "Alpha,Beta,Gamma")
	_ = wb.SetCellText(sheet, "A2", "One,Two,Three")

	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 1, Column: 0}, // A1:A2
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionTextToColumns) {
		t.Fatal("toolbarActionTextToColumns should be enabled")
	}

	app.showSpreadsheetTextToColumnsDialog()
	if !app.modal.Visible || app.modal.Title != "文本分列" {
		t.Fatalf("expected modal dialog for text to columns, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}

	// Split by comma
	app.modal.OnAction("comma")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("split text to columns failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for text to columns formatResult")
	}

	// Verify values in A1..C2
	vA1, _ := wb.Value(sheet, "A1")
	vB1, _ := wb.Value(sheet, "B1")
	vC1, _ := wb.Value(sheet, "C1")
	if vA1 != "Alpha" || vB1 != "Beta" || vC1 != "Gamma" {
		t.Fatalf("unexpected row 1 split: %q, %q, %q", vA1, vB1, vC1)
	}
	vA2, _ := wb.Value(sheet, "A2")
	vB2, _ := wb.Value(sheet, "B2")
	vC2, _ := wb.Value(sheet, "C2")
	if vA2 != "One" || vB2 != "Two" || vC2 != "Three" {
		t.Fatalf("unexpected row 2 split: %q, %q, %q", vA2, vB2, vC2)
	}
}

func TestSpreadsheetInsertTableDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]

	_ = wb.SetCellText(sheet, "A1", "Name")
	_ = wb.SetCellText(sheet, "B1", "Score")
	_ = wb.SetCellText(sheet, "A2", "Alice")
	_ = wb.SetCellText(sheet, "B2", "100")

	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 1, Column: 1}, // A1:B2
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionInsertTable) {
		t.Fatal("toolbarActionInsertTable should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionTableFormat) {
		t.Fatal("toolbarActionTableFormat should be enabled")
	}

	app.showSpreadsheetInsertTableDialog("")
	if !app.modal.Visible || app.modal.Title != "创建表格" {
		t.Fatalf("expected modal dialog for insert table, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}

	app.modal.OnAction("medium")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("create table failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for table formatResult")
	}

	tables, err := wb.Range(sheet, 5, 5)
	if err != nil {
		t.Fatalf("failed to read range: %v", err)
	}
	if len(tables) < 2 {
		t.Fatalf("expected at least 2 rows in range, got %d", len(tables))
	}
}



func TestSpreadsheetViewModesAndRuler(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = sheet
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionNormalView) {
		t.Fatal("normal view should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionPageBreakPreview) {
		t.Fatal("page break preview should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionRuler) {
		t.Fatal("ruler should be enabled")
	}

	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}

	// 1. Switch to Page Break Preview
	state.toolbarTab = toolbarTabView
	state.toolbarActionButtons[toolbarActionPageBreakPreview].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("set page break preview failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for page break preview formatResult")
	}
	if state.spreadsheetViewMode != "pageBreakPreview" {
		t.Fatalf("expected state view mode pageBreakPreview, got %q", state.spreadsheetViewMode)
	}
	view, err := wb.GetSheetView(sheet)
	if err != nil || view.ViewMode != "pageBreakPreview" {
		t.Fatalf("expected workbook view mode pageBreakPreview, got %+v (err=%v)", view, err)
	}

	// 2. Switch back to Normal View
	state.toolbarActionButtons[toolbarActionNormalView].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("set normal view failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for normal view formatResult")
	}
	if state.spreadsheetViewMode != "normal" {
		t.Fatalf("expected state view mode normal, got %q", state.spreadsheetViewMode)
	}
	view, err = wb.GetSheetView(sheet)
	if err != nil || view.ViewMode != "normal" {
		t.Fatalf("expected workbook view mode normal, got %+v (err=%v)", view, err)
	}

	// 3. Toggle Ruler
	initRuler := state.spreadsheetRulerVisible
	state.toolbarActionButtons[toolbarActionRuler].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("toggle ruler failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ruler formatResult")
	}
	if state.spreadsheetRulerVisible == initRuler {
		t.Fatal("expected ruler visibility to toggle")
	}
	view, err = wb.GetSheetView(sheet)
	if err != nil || view.ShowRuler != state.spreadsheetRulerVisible {
		t.Fatalf("expected workbook ruler %v, got %+v (err=%v)", state.spreadsheetRulerVisible, view, err)
	}
}

func TestSpreadsheetProtectSheetDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = sheet
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionProtectSheet) {
		t.Fatal("protect sheet should be enabled")
	}

	// 1. Show protect sheet dialog (currently unprotected)
	app.showSpreadsheetProtectSheetDialog()
	if !app.modal.Visible || app.modal.Title != "保护工作表" {
		t.Fatalf("expected protect sheet modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("testpass")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("protect sheet failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for protect sheet formatResult")
	}

	protected, err := wb.GetSheetProtection(sheet)
	if err != nil || !protected {
		t.Fatalf("expected sheet protected, got %v (err=%v)", protected, err)
	}

	// 2. Show unprotect sheet dialog (currently protected)
	app.showSpreadsheetProtectSheetDialog()
	if !app.modal.Visible || app.modal.Title != "撤销保护工作表" {
		t.Fatalf("expected unprotect sheet modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("testpass")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("unprotect sheet failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for unprotect sheet formatResult")
	}

	protected, err = wb.GetSheetProtection(sheet)
	if err != nil || protected {
		t.Fatalf("expected sheet unprotected, got %v (err=%v)", protected, err)
	}

	// 3. Test Undo restores protection
	applied, err := wb.Undo()
	if err != nil || !applied {
		t.Fatalf("undo failed: applied=%v, err=%v", applied, err)
	}
	protected, err = wb.GetSheetProtection(sheet)
	if err != nil || !protected {
		t.Fatalf("expected sheet protected after undo, got %v (err=%v)", protected, err)
	}
}

func TestSpreadsheetProtectWorkbookDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = wb.SheetNames()[0]
	v := newGridViewport()
	state.virtualGrid = v
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionProtectWorkbook) {
		t.Fatal("protect-workbook should be enabled")
	}

	// 1. Show protect workbook dialog (currently unprotected)
	app.showSpreadsheetProtectWorkbookDialog()
	if !app.modal.Visible || app.modal.Title != "保护工作簿" {
		t.Fatalf("expected protect workbook modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("bookpass")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("protect workbook failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for protect workbook formatResult")
	}

	protected, err := wb.GetWorkbookProtection()
	if err != nil || !protected {
		t.Fatalf("expected workbook protected, got %v (err=%v)", protected, err)
	}

	// 2. Show unprotect workbook dialog (currently protected)
	app.showSpreadsheetProtectWorkbookDialog()
	if !app.modal.Visible || app.modal.Title != "撤销保护工作簿" {
		t.Fatalf("expected unprotect workbook modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("bookpass")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("unprotect workbook failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for unprotect workbook formatResult")
	}

	protected, err = wb.GetWorkbookProtection()
	if err != nil || protected {
		t.Fatalf("expected workbook unprotected, got %v (err=%v)", protected, err)
	}

	// 3. Test Undo restores protection
	applied, err := wb.Undo()
	if err != nil || !applied {
		t.Fatalf("undo failed: applied=%v, err=%v", applied, err)
	}
	protected, err = wb.GetWorkbookProtection()
	if err != nil || !protected {
		t.Fatalf("expected workbook protected after undo, got %v (err=%v)", protected, err)
	}
}

func TestSpreadsheetRecalculateAndRefreshAll(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]

	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.virtualGrid = newGridViewport()
	app := &App{documentSessionState: state, theme: material.NewTheme()}

	if !app.toolbarActionEnabled(toolbarActionRecalculate) {
		t.Fatal("toolbarActionRecalculate should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionRefreshAll) {
		t.Fatal("toolbarActionRefreshAll should be enabled")
	}

	app.recalculateSpreadsheet()
	if app.status != "已重新计算工作簿公式" {
		t.Fatalf("unexpected status after recalculate: %q", app.status)
	}

	app.refreshAllSpreadsheet()
	if app.status != "已刷新全部工作表数据" {
		t.Fatalf("unexpected status after refresh all: %q", app.status)
	}
}

func TestSpreadsheetFormulaAndNameManagerAndFlashFill(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]

	_ = wb.SetCellText(sheet, "A1", "10")
	_ = wb.SetCellText(sheet, "A2", "20")

	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.virtualGrid = newGridViewport()
	state.selection = CellSelection{Active: CellPosition{Row: 2, Column: 0}}
	state.selectionSet = true
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	// 1. Verify toolbar actions are implemented and enabled
	for _, slot := range []int{
		toolbarActionInsertFunction, toolbarActionRecentFunctions,
		toolbarActionLogicalFunctions, toolbarActionTextFunctions,
		toolbarActionDateTimeFunctions, toolbarActionNameManager,
		toolbarActionFlashFill,
	} {
		if unimplementedSpreadsheetToolbarAction(slot) {
			t.Fatalf("slot %d should not be reported as unimplemented", slot)
		}
		if !app.toolbarActionEnabled(slot) {
			t.Fatalf("slot %d should be enabled", slot)
		}
	}

	// 2. Insert Formula Dialog and execution
	app.showSpreadsheetInsertFunctionDialog("")
	if !app.modal.Visible || app.modal.Title != "插入函数" {
		t.Fatalf("expected 插入函数 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("=SUM(A1:A2)")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert formula failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert formula formatResult")
	}

	formula, err := wb.GetCellFormula(sheet, "A3")
	if err != nil || formula != "SUM(A1:A2)" {
		t.Fatalf("expected formula SUM(A1:A2), got %q (err=%v)", formula, err)
	}

	// 3. Name Manager Dialog and execution
	app.showSpreadsheetNameManagerDialog()
	if !app.modal.Visible || app.modal.Title != "名称管理器" {
		t.Fatalf("expected 名称管理器 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("TestTotal")
	app.modal.OnAction("confirm")

	names, err := wb.GetDefinedNames()
	if err != nil {
		t.Fatalf("GetDefinedNames failed: %v", err)
	}
	found := false
	for _, n := range names {
		if n.Name == "TestTotal" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected defined name TestTotal in %v", names)
	}

	// 4. Flash Fill execution
	_ = wb.SetCellText(sheet, "B1", "1")
	_ = wb.SetCellText(sheet, "B2", "2")
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 1},
		Active: CellPosition{Row: 4, Column: 1},
	}
	app.applySpreadsheetFlashFill()
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("flash fill failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for flash fill formatResult")
	}

	vB3, _ := wb.Value(sheet, "B3")
	vB4, _ := wb.Value(sheet, "B4")
	vB5, _ := wb.Value(sheet, "B5")
	if vB3 != "3" || vB4 != "4" || vB5 != "5" {
		t.Fatalf("unexpected flash fill values: B3=%s, B4=%s, B5=%s", vB3, vB4, vB5)
	}
}

func TestSpreadsheetToolbar_DataValidationAndComments(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.virtualGrid = newGridViewport()
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 4, Column: 0},
	}
	state.selectionSet = true
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	// 1. Verify actions are implemented and enabled
	for _, slot := range []int{
		toolbarActionDataValidation,
		toolbarActionNewComment,
		toolbarActionDeleteComment,
	} {
		if unimplementedSpreadsheetToolbarAction(slot) {
			t.Fatalf("slot %d should not be reported as unimplemented", slot)
		}
		if !app.toolbarActionEnabled(slot) {
			t.Fatalf("slot %d should be enabled", slot)
		}
	}

	// 2. Data Validation Dialog & Execution
	app.showSpreadsheetDataValidationDialog()
	if !app.modal.Visible || app.modal.Title != "数据验证" {
		t.Fatalf("expected 数据验证 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("选项A,选项B,选项C")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("apply data validation failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for data validation formatResult")
	}

	// 3. New Comment Dialog & Execution on A1
	state.selection = CellSelection{Active: CellPosition{Row: 0, Column: 0}}
	app.showSpreadsheetNewCommentDialog()
	if !app.modal.Visible || app.modal.Title != "新建批注" {
		t.Fatalf("expected 新建批注 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("这是一条测试批注")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("apply comment failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for comment formatResult")
	}

	comments, err := wb.GetComments(sheet)
	if err != nil {
		t.Fatalf("GetComments failed: %v", err)
	}
	if len(comments) != 1 || comments[0].Cell != "A1" || comments[0].Text != "这是一条测试批注" {
		t.Fatalf("unexpected comments: %+v", comments)
	}

	// 4. Delete Comment Execution on A1
	app.deleteSpreadsheetComment()
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("delete comment failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for delete comment formatResult")
	}

	comments, err = wb.GetComments(sheet)
	if err != nil {
		t.Fatalf("GetComments after delete failed: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("expected 0 comments after delete, got %d", len(comments))
	}
}

func TestSpreadsheetToolbar_ConditionalFormat(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.virtualGrid = newGridViewport()
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 5, Column: 1},
	}
	state.selectionSet = true
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	// 1. Verify availability
	if unimplementedSpreadsheetToolbarAction(toolbarActionConditionalFormat) {
		t.Fatal("toolbarActionConditionalFormat should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionConditionalFormat) {
		t.Fatal("toolbarActionConditionalFormat should be enabled")
	}

	// 2. Open dialog and apply data bar
	app.showSpreadsheetConditionalFormatDialog()
	if !app.modal.Visible || app.modal.Title != "条件格式" {
		t.Fatalf("expected 条件格式 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("data_bar")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("apply conditional format failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for conditional format formatResult")
	}

	// 3. Clear conditional format
	app.showSpreadsheetConditionalFormatDialog()
	app.modal.OnAction("clear")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("clear conditional format failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for clear conditional format formatResult")
	}
}

func TestSpreadsheetToolbar_ShapesAndCharts(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	_ = wb.SetCellText(sheet, "A1", "季度")
	_ = wb.SetCellText(sheet, "B1", "营收")
	_ = wb.SetCellText(sheet, "A2", "Q1")
	_ = wb.SetCellText(sheet, "B2", "100")
	_ = wb.SetCellText(sheet, "A3", "Q2")
	_ = wb.SetCellText(sheet, "B3", "120")

	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.virtualGrid = newGridViewport()
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 2, Column: 1},
	}
	state.selectionSet = true
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	// 1. Shapes
	if unimplementedSpreadsheetToolbarAction(toolbarActionInsertShapes) {
		t.Fatal("toolbarActionInsertShapes should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionInsertShapes) {
		t.Fatal("toolbarActionInsertShapes should be enabled")
	}
	app.showSpreadsheetInsertShapeDialog()
	if !app.modal.Visible || app.modal.Title != "插入形状" {
		t.Fatalf("expected 插入形状 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("roundRect")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert shape failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert shape formatResult")
	}

	// 2. Text Box
	if unimplementedSpreadsheetToolbarAction(toolbarActionInsertTextBox) {
		t.Fatal("toolbarActionInsertTextBox should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionInsertTextBox) {
		t.Fatal("toolbarActionInsertTextBox should be enabled")
	}
	app.showSpreadsheetInsertTextBoxDialog()
	if !app.modal.Visible || app.modal.Title != "插入文本框" {
		t.Fatalf("expected 插入文本框 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("关键指标备注")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert text box failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert text box formatResult")
	}

	// 3. Column Chart
	if unimplementedSpreadsheetToolbarAction(toolbarActionInsertColumnChart) {
		t.Fatal("toolbarActionInsertColumnChart should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionInsertColumnChart) {
		t.Fatal("toolbarActionInsertColumnChart should be enabled")
	}
	app.showSpreadsheetInsertChartDialog(document.SpreadsheetChartColumn)
	if !app.modal.Visible || app.modal.Title != "插入柱状图" {
		t.Fatalf("expected 插入柱状图 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert column chart failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert column chart formatResult")
	}

	// 4. Line Chart
	if unimplementedSpreadsheetToolbarAction(toolbarActionInsertLineChart) {
		t.Fatal("toolbarActionInsertLineChart should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionInsertLineChart) {
		t.Fatal("toolbarActionInsertLineChart should be enabled")
	}
	app.showSpreadsheetInsertChartDialog(document.SpreadsheetChartLine)
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert line chart failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert line chart formatResult")
	}

	// 5. Pie Chart
	if unimplementedSpreadsheetToolbarAction(toolbarActionInsertPieChart) {
		t.Fatal("toolbarActionInsertPieChart should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionInsertPieChart) {
		t.Fatal("toolbarActionInsertPieChart should be enabled")
	}
	app.showSpreadsheetInsertChartDialog(document.SpreadsheetChartPie)
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert pie chart failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert pie chart formatResult")
	}
}
