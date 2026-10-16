package xlsx

import (
	"testing"

	"glance/internal/document"
)

func TestDataRegionEdgeMovesToCurrentOrNextContiguousRun(t *testing.T) {
	w := NewBlank()
	defer func() { _ = w.Close() }()
	sheet := w.SheetNames()[0]
	for cell, value := range map[string]string{
		"A2": "one", "B2": "two", "D2": "four", "E2": "five",
		"A4": "row four", "A5": "row five",
	} {
		if err := w.SetCellText(sheet, cell, value); err != nil {
			t.Fatalf("SetCellText(%s): %v", cell, err)
		}
	}

	for _, test := range []struct {
		name      string
		current   document.CellCoordinate
		direction document.SpreadsheetDirection
		want      document.CellCoordinate
	}{
		{name: "right through current run", current: document.CellCoordinate{Row: 2, Column: 1}, direction: document.SpreadsheetDirectionRight, want: document.CellCoordinate{Row: 2, Column: 2}},
		{name: "right from run edge skips blank gap and next run", current: document.CellCoordinate{Row: 2, Column: 2}, direction: document.SpreadsheetDirectionRight, want: document.CellCoordinate{Row: 2, Column: 5}},
		{name: "right from blank enters next run", current: document.CellCoordinate{Row: 2, Column: 3}, direction: document.SpreadsheetDirectionRight, want: document.CellCoordinate{Row: 2, Column: 5}},
		{name: "right with no following data reaches worksheet edge", current: document.CellCoordinate{Row: 2, Column: 5}, direction: document.SpreadsheetDirectionRight, want: document.CellCoordinate{Row: 2, Column: maxXLSXColumns}},
		{name: "left from blank enters previous run", current: document.CellCoordinate{Row: 2, Column: 3}, direction: document.SpreadsheetDirectionLeft, want: document.CellCoordinate{Row: 2, Column: 1}},
		{name: "down through current run", current: document.CellCoordinate{Row: 1, Column: 1}, direction: document.SpreadsheetDirectionDown, want: document.CellCoordinate{Row: 2, Column: 1}},
		{name: "down from run edge skips blank gap and next run", current: document.CellCoordinate{Row: 2, Column: 1}, direction: document.SpreadsheetDirectionDown, want: document.CellCoordinate{Row: 5, Column: 1}},
		{name: "up with no previous data reaches worksheet edge", current: document.CellCoordinate{Row: 2, Column: 1}, direction: document.SpreadsheetDirectionUp, want: document.CellCoordinate{Row: 1, Column: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := w.DataRegionEdge(sheet, test.current, test.direction)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("DataRegionEdge(%+v, %v) = %+v; want %+v", test.current, test.direction, got, test.want)
			}
		})
	}
}

func TestDataRegionEdgeValidatesCoordinatesAndDirection(t *testing.T) {
	w := NewBlank()
	defer func() { _ = w.Close() }()
	sheet := w.SheetNames()[0]
	for _, test := range []struct {
		name      string
		current   document.CellCoordinate
		direction document.SpreadsheetDirection
	}{
		{name: "row zero", current: document.CellCoordinate{Row: 0, Column: 1}, direction: document.SpreadsheetDirectionDown},
		{name: "column beyond xlsx", current: document.CellCoordinate{Row: 1, Column: maxXLSXColumns + 1}, direction: document.SpreadsheetDirectionRight},
		{name: "unknown direction", current: document.CellCoordinate{Row: 1, Column: 1}, direction: document.SpreadsheetDirection(99)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := w.DataRegionEdge(sheet, test.current, test.direction); err == nil {
				t.Fatal("DataRegionEdge accepted invalid input")
			}
		})
	}
}

