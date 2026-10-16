package desktopui

import "testing"

func TestFormatCellsVerticalAlignmentOnlyChangesWhenSelected(t *testing.T) {
	draft := spreadsheetFormatCellsDialogState{}
	draft.init()
	draft.verticalAlignment = "center"
	if patch, changed := draft.patch(); changed || patch.VerticalAlignment != nil {
		t.Fatalf("untouched vertical alignment patch=%+v, changed=%t; want no change", patch, changed)
	}

	draft.verticalAlignment = "top"
	draft.verticalTouched = true
	patch, changed := draft.patch()
	if !changed || patch.VerticalAlignment == nil || *patch.VerticalAlignment != "top" {
		t.Fatalf("selected top alignment patch=%+v, changed=%t; want vertical_alignment=top", patch, changed)
	}
	if patch.HorizontalAlignment != nil {
		t.Fatalf("vertical alignment change also modified horizontal alignment: %+v", patch)
	}
}
