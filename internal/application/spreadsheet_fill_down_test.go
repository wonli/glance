package application

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"glance/internal/document"
)

func TestSpreadsheetFillDownCapabilityTracksRevisionAndNoOp(t *testing.T) {
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
	requestNumber := 0
	invoke := func(capabilityID string, arguments any) CapabilityInvocationResult {
		t.Helper()
		requestNumber++
		info, err := runtime.GetDocument(ctx, doc.DocumentID)
		if err != nil {
			t.Fatalf("GetDocument: %v", err)
		}
		result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("fill-down-%d", requestNumber), DocumentID: doc.DocumentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "fill-down-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
		if err != nil {
			t.Fatalf("InvokeCapability(%s): %v", capabilityID, err)
		}
		return result
	}
	// Seed a constant and two destination values through the ordinary cell command.
	invoke("spreadsheet.cell.set", spreadsheetCellSetPayload{Sheet: "Sheet1", Cell: "A1", Value: "10"})
	for _, cell := range []string{"A2", "A3"} {
		invoke("spreadsheet.cell.set", spreadsheetCellSetPayload{Sheet: "Sheet1", Cell: cell, Value: "old"})
	}
	invoke("spreadsheet.formula.set", testSpreadsheetformulaSheetCellArguments{Sheet: "Sheet1", Cell: "B1", Formula: "=A1*2"})
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 2}
	fillArguments := struct {
		Sheet  string             `json:"sheet"`
		Bounds document.CellRange `json:"bounds"`
	}{"Sheet1", bounds}
	before, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before fill: %v", err)
	}
	result := invoke("spreadsheet.range.fill_down", fillArguments)
	var output struct {
		Changed bool               `json:"changed"`
		Sheet   string             `json:"sheet"`
		Bounds  document.CellRange `json:"bounds"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode fill result: %v; data=%s", err, result.Data)
	}
	if !output.Changed || output.Sheet != "Sheet1" || output.Bounds != bounds || result.Revision != before.Revision+1 {
		t.Fatalf("fill result=%+v revision=%d; before=%+v", output, result.Revision, before)
	}
	query, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Sheet: "Sheet1", Range: &bounds})
	if err != nil {
		t.Fatalf("InspectDocument after fill: %v", err)
	}
	var inspection struct {
		Rows [][]struct {
			Value   string `json:"value"`
			Formula string `json:"formula"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(query.Content, &inspection); err != nil {
		t.Fatalf("decode filled cells: %v; content=%s", err, query.Content)
	}
	want := [][]struct{ value, formula string }{
		{{"10", ""}, {"20", "A1*2"}},
		{{"10", ""}, {"20", "A2*2"}},
		{{"10", ""}, {"20", "A3*2"}},
	}
	if len(inspection.Rows) != len(want) {
		t.Fatalf("filled rows=%d; want %d (%s)", len(inspection.Rows), len(want), query.Content)
	}
	for row := range want {
		for column := range want[row] {
			got := inspection.Rows[row][column]
			if got.Value != want[row][column].value || got.Formula != want[row][column].formula {
				t.Errorf("filled cell [%d,%d]=(%q,%q); want (%q,%q)", row, column, got.Value, got.Formula, want[row][column].value, want[row][column].formula)
			}
		}
	}
	result = invoke("spreadsheet.range.fill_down", fillArguments)
	if err := json.Unmarshal(result.Data, &output); err != nil || output.Changed {
		t.Fatalf("repeated fill changed=%v err=%v data=%s; want no-op", output.Changed, err, result.Data)
	}
	if after := mustGetDocumentInfo(t, runtime, doc.DocumentID); after.Revision != before.Revision+1 {
		t.Fatalf("revision after no-op fill=%d; want %d", after.Revision, before.Revision+1)
	}
	invalid := fillArguments
	invalid.Bounds.EndRow = invalid.Bounds.StartRow
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "fill-down-invalid-one-row", DocumentID: doc.DocumentID,
		CapabilityID: "spreadsheet.range.fill_down", ExpectedRevision: mustGetDocumentInfo(t, runtime, doc.DocumentID).Revision,
		Operator: Operator{ID: "fill-down-test", Kind: OperatorExternalAgent}, Arguments: mustJSON(t, invalid),
	}); err == nil {
		t.Fatal("fill down accepted a one-row range; want invalid arguments")
	}
}
