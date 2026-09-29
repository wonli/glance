package desktopui

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
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
	if len(homeToolbarActions) != 22 || len(dataToolbarActions) != 3 {
		t.Fatalf("toolbar actions home=%d data=%d; want 22 and 3", len(homeToolbarActions), len(dataToolbarActions))
	}
	seen := map[int]bool{}
	for _, action := range append(append([]spreadsheetToolbarAction(nil), homeToolbarActions...), dataToolbarActions...) {
		if action.slot < 0 || action.slot >= 25 || seen[action.slot] {
			t.Fatalf("invalid or duplicate toolbar action slot %d", action.slot)
		}
		seen[action.slot] = true
	}
	for slot := 0; slot < 25; slot++ {
		if !seen[slot] {
			t.Fatalf("toolbar action slot %d is missing", slot)
		}
	}
}

func TestSpreadsheetToolbarActionsHaveIconGlyphsAndAccessibleLabels(t *testing.T) {
	actions := append(append([]spreadsheetToolbarAction(nil), homeToolbarActions...), dataToolbarActions...)
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
	position := layout.Position{First: 7, Offset: 8}
	if got := toolbarPickerPanelX(10, homeToolbarActions, position, 40, 48, 800, 260); got != 120 {
		t.Fatalf("panel x=%d; want 120px under the scrolled action", got)
	}
	if got := toolbarPickerPanelX(17, homeToolbarActions, layout.Position{}, 40, 48, 300, 260); got != 40 {
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