func TestNextNonEmptyCellStopsAtTheFirstOccupiedCell(t *testing.T) {
	w := NewBlank()
	defer func() { _ = w.Close() }()
	sheet := w.SheetNames()[0]
	for cell, value := range map[string]string{
		"A1": "one", "D1": "four",
		"A2": "two", "B2": "three", "D2": "five", "E2": "six",
		"A4": "fourth row", "A5": "fifth row",
	} {
		if err := w.SetCellText(sheet, cell, value); err != nil {
			t.Fatalf("SetCellText(%s): %v", cell, err)
		}
	}
	if err := w.SetCellFormula(sheet, "B1", `=""`); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name      string
		current   document.CellCoordinate
		direction document.SpreadsheetDirection
		want      document.CellCoordinate
	}{
		{name: "next within current run", current: document.CellCoordinate{Row: 2, Column: 1}, direction: document.SpreadsheetDirectionRight, want: document.CellCoordinate{Row: 2, Column: 2}},
		{name: "first cell after blank gap", current: document.CellCoordinate{Row: 2, Column: 2}, direction: document.SpreadsheetDirectionRight, want: document.CellCoordinate{Row: 2, Column: 4}},
		{name: "right from blank cell", current: document.CellCoordinate{Row: 2, Column: 3}, direction: document.SpreadsheetDirectionRight, want: document.CellCoordinate{Row: 2, Column: 4}},
		{name: "nearest cell from empty result formula", current: document.CellCoordinate{Row: 1, Column: 1}, direction: document.SpreadsheetDirectionRight, want: document.CellCoordinate{Row: 1, Column: 2}},
		{name: "left skips back to nearest cell", current: document.CellCoordinate{Row: 2, Column: 4}, direction: document.SpreadsheetDirectionLeft, want: document.CellCoordinate{Row: 2, Column: 2}},
		{name: "down skips blank rows", current: document.CellCoordinate{Row: 2, Column: 1}, direction: document.SpreadsheetDirectionDown, want: document.CellCoordinate{Row: 4, Column: 1}},
		{name: "up finds nearest cell", current: document.CellCoordinate{Row: 4, Column: 1}, direction: document.SpreadsheetDirectionUp, want: document.CellCoordinate{Row: 2, Column: 1}},
		{name: "no next cell reaches sheet edge", current: document.CellCoordinate{Row: 2, Column: 5}, direction: document.SpreadsheetDirectionRight, want: document.CellCoordinate{Row: 2, Column: maxXLSXColumns}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := w.NextNonEmptyCell(sheet, test.current, test.direction)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("NextNonEmptyCell(%+v, %v) = %+v; want %+v", test.current, test.direction, got, test.want)
			}
		})
	}
}

func TestDataRegionEdgeTreatsEmptyResultFormulaAsOccupied(t *testing.T) {
	w := NewBlank()
	defer func() { _ = w.Close() }()
	sheet := w.SheetNames()[0]
	if err := w.SetCellText(sheet, "A1", "one"); err != nil {
		t.Fatal(err)
	}
	if err := w.SetCellFormula(sheet, "B1", `=""`); err != nil {
		t.Fatal(err)
	}
	if err := w.SetCellText(sheet, "D1", "four"); err != nil {
		t.Fatal(err)
	}
	got, err := w.DataRegionEdge(sheet, document.CellCoordinate{Row: 1, Column: 1}, document.SpreadsheetDirectionRight)
	if err != nil {
		t.Fatal(err)
	}
	if want := (document.CellCoordinate{Row: 1, Column: 2}); got != want {
		t.Fatalf("DataRegionEdge from A1 = %+v; want B1 because the formula cell is occupied even though its result is empty", got)
	}
}

func TestDataRegionEdgeDoesNotReusePositionsAfterWorkbookEdit(t *testing.T) {
	w := NewBlank()
	defer func() { _ = w.Close() }()
	sheet := w.SheetNames()[0]
	if err := w.SetCellText(sheet, "A1", "one"); err != nil {
		t.Fatal(err)
	}
	first, err := w.DataRegionEdge(sheet, document.CellCoordinate{Row: 1, Column: 1}, document.SpreadsheetDirectionRight)
	if err != nil {
		t.Fatal(err)
	}
	if want := (document.CellCoordinate{Row: 1, Column: maxXLSXColumns}); first != want {
		t.Fatalf("initial edge = %+v; want XFD1", first)
	}
	if err := w.SetCellText(sheet, "B1", "two"); err != nil {
		t.Fatal(err)
	}
	got, err := w.DataRegionEdge(sheet, document.CellCoordinate{Row: 1, Column: 1}, document.SpreadsheetDirectionRight)
	if err != nil {
		t.Fatal(err)
	}
	if want := (document.CellCoordinate{Row: 1, Column: 2}); got != want {
		t.Fatalf("edge after edit = %+v; want B1", got)
	}
}
