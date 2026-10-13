package application

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"glance/internal/document"
)

type spreadsheetWorkbookRecalculatePayload struct {
}

type spreadsheetCellSetPayload struct {
	Sheet string `json:"sheet"`
	Cell  string `json:"cell"`
	Value string `json:"value"`
}

type spreadsheetCellsSetPayloadSheet struct {
	Sheet string                       `json:"sheet"`
	Edits []spreadsheetCellEditFixture `json:"edits"`
}

func TestSpreadsheetRecalculateCapabilityRecalculatesWorkbook(t *testing.T) {
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
			RequestID: fmt.Sprintf("xlsx-recalculate-%d", requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "recalculate-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}
	inspectFormula := func() (string, string) {
		t.Helper()
		bounds := document.CellRange{StartRow: 1, StartColumn: 2, EndRow: 1, EndColumn: 2}
		snapshot, inspectErr := runtime.Queries().InspectDocument(ctx, InspectRequest{
			DocumentID: documentID, Sheet: "Sheet1", Range: &bounds,
		})
		if inspectErr != nil {
			t.Fatalf("InspectDocument(B1): %v", inspectErr)
		}
		var output struct {
			Rows [][]struct {
				Value   string `json:"value"`
				Formula string `json:"formula"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(snapshot.Content, &output); err != nil {
			t.Fatalf("decode B1 query: %v; content=%s", err, snapshot.Content)
		}
		if len(output.Rows) != 1 || len(output.Rows[0]) != 1 {
			t.Fatalf("B1 query returned unexpected range: %s", snapshot.Content)
		}
		return output.Rows[0][0].Value, output.Rows[0][0].Formula
	}

	if _, err := invoke("spreadsheet.cells.set", spreadsheetCellsSetPayloadSheet{Sheet: "Sheet1", Edits: []spreadsheetCellEditFixture{
		{Row: 1, Column: 1, Value: "1"},
		{Row: 2, Column: 1, Value: "2"},
	}}); err != nil {
		t.Fatalf("seed formula inputs: %v", err)
	}
	if _, err := invoke("spreadsheet.formula.set", spreadsheetFormulaSetPayload{Sheet: "Sheet1", Cell: "B1", Formula: "=SUM(A1:A2)"}); err != nil {
		t.Fatalf("set formula: %v", err)
	}
	if value, formula := inspectFormula(); value != "3" || formula != "SUM(A1:A2)" {
		t.Fatalf("formula before input edit = value %q formula %q; want 3 and SUM(A1:A2)", value, formula)
	}
	if _, err := invoke("spreadsheet.cell.set", spreadsheetCellSetPayload{Sheet: "Sheet1", Cell: "A1", Value: "5"}); err != nil {
		t.Fatalf("change formula input: %v", err)
	}

	before := mustGetDocumentInfo(t, runtime, documentID)
	result, err := invoke("spreadsheet.workbook.recalculate", spreadsheetWorkbookRecalculatePayload{})
	if err != nil {
		t.Fatalf("InvokeCapability(spreadsheet.workbook.recalculate): %v", err)
	}
	var data struct {
		Changed                 bool `json:"changed"`
		FormulaCacheInvalidated bool `json:"formula_cache_invalidated"`
		ExternalDataRefreshed   bool `json:"external_data_refreshed"`
	}
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("decode recalculate result: %v", err)
	}
	updated := mustGetDocumentInfo(t, runtime, documentID)
	if !data.Changed || !data.FormulaCacheInvalidated || data.ExternalDataRefreshed || result.Revision != before.Revision+1 || updated.Revision != result.Revision || !updated.Dirty {
		t.Fatalf("recalculate data=%+v result revision=%d before=%+v updated=%+v; want cache invalidation without external data refresh", data, result.Revision, before, updated)
	}
	if value, formula := inspectFormula(); value != "7" || formula != "SUM(A1:A2)" {
		t.Fatalf("formula after recalculation = value %q formula %q; want 7 and SUM(A1:A2)", value, formula)
	}

	path := filepath.Join(t.TempDir(), "recalculated.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs after recalculation: %v", err)
	}
	if saved := mustGetDocumentInfo(t, runtime, documentID); saved.Dirty {
		t.Fatalf("saved recalculated workbook remains dirty: %+v", saved)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after recalculation: %v", err)
	}
	documentID = reopened.DocumentID
	if reopened.Dirty {
		t.Fatalf("reopened workbook is dirty: %+v", reopened)
	}
	if value, formula := inspectFormula(); value != "7" || formula != "SUM(A1:A2)" {
		t.Fatalf("formula after save/reopen = value %q formula %q; want 7 and SUM(A1:A2)", value, formula)
	}
}
