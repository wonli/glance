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

func TestFormatPatchForExcelP0ToolbarActions(t *testing.T) {
	// Align Left
	patch, ok := formatPatchForToolbarAction(toolbarActionAlignLeft, document.CellStyle{HorizontalAlignment: ""})
	if !ok || patch.HorizontalAlignment == nil || *patch.HorizontalAlignment != "left" {
		t.Fatalf("align left empty: got %+v, ok=%t", patch, ok)
	}
	patch, ok = formatPatchForToolbarAction(toolbarActionAlignLeft, document.CellStyle{HorizontalAlignment: "left"})
	if !ok || patch.HorizontalAlignment == nil || *patch.HorizontalAlignment != "general" {
		t.Fatalf("align left toggle: got %+v, ok=%t", patch, ok)
	}

	// Align Right
	patch, ok = formatPatchForToolbarAction(toolbarActionAlignRight, document.CellStyle{HorizontalAlignment: ""})
	if !ok || patch.HorizontalAlignment == nil || *patch.HorizontalAlignment != "right" {
		t.Fatalf("align right empty: got %+v, ok=%t", patch, ok)
	}
	patch, ok = formatPatchForToolbarAction(toolbarActionAlignRight, document.CellStyle{HorizontalAlignment: "right"})
	if !ok || patch.HorizontalAlignment == nil || *patch.HorizontalAlignment != "general" {
		t.Fatalf("align right toggle: got %+v, ok=%t", patch, ok)
	}

	// Wrap Text
	patch, ok = formatPatchForToolbarAction(toolbarActionWrapText, document.CellStyle{WrapText: false})
	if !ok || patch.WrapText == nil || !*patch.WrapText {
		t.Fatalf("wrap text false: got %+v, ok=%t", patch, ok)
	}
	patch, ok = formatPatchForToolbarAction(toolbarActionWrapText, document.CellStyle{WrapText: true})
	if !ok || patch.WrapText == nil || *patch.WrapText {
		t.Fatalf("wrap text true: got %+v, ok=%t", patch, ok)
	}

	// Percent Format
	patch, ok = formatPatchForToolbarAction(toolbarActionPercentFormat, document.CellStyle{NumberFormat: ""})
	if !ok || patch.NumberFormat == nil || *patch.NumberFormat != "0.00%" {
		t.Fatalf("percent empty: got %+v, ok=%t", patch, ok)
	}
	patch, ok = formatPatchForToolbarAction(toolbarActionPercentFormat, document.CellStyle{NumberFormat: "0.00%"})
	if !ok || patch.NumberFormat == nil || *patch.NumberFormat != "General" {
		t.Fatalf("percent 0.00%% toggle: got %+v, ok=%t", patch, ok)
	}
	patch, ok = formatPatchForToolbarAction(toolbarActionPercentFormat, document.CellStyle{NumberFormat: "0%"})
	if !ok || patch.NumberFormat == nil || *patch.NumberFormat != "General" {
		t.Fatalf("percent 0%% toggle: got %+v, ok=%t", patch, ok)
	}

	// Check unimplemented returns false
	for _, slot := range []int{toolbarActionAlignLeft, toolbarActionAlignRight, toolbarActionWrapText, toolbarActionPercentFormat} {
		if unimplementedSpreadsheetToolbarAction(slot) {
			t.Errorf("slot %d should not be marked unimplemented", slot)
		}
		if !formatStyleSlot(slot) {
			t.Errorf("slot %d should be recognized as formatStyleSlot", slot)
		}
	}

	// Check formatStyleActive
	if !formatStyleActive(toolbarActionAlignLeft, document.CellStyle{HorizontalAlignment: "left"}) {
		t.Error("align left should be active")
	}
	if formatStyleActive(toolbarActionAlignLeft, document.CellStyle{HorizontalAlignment: "right"}) {
		t.Error("align left should not be active when right")
	}
	if !formatStyleActive(toolbarActionAlignRight, document.CellStyle{HorizontalAlignment: "right"}) {
		t.Error("align right should be active")
	}
	if !formatStyleActive(toolbarActionWrapText, document.CellStyle{WrapText: true}) {
		t.Error("wrap text should be active")
	}
	if formatStyleActive(toolbarActionWrapText, document.CellStyle{WrapText: false}) {
		t.Error("wrap text should not be active")
	}
	if !formatStyleActive(toolbarActionPercentFormat, document.CellStyle{NumberFormat: "0.00%"}) {
		t.Error("percent 0.00% should be active")
	}
	if !formatStyleActive(toolbarActionPercentFormat, document.CellStyle{NumberFormat: "0%"}) {
		t.Error("percent 0% should be active")
	}
	if formatStyleActive(toolbarActionPercentFormat, document.CellStyle{NumberFormat: "General"}) {
		t.Error("percent General should not be active")
	}
}
