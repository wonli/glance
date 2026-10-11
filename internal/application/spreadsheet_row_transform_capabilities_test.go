package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
)

func TestSpreadsheetDuplicateRemovalAndTextSplitCapabilitiesRoundTrip(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument(XLSX): %v", err)
	}
	documentID := doc.DocumentID
	requestNumber := 0
	invoke := func(capabilityID string, arguments any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		info := mustGetDocumentInfo(t, runtime, documentID)
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("xlsx-transform-%d", requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "xlsx-transform-human", Kind: OperatorHuman},
			Arguments: mustJSON(t, arguments),
		})
	}
	inspect := func() (DocumentSnapshot, [][]string) {
		t.Helper()
		bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 7, EndColumn: 5}
		snapshot, inspectErr := runtime.Queries().InspectDocument(ctx, InspectRequest{
			DocumentID: documentID, Sheet: "Sheet1", Range: &bounds,
		})
		if inspectErr != nil {
			t.Fatalf("InspectDocument(A1:E7): %v", inspectErr)
		}
		var output struct {
			Rows [][]struct {
				Value string `json:"value"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(snapshot.Content, &output); err != nil {
			t.Fatalf("decode workbook range: %v; content=%s", err, snapshot.Content)
		}
		rows := make([][]string, len(output.Rows))
		for rowIndex, row := range output.Rows {
			rows[rowIndex] = make([]string, len(row))
			for columnIndex, cell := range row {
				rows[rowIndex][columnIndex] = cell.Value
			}
		}
		return snapshot, rows
	}
	assertAtomicFailure := func(capabilityID string, arguments any) {
		t.Helper()
		before, beforeRows := inspect()
		_, invokeErr := invoke(capabilityID, arguments)
		if invokeErr == nil {
			t.Fatalf("%s accepted invalid target/operation: %v", capabilityID, arguments)
		}
		after, afterRows := inspect()
		if after.Info.Revision != before.Info.Revision || after.Info.Dirty != before.Info.Dirty || !bytes.Equal(after.Content, before.Content) {
			t.Fatalf("%s partially changed workbook: before=%+v after=%+v beforeRows=%+v afterRows=%+v", capabilityID, before.Info, after.Info, beforeRows, afterRows)
		}
	}
	assertTransformResult := func(capabilityID string, arguments any, wantChanged bool) json.RawMessage {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, documentID)
		result, invokeErr := invoke(capabilityID, arguments)
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(%s): %v", capabilityID, invokeErr)
		}
		changed := false
		var output map[string]json.RawMessage
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode %s result: %v; data=%s", capabilityID, err, result.Data)
		}
		if raw := output["changed"]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &changed); err != nil {
				t.Fatalf("decode %s changed: %v", capabilityID, err)
			}
		} else if raw := output["removed_count"]; len(raw) > 0 {
			var removed int
			if err := json.Unmarshal(raw, &removed); err != nil {
				t.Fatalf("decode removed_count: %v; data=%s", err, result.Data)
			}
			changed = removed > 0
		} else if raw := output["rows_split"]; len(raw) > 0 {
			var rowsSplit int
			if err := json.Unmarshal(raw, &rowsSplit); err != nil {
				t.Fatalf("decode rows_split: %v; data=%s", err, result.Data)
			}
			changed = rowsSplit > 0
		} else {
			t.Fatalf("%s result has no changed indicator: %s", capabilityID, result.Data)
		}
		if changed != wantChanged {
			t.Fatalf("%s changed=%v, want %v; data=%s", capabilityID, changed, wantChanged, result.Data)
		}
		after := mustGetDocumentInfo(t, runtime, documentID)
		wantRevision := before.Revision
		if wantChanged {
			wantRevision++
		}
		if after.Revision != wantRevision || after.Dirty != (before.Dirty || wantChanged) {
			t.Fatalf("%s state: before=%+v after=%+v changed=%v", capabilityID, before, after, changed)
		}
		return result.Data
	}
	cell := func(rows [][]string, row, column int) string {
		t.Helper()
		if row < 1 || row > len(rows) || column < 1 || column > len(rows[row-1]) {
			t.Fatalf("cell outside inspected grid: row=%d column=%d", row, column)
		}
		return rows[row-1][column-1]
	}
	duplicatesArgs := map[string]any{
		"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 4, EndColumn: 2}, "key_column": 1,
	}
	delimiter := strings.Repeat("🙂", 100)
	splitArgs := map[string]any{
		"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 5, StartColumn: 4, EndRow: 6, EndColumn: 4}, "key_column": 4, "delimiter": delimiter,
	}

	seed := []map[string]any{
		{"row": 1, "column": 1, "value": "key-1"}, {"row": 1, "column": 2, "value": "first"},
		{"row": 2, "column": 1, "value": "key-1"}, {"row": 2, "column": 2, "value": "duplicate"},
		{"row": 3, "column": 1, "value": "key-2"}, {"row": 3, "column": 2, "value": "second"},
		{"row": 4, "column": 1, "value": "key-3"}, {"row": 4, "column": 2, "value": "third"},
		{"row": 6, "column": 4, "value": "alpha" + delimiter + "beta"}, {"row": 6, "column": 5, "value": "overwritten"},
		{"row": 7, "column": 4, "value": "gamma" + delimiter + "delta"}, {"row": 7, "column": 5, "value": "overwritten"},
	}
	assertTransformResult("spreadsheet.cells.set", map[string]any{"sheet": "Sheet1", "edits": seed}, true)
	assertAtomicFailure("spreadsheet.duplicates.remove", map[string]any{
		"sheet": "Missing", "bounds": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 4, EndColumn: 2}, "key_column": 1,
	})
	var duplicateResult struct {
		RemovedCount   int `json:"removed_count"`
		RemainingCount int `json:"remaining_count"`
	}
	if err := json.Unmarshal(assertTransformResult("spreadsheet.duplicates.remove", duplicatesArgs, true), &duplicateResult); err != nil || duplicateResult.RemovedCount != 1 || duplicateResult.RemainingCount != 3 {
		t.Fatalf("duplicate removal result=%+v error=%v", duplicateResult, err)
	}
	_, afterDuplicates := inspect()
	if cell(afterDuplicates, 1, 2) != "first" || cell(afterDuplicates, 2, 2) != "second" || cell(afterDuplicates, 3, 2) != "third" {
		t.Fatalf("duplicate removal lost or reordered nonduplicate rows: %+v", afterDuplicates)
	}
	assertTransformResult("spreadsheet.duplicates.remove", duplicatesArgs, false)
	assertAtomicFailure("spreadsheet.text_to_columns.split", map[string]any{
		"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 5, StartColumn: 4, EndRow: 6, EndColumn: 4}, "key_column": 5, "delimiter": ",",
	})
	var splitResult struct {
		RowsSplit int `json:"rows_split"`
		MaxCols   int `json:"max_columns"`
	}
	if err := json.Unmarshal(assertTransformResult("spreadsheet.text_to_columns.split", splitArgs, true), &splitResult); err != nil || splitResult.RowsSplit != 2 || splitResult.MaxCols != 2 {
		t.Fatalf("text split result=%+v error=%v", splitResult, err)
	}
	_, afterSplit := inspect()
	if cell(afterSplit, 5, 4) != "alpha" || cell(afterSplit, 5, 5) != "beta" || cell(afterSplit, 6, 4) != "gamma" || cell(afterSplit, 6, 5) != "delta" {
		t.Fatalf("text split did not write tokens at explicit source column: %+v", afterSplit)
	}
	assertTransformResult("spreadsheet.text_to_columns.split", splitArgs, false)

	path := filepath.Join(t.TempDir(), "row-transforms.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if info := mustGetDocumentInfo(t, runtime, documentID); info.Dirty {
		t.Fatalf("saved workbook remains dirty: %+v", info)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after row transforms: %v", err)
	}
	documentID = reopened.DocumentID
	_, reopenedRows := inspect()
	if reopened.Dirty || cell(reopenedRows, 2, 2) != "second" || cell(reopenedRows, 3, 2) != "third" || cell(reopenedRows, 5, 4) != "alpha" || cell(reopenedRows, 5, 5) != "beta" || cell(reopenedRows, 6, 4) != "gamma" || cell(reopenedRows, 6, 5) != "delta" {
		t.Fatalf("row transforms did not persist cleanly: info=%+v rows=%+v", reopened, reopenedRows)
	}
}
