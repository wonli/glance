package application

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
)

type testSpreadsheettypedSheetEditsArguments struct {
	Sheet string                       `json:"sheet"`
	Edits []spreadsheetCellEditFixture `json:"edits"`
}

type testSpreadsheettypedSheetEditsArguments119 struct {
	Sheet string                       `json:"sheet"`
	Edits []spreadsheetCellEditFixture `json:"edits"`
}

type testSpreadsheettypedSheetCellArguments struct {
	Sheet string `json:"sheet"`
	Cell  string `json:"cell"`
	Value any    `json:"value"`
}

func TestSpreadsheetCellCommandsPreserveExplicitJSONValueTypes(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	operator := Operator{ID: "typed-cell-test", Kind: OperatorExternalAgent}
	revision := doc.Revision
	setCell := func(requestID, cell string, value any, wantChanged bool) {
		t.Helper()
		result, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: requestID, DocumentID: doc.DocumentID, CommandID: "spreadsheet.cell.set",
			ExpectedRevision: revision, Operator: operator,
			Arguments: mustJSON(t, testSpreadsheettypedSheetCellArguments{Sheet: "Sheet1", Cell: cell, Value: value}),
		})
		if err != nil {
			t.Fatalf("set %s (%T): %v", cell, value, err)
		}
		var output struct {
			Changed bool            `json:"changed"`
			Value   json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil || output.Changed != wantChanged {
			t.Fatalf("set %s (%T) result=%s error=%v; changed=%v want %v", cell, value, result.Data, err, output.Changed, wantChanged)
		}
		if wantChanged {
			revision++
		}
		if result.Revision != revision {
			t.Fatalf("set %s (%T) revision=%d want %d", cell, value, result.Revision, revision)
		}
	}
	setCell("typed-cell-text", "A1", "00123", true)
	setCell("typed-cell-number", "B1", 123.5, true)
	setCell("typed-cell-boolean", "C1", true, true)
	setCell("typed-cell-text-noop", "A1", "00123", false)
	setCell("typed-cell-boolean-noop", "C1", true, false)
	setCell("typed-cell-number-first", "D1", 123, true)
	setCell("typed-cell-number-to-text", "D1", "123", true)
	setCell("typed-cell-text-repeat", "D1", "123", false)
	longUnicodeText := strings.Repeat("中", 11000)
	setCell("typed-cell-long-unicode", "E1", longUnicodeText, true)

	batch := []spreadsheetCellEditFixture{
		{Row: 2, Column: 1, Value: "00045"},
		{Row: 2, Column: 2, Value: 45.25},
		{Row: 2, Column: 3, Value: false},
	}
	setBatch := func(requestID string, wantChanged bool) {
		t.Helper()
		result, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: requestID, DocumentID: doc.DocumentID, CommandID: "spreadsheet.cells.set",
			ExpectedRevision: revision, Operator: operator,
			Arguments: mustJSON(t, testSpreadsheettypedSheetEditsArguments119{Sheet: "Sheet1", Edits: batch}),
		})
		if err != nil {
			t.Fatalf("batch set: %v", err)
		}
		var output struct {
			Changed bool `json:"changed"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil || output.Changed != wantChanged {
			t.Fatalf("batch result=%s error=%v; changed=%v want %v", result.Data, err, output.Changed, wantChanged)
		}
		if wantChanged {
			revision++
		}
		if result.Revision != revision {
			t.Fatalf("batch revision=%d want %d", result.Revision, revision)
		}
	}
	setBatch("typed-cell-batch", true)
	setBatch("typed-cell-batch-noop", false)
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "typed-cell-invalid-object", DocumentID: doc.DocumentID, CommandID: "spreadsheet.cells.set",
		ExpectedRevision: revision, Operator: operator,
		Arguments: mustJSON(t, testSpreadsheettypedSheetEditsArguments{Sheet: "Sheet1", Edits: []spreadsheetCellEditFixture{{Row: 3, Column: 1, Value: map[string]string{"bad": "value"}}}}),
	}); err == nil {
		t.Fatal("batch cell set accepted a JSON object as a cell value")
	}
	state, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || state.Revision != revision || !state.Dirty {
		t.Fatalf("invalid typed value changed workbook state: state=%+v error=%v", state, err)
	}

	path := filepath.Join(t.TempDir(), "typed-values.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("CloseDocument before reopen: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after save: %v", err)
	}
	query, err := runtime.Queries().InspectDocument(ctx, InspectRequest{
		DocumentID: reopened.DocumentID, Sheet: "Sheet1",
		Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 5},
	})
	if err != nil {
		t.Fatalf("InspectDocument after reopen: %v", err)
	}
	var values struct {
		Rows [][]struct {
			Value string `json:"value"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(query.Content, &values); err != nil {
		t.Fatalf("decode reopened cells: %v", err)
	}
	want := [][]string{{"00123", "123.5", "TRUE", "123", longUnicodeText}, {"00045", "45.25", "FALSE", "", ""}}
	if len(values.Rows) != len(want) {
		t.Fatalf("reopened rows=%+v; want %v", values.Rows, want)
	}
	for row := range want {
		if len(values.Rows[row]) != len(want[row]) {
			t.Fatalf("reopened row %d=%+v; want %v", row, values.Rows[row], want[row])
		}
		for column := range want[row] {
			if values.Rows[row][column].Value != want[row][column] {
				t.Fatalf("reopened cell (%d,%d)=%q; want %q", row, column, values.Rows[row][column].Value, want[row][column])
			}
		}
	}

	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open saved XLSX: %v", err)
	}
	defer archive.Close()
	var worksheet []byte
	for _, file := range archive.File {
		if file.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open worksheet XML: %v", err)
		}
		worksheet, err = io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("read worksheet XML: read=%v close=%v", err, closeErr)
		}
		break
	}
	cellTypes := map[string]string{}
	decoder := xml.NewDecoder(bytes.NewReader(worksheet))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("parse worksheet XML: %v", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "c" {
			continue
		}
		var cell struct {
			Ref  string `xml:"r,attr"`
			Type string `xml:"t,attr"`
		}
		if err := decoder.DecodeElement(&cell, &start); err != nil {
			t.Fatalf("decode cell type: %v", err)
		}
		cellTypes[cell.Ref] = cell.Type
	}
	if cellTypes["A1"] != "s" || cellTypes["A2"] != "s" || cellTypes["D1"] != "s" || cellTypes["E1"] != "s" || (cellTypes["B1"] != "" && cellTypes["B1"] != "n") || (cellTypes["B2"] != "" && cellTypes["B2"] != "n") || cellTypes["C1"] != "b" || cellTypes["C2"] != "b" {
		t.Fatalf("XLSX stored the wrong explicit cell types: %v XML=%s", cellTypes, worksheet)
	}
}
