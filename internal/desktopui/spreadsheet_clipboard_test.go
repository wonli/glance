package desktopui

import (
	"glance/internal/document"
	"reflect"
	"testing"
)

func TestTSVRoundTripAndEmptyFields(t *testing.T) {
	want := [][]string{{"A", "", "C"}, {"", "2", ""}}
	cells := [][]document.Cell{{{Value: "A"}, {}, {Value: "C"}}, {{}, {Value: "2"}, {}}}
	got, err := decodeTSV(encodeTSV(cells))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TSV=%#v want %#v", got, want)
	}
}

func TestDecodeTSVRaggedTrailingNewlineAndQuotedTabs(t *testing.T) {
	got, err := decodeTSV("a\tb\n\"line1\nline2\"\t\"x\ty\"\n")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"a", "b"}, {"line1\nline2", "x\ty"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded=%#v want %#v", got, want)
	}
}

func TestPasteOverflowIsRejectedAtomicallyAtWorksheetEdges(t *testing.T) {
	if pasteFits(document.CellRange{StartRow: 1048576, StartColumn: 16383}, [][]string{{"a", "b"}, {"c"}}) {
		t.Fatal("paste extending past the worksheet edge must be rejected instead of partially applied")
	}
	if !pasteFits(document.CellRange{StartRow: 1048576, StartColumn: 16383}, [][]string{{"a", "b"}}) {
		t.Fatal("paste exactly fitting the worksheet edge should be allowed")
	}
}
