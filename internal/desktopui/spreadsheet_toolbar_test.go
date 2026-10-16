package desktopui

import (
	"context"
	"image"
	"image/color"

	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"

	"glance/internal/printservice"
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
		toolbarTabFile:       {"打开", "保存", "另存为", "导出", "打印", "共享"},
		toolbarTabHome:       {"剪切", "复制", "粘贴", "格式刷", "字体", "字号", "减小字号", "增大字号", "加粗", "斜体", "下划线", "边框", "填充颜色", "字体颜色", "左对齐", "居中", "右对齐", "自动换行", "百分比", "货币格式", "增加小数位数", "减少小数位数", "数字格式", "插入", "删除", "条件格式", "单元格样式", "格式", "表格格式", "求和", "排序和筛选", "筛选", "查找", "撤销", "重做"},
		toolbarTabInsert:     {"表格", "数据透视表", "图片", "形状", "柱状图", "折线图", "饼图", "链接", "文本框"},
		toolbarTabPageLayout: {"页边距", "纸张方向", "纸张大小", "打印区域", "打印标题", "打印网格线"},
		toolbarTabFormula:    {"插入函数", "自动求和", "最近使用", "逻辑函数", "文本函数", "日期时间", "名称管理器", "重新计算"},
		toolbarTabData:       {"刷新全部", "排序", "筛选", "分列", "快速填充", "删除重复项", "数据验证"},
		toolbarTabReview:     {"拼写检查", "翻译", "新建批注", "删除批注", "保护工作表", "保护工作簿"},
		toolbarTabView:       {"普通视图", "分页符预览", "标尺", "网格线", "放大", "100%显示", "冻结窗格"},
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
		if action.icon == "" && action.symbol == "" {
			t.Errorf("toolbar action %q has neither an icon nor a symbol", action.label)
		}
		if action.label == "" {
			t.Errorf("toolbar action slot %d has no accessible label", action.slot)
		}
		if action.icon != "" {
			if _, ok := toolbarIconData(action.icon); !ok {
				if _, ok := svgIconOriginal("ribbon", action.icon); !ok {
					t.Errorf("toolbar action %q references missing icon %q", action.label, action.icon)
				}
			}
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

func TestExcelRibbonActionsUseBundledIconAssets(t *testing.T) {
	for tab := toolbarTabFile; tab <= toolbarTabView; tab++ {
		for _, action := range toolbarActionsForTab(tab) {
			if action.icon == "" {
				t.Errorf("tab %d action %q has no icon", tab, action.label)
				continue
			}
			if _, ok := toolbarIconData(action.icon); !ok {
				if _, ok := svgIconOriginal("ribbon", action.icon); !ok {
					t.Errorf("tab %d action %q does not resolve through bundled icon assets: %q", tab, action.label, action.icon)
				}
			}
		}
	}
}

func TestExcelMissingNoticeFallback(t *testing.T) {
	app := &App{theme: material.NewTheme()}
	app.showSpreadsheetFeatureMissing("自定义扩展")
	if want := "“自定义扩展”功能暂未实现"; app.status != want {
		t.Fatalf("status after showSpreadsheetFeatureMissing = %q, want %q", app.status, want)
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
	enabledColor := color.NRGBA{A: 255}
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
		WindowRange: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1},
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
		WindowRange: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2},
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
	if !app.fontPicker.open {
		t.Fatal("font picker did not open")
	}
	app.applyFontChoice("Microsoft YaHei")

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

type darwinTestPrinter struct{}

func (p *darwinTestPrinter) Available() bool { return true }
func (p *darwinTestPrinter) Print(ctx context.Context, data []byte, opts printservice.Options) error {
	return nil
}
