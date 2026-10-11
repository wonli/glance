package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"glance/internal/document"
)

func TestSpreadsheetSortAndHistoryCapabilitiesUseLiveWorkbookAndRoundTrip(t *testing.T) {
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
			RequestID: fmt.Sprintf("xlsx-sort-%d", requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "sort-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}
	inspect := func() (DocumentSnapshot, [][]struct {
		Value   string `json:"value"`
		Formula string `json:"formula"`
	}) {
		t.Helper()
		bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 3}
		snapshot, inspectErr := runtime.Queries().InspectDocument(ctx, InspectRequest{
			DocumentID: documentID, Sheet: "Sheet1", Range: &bounds,
		})
		if inspectErr != nil {
			t.Fatalf("InspectDocument(A1:C3): %v", inspectErr)
		}
		var output struct {
			Rows [][]struct {
				Value   string `json:"value"`
				Formula string `json:"formula"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(snapshot.Content, &output); err != nil {
			t.Fatalf("decode workbook range: %v; content=%s", err, snapshot.Content)
		}
		return snapshot, output.Rows
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
	assertChanged := func(capabilityID string, arguments any, wantChanged bool) CapabilityInvocationResult {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, documentID)
		result, invokeErr := invoke(capabilityID, arguments)
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(%s): %v", capabilityID, invokeErr)
		}
		var output struct {
			Changed bool `json:"changed"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode %s result: %v; data=%s", capabilityID, err, result.Data)
		}
		if output.Changed != wantChanged {
			t.Fatalf("%s changed=%v, want %v; data=%s", capabilityID, output.Changed, wantChanged, result.Data)
		}
		after := mustGetDocumentInfo(t, runtime, documentID)
		wantRevision := before.Revision
		if wantChanged {
			wantRevision++
		}
		if after.Revision != wantRevision || after.Dirty != (before.Dirty || wantChanged) {
			t.Fatalf("%s state: before=%+v after=%+v changed=%v", capabilityID, before, after, wantChanged)
		}
		return result
	}
	sortArgs := map[string]any{
		"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 2},
		"key_column": 1, "descending": false,
	}
	assertChanged("spreadsheet.history.apply", map[string]any{"action": "undo"}, false)
	assertAtomicFailure("spreadsheet.range.sort", map[string]any{
		"sheet": "Missing", "bounds": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 2}, "key_column": 1, "descending": false,
	})

	seed := []map[string]any{
		{"row": 1, "column": 1, "value": "3"}, {"row": 1, "column": 2, "value": "three"},
		{"row": 2, "column": 1, "value": "1"}, {"row": 2, "column": 2, "value": "one"},
		{"row": 3, "column": 1, "value": "2"}, {"row": 3, "column": 2, "value": "two"},
		{"row": 1, "column": 3, "value": "not sorted"}, {"row": 2, "column": 3, "value": "plain text"},
	}
	assertChanged("spreadsheet.cells.set", map[string]any{"sheet": "Sheet1", "edits": seed}, true)
	assertChanged("spreadsheet.formula.set", map[string]any{"sheet": "Sheet1", "cell": "C1", "formula": "=SUM(1,2)"}, true)
	assertAtomicFailure("spreadsheet.range.sort", map[string]any{
		"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 3}, "key_column": 1, "descending": false,
	})
	assertChanged("spreadsheet.range.sort", sortArgs, true)
	_, sortedRows := inspect()
	if sortedRows[0][0].Value != "1" || sortedRows[0][1].Value != "one" || sortedRows[1][0].Value != "2" || sortedRows[1][1].Value != "two" || sortedRows[2][0].Value != "3" || sortedRows[2][1].Value != "three" {
		t.Fatalf("sorted range did not preserve row associations: %+v", sortedRows)
	}
	assertChanged("spreadsheet.range.sort", sortArgs, false)
	assertChanged("spreadsheet.history.apply", map[string]any{"action": "undo"}, true)
	_, undoneRows := inspect()
	if undoneRows[0][0].Value != "3" || undoneRows[1][0].Value != "1" || undoneRows[2][0].Value != "2" {
		t.Fatalf("undo did not restore pre-sort rows: %+v", undoneRows)
	}
	assertChanged("spreadsheet.history.apply", map[string]any{"action": "redo"}, true)

	path := filepath.Join(t.TempDir(), "sorted.xlsx")
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
		t.Fatalf("OpenDocument after sort: %v", err)
	}
	documentID = reopened.DocumentID
	_, reopenedRows := inspect()
	if reopened.Dirty || reopenedRows[0][0].Value != "1" || reopenedRows[1][0].Value != "2" || reopenedRows[2][0].Value != "3" {
		t.Fatalf("saved sorted workbook did not reopen cleanly: info=%+v rows=%+v", reopened, reopenedRows)
	}
	assertChanged("spreadsheet.range.sort", sortArgs, false)
}
