package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"glance/internal/document"
)

type testSpreadsheetcellsSheetEditsArguments struct {
	Sheet string                       `json:"sheet"`
	Edits []spreadsheetCellEditFixture `json:"edits"`
}

func TestSpreadsheetBatchCellSetIsAtomicIdempotentAndRoundTrips(t *testing.T) {
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
	operator := Operator{ID: "spreadsheet-batch-test", Kind: OperatorExternalAgent}
	edits := []spreadsheetCellEditFixture{
		{Row: 1, Column: 1, Value: "人员"},
		{Row: 1, Column: 2, Value: "工资"},
	}
	invoke := func(requestID string, revision uint64, sheet string, batch []spreadsheetCellEditFixture) (CommandResult, error) {
		t.Helper()
		return runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: requestID, DocumentID: doc.DocumentID, CommandID: "spreadsheet.cells.set",
			ExpectedRevision: revision, Operator: operator,
			Arguments: mustJSON(t, testSpreadsheetcellsSheetEditsArguments{Sheet: sheet, Edits: batch}),
		})
	}
	first, err := invoke("batch-set-first", doc.Revision, "Sheet1", edits)
	if err != nil {
		t.Fatalf("write batch: %v", err)
	}
	var firstData struct {
		Changed        bool   `json:"changed"`
		Sheet          string `json:"sheet"`
		RequestedCount int    `json:"requested_count"`
	}
	if err := json.Unmarshal(first.Data, &firstData); err != nil || !firstData.Changed || firstData.Sheet != "Sheet1" || firstData.RequestedCount != 2 || first.Revision != doc.Revision+1 {
		t.Fatalf("batch result=%+v revision=%d data=%s error=%v", firstData, first.Revision, first.Data, err)
	}

	readCells := func(documentID string) [][]struct {
		Value string `json:"value"`
	} {
		t.Helper()
		query, err := runtime.Queries().InspectDocument(ctx, InspectRequest{
			DocumentID: documentID, Sheet: "Sheet1",
			Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2},
		})
		if err != nil {
			t.Fatalf("InspectDocument: %v", err)
		}
		var sheet struct {
			Rows [][]struct {
				Value string `json:"value"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(query.Content, &sheet); err != nil {
			t.Fatalf("decode cell range: %v; content=%s", err, query.Content)
		}
		return sheet.Rows
	}
	assertValues := func(documentID string) {
		t.Helper()
		rows := readCells(documentID)
		if len(rows) != 1 || len(rows[0]) != 2 || rows[0][0].Value != "人员" || rows[0][1].Value != "工资" {
			t.Fatalf("batch values = %+v; want [人员 工资]", rows)
		}
	}
	assertValues(doc.DocumentID)

	noop, err := invoke("batch-set-repeat", first.Revision, "Sheet1", edits)
	var noopData struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noop.Data, &noopData) != nil || noopData.Changed || noop.Revision != first.Revision {
		t.Fatalf("repeat batch result=%s revision=%d error=%v; want no-op at %d", noop.Data, noop.Revision, err, first.Revision)
	}
	stateAfterNoop, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterNoop.Revision != first.Revision || !stateAfterNoop.Dirty {
		t.Fatalf("repeat batch changed document state: state=%+v error=%v", stateAfterNoop, err)
	}

	invalidBatches := []struct {
		requestID string
		sheet     string
		edits     []spreadsheetCellEditFixture
	}{
		{
			requestID: "batch-set-duplicate-coordinate", sheet: "Sheet1",
			edits: []spreadsheetCellEditFixture{{Row: 1, Column: 1, Value: "覆盖"}, {Row: 1, Column: 1, Value: "应拒绝"}},
		},
		{requestID: "batch-set-missing-sheet", sheet: "MissingSheet", edits: []spreadsheetCellEditFixture{{Row: 1, Column: 1, Value: "不应写入"}}},
	}
	for _, invalid := range invalidBatches {
		if _, err := invoke(invalid.requestID, first.Revision, invalid.sheet, invalid.edits); err == nil {
			t.Fatalf("batch write accepted invalid request: %+v", invalid)
		}
		state, err := runtime.GetDocument(ctx, doc.DocumentID)
		if err != nil || state.Revision != stateAfterNoop.Revision || state.Dirty != stateAfterNoop.Dirty {
			t.Fatalf("invalid batch changed document state: before=%+v after=%+v error=%v", stateAfterNoop, state, err)
		}
		assertValues(doc.DocumentID)
	}

	path := filepath.Join(t.TempDir(), "batch-cells.xlsx")
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
	assertValues(reopened.DocumentID)
	reopenedState, err := runtime.GetDocument(ctx, reopened.DocumentID)
	if err != nil || reopenedState.Dirty {
		t.Fatalf("reopened workbook is unexpectedly dirty: state=%+v error=%v", reopenedState, err)
	}
}
