package desktopui

import (
	"testing"

	"glance/internal/document"
)

func TestFormatPatchForToolbarActionTogglesAndClears(t *testing.T) {
	for _, item := range []struct {
		slot  int
		style document.CellStyle
		want  *bool
	}{
		{slot: 4, style: document.CellStyle{Bold: false}, want: boolPointer(true)},
		{slot: 5, style: document.CellStyle{Italic: true}, want: boolPointer(false)},
		{slot: 6, style: document.CellStyle{Underline: false}, want: boolPointer(true)},
	} {
		patch, ok := formatPatchForToolbarAction(item.slot, item.style)
		if !ok || patch.Bold != nil && *patch.Bold != *item.want || patch.Italic != nil && *patch.Italic != *item.want || patch.Underline != nil && *patch.Underline != *item.want {
			t.Fatalf("format action %d returned %+v, ok=%t", item.slot, patch, ok)
		}
	}
	patch, ok := formatPatchForToolbarAction(16, document.CellStyle{})
	if !ok || !patch.ClearFormat {
		t.Fatalf("clear-format action returned %+v, ok=%t", patch, ok)
	}
	if _, ok := formatPatchForToolbarAction(7, document.CellStyle{}); ok {
		t.Fatal("font color should open a picker instead of applying an implicit color")
	}
}

func TestToolbarPickerOptionsAndPatches(t *testing.T) {
	if got := toolbarPickerOptions(pickerAlignment); len(got) != 4 || got[2] != "居中" {
		t.Fatalf("alignment options=%v", got)
	}
	patch, ok := toolbarPickerPatch(pickerFillColor, 0)
	if !ok || patch.FillColor == nil || *patch.FillColor != "" {
		t.Fatalf("fill clear option=%+v, ok=%t", patch, ok)
	}
	patch, ok = toolbarPickerPatch(pickerNumberFormat, 3)
	if !ok || patch.NumberFormat == nil || *patch.NumberFormat != "yyyy-mm-dd" {
		t.Fatalf("date number format option=%+v, ok=%t", patch, ok)
	}
}

func boolPointer(value bool) *bool { return &value }
