package xls

import (
	"reflect"
	"testing"
)

func TestXFStyleMapsFontFillBorderAlignmentAndNumberFormat(t *testing.T) {
	reader, err := OpenImportReader("testdata/formula-style-multisheet.xls")
	if err != nil {
		t.Fatalf("OpenImportReader: %v", err)
	}
	defer reader.Close()

	// The sample has the same styled XF on A1 and the otherwise blank F4.
	style, err := reader.ExcelizeStyle(0x15)
	if err != nil {
		t.Fatalf("ExcelizeStyle: %v", err)
	}
	if style.Font == nil || !style.Font.Bold {
		t.Errorf("font = %#v, want bold font", style.Font)
	}
	if style.Fill.Type != "pattern" || style.Fill.Pattern != 1 || len(style.Fill.Color) == 0 {
		t.Errorf("fill = %#v, want mapped solid pattern fill", style.Fill)
	} else if style.Fill.Color[0] != "FFF2CC" {
		t.Errorf("fill foreground = %q, want source color FFF2CC", style.Fill.Color[0])
	}
	if len(style.Border) == 0 {
		t.Error("border is empty, want the source bottom border")
	}
	if style.Alignment == nil || style.Alignment.Horizontal != "center" || style.Alignment.Vertical != "center" || !style.Alignment.WrapText {
		t.Errorf("alignment = %#v, want centered wrapped text", style.Alignment)
	}
	if style.CustomNumFmt == nil || *style.CustomNumFmt != "0.00" {
		t.Errorf("custom number format = %#v, want 0.00", style.CustomNumFmt)
	}
}

func TestXFStyleLeavesDefaultCellsUnfilled(t *testing.T) {
	reader, err := OpenImportReader("testdata/formula-style-multisheet.xls")
	if err != nil {
		t.Fatalf("OpenImportReader: %v", err)
	}
	defer reader.Close()

	style, err := reader.ExcelizeStyle(0)
	if err != nil {
		t.Fatalf("ExcelizeStyle: %v", err)
	}
	if style.Fill.Type != "" || style.Fill.Pattern != 0 || len(style.Fill.Color) != 0 {
		t.Fatalf("default XF fill = %#v, want no fill", style.Fill)
	}
}

func TestXFStyleValuesRemainDistinctByXFIdentity(t *testing.T) {
	reader, err := OpenImportReader("testdata/formula-style-multisheet.xls")
	if err != nil {
		t.Fatalf("OpenImportReader: %v", err)
	}
	defer reader.Close()

	first, err := reader.ExcelizeStyle(0x15)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := reader.ExcelizeStyle(0x15)
	if err != nil {
		t.Fatal(err)
	}
	other, err := reader.ExcelizeStyle(0x0F)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, repeated) {
		t.Fatal("same XF produced different Excelize styles")
	}
	if reflect.DeepEqual(first, other) {
		t.Fatal("different XF indices collapsed to the same style")
	}
}
