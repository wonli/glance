package xlsx

import (
	"reflect"
	"testing"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

type spreadsheetDimensionRangeCellReader interface {
	VisitDimensionRangeCells(sheet string, axis document.SpreadsheetDimensionAxis, start, end int, visit func(document.SpreadsheetDimensionCell) error) error
}

func TestWorkbookVisitDimensionRangeCellsReportsBothCoordinatesAndSizes(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", "short"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "B1", "longer text"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "A2", "second row"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "B2", "merged anchor"); err != nil {
		t.Fatal(err)
	}
	if err := f.MergeCell("Sheet1", "B2", "C2"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetRowHeight("Sheet1", 1, 27); err != nil {
		t.Fatal(err)
	}
	if err := f.SetRowHeight("Sheet1", 2, 31); err != nil {
		t.Fatal(err)
	}
	if err := f.SetColWidth("Sheet1", "A", "A", 12); err != nil {
		t.Fatal(err)
	}
	if err := f.SetColWidth("Sheet1", "B", "B", 18); err != nil {
		t.Fatal(err)
	}
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()

	reader, ok := any(wb).(spreadsheetDimensionRangeCellReader)
	if !ok {
		t.Fatal("Workbook does not implement range dimension cell reading")
	}
	type observed struct {
		dimension, index int
		value            string
		dimensionSize    float64
		columnWidth      float64
	}
	var rows []observed
	err = reader.VisitDimensionRangeCells("Sheet1", document.SpreadsheetDimensionRow, 1, 2, func(entry document.SpreadsheetDimensionCell) error {
		rows = append(rows, observed{entry.DimensionIndex, entry.Index, entry.Cell.Value, entry.DimensionSize, entry.ColumnWidthChars})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRows := []observed{
		{dimension: 1, index: 1, value: "short", dimensionSize: 27, columnWidth: 12},
		{dimension: 1, index: 2, value: "longer text", dimensionSize: 27, columnWidth: 18},
		{dimension: 2, index: 1, value: "second row", dimensionSize: 31, columnWidth: 12},
	}
	if !reflect.DeepEqual(rows, wantRows) {
		t.Fatalf("row range entries = %+v, want %+v", rows, wantRows)
	}

	var columns []observed
	err = reader.VisitDimensionRangeCells("Sheet1", document.SpreadsheetDimensionColumn, 1, 2, func(entry document.SpreadsheetDimensionCell) error {
		columns = append(columns, observed{entry.DimensionIndex, entry.Index, entry.Cell.Value, entry.DimensionSize, entry.ColumnWidthChars})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	wantColumns := []observed{
		{dimension: 1, index: 1, value: "short", dimensionSize: 12},
		{dimension: 2, index: 1, value: "longer text", dimensionSize: 18},
		{dimension: 1, index: 2, value: "second row", dimensionSize: 12},
	}
	if !reflect.DeepEqual(columns, wantColumns) {
		t.Fatalf("column range entries = %+v, want %+v", columns, wantColumns)
	}
}
