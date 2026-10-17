package application

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestSpreadsheetFillRightCapabilityTracksRevisionAndNoOp(t *testing.T) {
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
			RequestID: fmt.Sprintf("fill-right-%d", requestNumber), DocumentID: doc.DocumentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator: Operator{ID: "fill-right-test", Kind: OperatorExternalAgent}, Arguments: mustJSON(t, arguments),
		})
		if err != nil {
			t.Fatalf("InvokeCapability(%s): %v", capabilityID, err)
		}
		return result
	}
	invoke("spreadsheet.cell.set", spreadsheetCellSetPayload{Sheet: "Sheet1", Cell: "A1", Value: "10"})
	invoke("spreadsheet.cell.set", spreadsheetCellSetPayload{Sheet: "Sheet1", Cell: "A2", Value: "label"})
	invoke("spreadsheet.formula.set", testSpreadsheetformulaSheetCellArguments{Sheet: "Sheet1", Cell: "A1", Formula: "=B1*2+$D$1"})
	bounds := struct {
		StartRow    int `json:"start_row"`
		StartColumn int `json:"start_column"`
		EndRow      int `json:"end_row"`
		EndColumn   int `json:"end_column"`
	}{1, 1, 2, 3}
	arguments := struct {
		Sheet  string `json:"sheet"`
		Bounds any    `json:"bounds"`
	}{"Sheet1", bounds}
	before, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before fill: %v", err)
	}
	result := invoke("spreadsheet.range.fill_right", arguments)
	var output struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode fill result: %v; data=%s", err, result.Data)
	}
	if !output.Changed || result.Revision != before.Revision+1 {
		t.Fatalf("fill result changed=%t revision=%d; before revision=%d", output.Changed, result.Revision, before.Revision)
	}
	after, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Sheet: "Sheet1"})
	if err != nil {
		t.Fatalf("InspectDocument after fill: %v", err)
	}
	var cells struct {
		Rows [][]struct {
			Value   string `json:"value"`
			Formula string `json:"formula"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(after.Content, &cells); err != nil {
		t.Fatalf("decode filled cells: %v; content=%s", err, after.Content)
	}
	if len(cells.Rows) < 2 || len(cells.Rows[0]) < 3 || len(cells.Rows[1]) < 3 {
		t.Fatalf("inspected rows do not include A1:C2: %s", after.Content)
	}
	if got := []string{cells.Rows[0][0].Formula, cells.Rows[0][1].Formula, cells.Rows[0][2].Formula}; fmt.Sprint(got) != "[B1*2+$D$1 C1*2+$D$1 D1*2+$D$1]" {
		t.Errorf("filled formulas = %v; want relative column references shifted", got)
	}
	for column := 0; column < 3; column++ {
		if got := cells.Rows[1][column].Value; got != "label" {
			t.Errorf("row 2 column %d value = %q; want copied leftmost value", column+1, got)
		}
	}
	result = invoke("spreadsheet.range.fill_right", arguments)
	if err := json.Unmarshal(result.Data, &output); err != nil || output.Changed {
		t.Fatalf("repeated fill changed=%t err=%v data=%s; want no-op", output.Changed, err, result.Data)
	}
	if got := mustGetDocumentInfo(t, runtime, doc.DocumentID).Revision; got != before.Revision+1 {
		t.Fatalf("revision after no-op fill=%d; want %d", got, before.Revision+1)
	}
}
