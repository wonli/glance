package application

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"glance/internal/document"
)

func TestSpreadsheetFlashFillCapabilityTreatsFilledRangeAsNoop(t *testing.T) {
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
			RequestID: fmt.Sprintf("xlsx-flash-fill-%d", requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "flash-fill-agent", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}
	inspectValues := func() []string {
		t.Helper()
		bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 4, EndColumn: 1}
		snapshot, inspectErr := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: documentID, Sheet: "Sheet1", Range: &bounds})
		if inspectErr != nil {
			t.Fatalf("InspectDocument(A1:A4): %v", inspectErr)
		}
		var output struct {
			Rows [][]struct {
				Value string `json:"value"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(snapshot.Content, &output); err != nil {
			t.Fatalf("decode flash-fill range: %v; content=%s", err, snapshot.Content)
		}
		values := make([]string, len(output.Rows))
		for index := range output.Rows {
			values[index] = output.Rows[index][0].Value
		}
		return values
	}
	assertFill := func(wantRowsFilled int, wantRevisionDelta uint64, wantDirty bool) {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, documentID)
		result, invokeErr := invoke("spreadsheet.flash_fill.apply", map[string]any{
			"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 4, EndColumn: 1},
		})
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(spreadsheet.flash_fill.apply): %v", invokeErr)
		}
		var output struct {
			RowsFilled int `json:"rows_filled"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode flash-fill result: %v; data=%s", err, result.Data)
		}
		if output.RowsFilled != wantRowsFilled {
			t.Fatalf("rows_filled=%d, want %d; data=%s", output.RowsFilled, wantRowsFilled, result.Data)
		}
		after := mustGetDocumentInfo(t, runtime, documentID)
		if after.Revision != before.Revision+wantRevisionDelta || after.Dirty != wantDirty {
			t.Fatalf("flash-fill state: before=%+v after=%+v want revision delta=%d dirty=%v", before, after, wantRevisionDelta, wantDirty)
		}
	}

	if _, err := invoke("spreadsheet.cells.set", map[string]any{"sheet": "Sheet1", "edits": []map[string]any{
		{"row": 1, "column": 1, "value": "1"}, {"row": 2, "column": 1, "value": "2"},
	}}); err != nil {
		t.Fatalf("seed workbook: %v", err)
	}
	assertFill(2, 1, true)
	if got := inspectValues(); got[0] != "1" || got[1] != "2" || got[2] != "3" || got[3] != "4" {
		t.Fatalf("flash-fill values=%v, want [1 2 3 4]", got)
	}
	assertFill(0, 0, true)
	beforeFailure := mustGetDocumentInfo(t, runtime, documentID)
	beforeValues := inspectValues()
	if _, err := invoke("spreadsheet.flash_fill.apply", map[string]any{
		"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 1, StartColumn: 2, EndRow: 4, EndColumn: 2},
	}); err == nil {
		t.Fatal("Flash Fill accepted a range without source pattern data")
	}
	if afterFailure := mustGetDocumentInfo(t, runtime, documentID); afterFailure.Revision != beforeFailure.Revision || afterFailure.Dirty != beforeFailure.Dirty || fmt.Sprint(inspectValues()) != fmt.Sprint(beforeValues) {
		t.Fatalf("missing-pattern error changed workbook: before=%+v after=%+v", beforeFailure, afterFailure)
	}

	path := filepath.Join(t.TempDir(), "flash-fill.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after Flash Fill: %v", err)
	}
	documentID = reopened.DocumentID
	if reopened.Dirty {
		t.Fatalf("reopened workbook is dirty: %+v", reopened)
	}
	assertFill(0, 0, false)
}
