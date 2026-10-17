package application

import (
	"context"
	"encoding/json"
	"testing"

	"glance/internal/document"
)

func TestSpreadsheetFormulaRangeCapabilityAdjustsReferencesAndTracksRevision(t *testing.T) {
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
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2}
	invoke := func(requestID string) (CapabilityInvocationResult, error) {
		t.Helper()
		info, getErr := runtime.GetDocument(ctx, doc.DocumentID)
		if getErr != nil {
			t.Fatalf("GetDocument: %v", getErr)
		}
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: doc.DocumentID,
			CapabilityID: "spreadsheet.formulas.fill", ExpectedRevision: info.Revision,
			Operator: Operator{ID: "formula-range-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, struct {
				Sheet  string `json:"sheet"`
				Active struct {
					Row    int `json:"row"`
					Column int `json:"column"`
				} `json:"active"`
				Bounds  document.CellRange `json:"bounds"`
				Formula string             `json:"formula"`
			}{"Sheet1", struct {
				Row    int `json:"row"`
				Column int `json:"column"`
			}{1, 1}, bounds, "=SUM(A1:A2)"}),
		})
	}
	readRevision := func() uint64 {
		t.Helper()
		info, getErr := runtime.GetDocument(ctx, doc.DocumentID)
		if getErr != nil {
			t.Fatalf("GetDocument: %v", getErr)
		}
		return info.Revision
	}
	inspect := func() [][]string {
		t.Helper()
		snapshot, inspectErr := runtime.Queries().InspectDocument(ctx, InspectRequest{
			DocumentID: doc.DocumentID, Sheet: "Sheet1", Range: &bounds,
		})
		if inspectErr != nil {
			t.Fatalf("InspectDocument(A1:B2): %v", inspectErr)
		}
		var output struct {
			Rows [][]struct {
				Formula string `json:"formula"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(snapshot.Content, &output); err != nil {
			t.Fatalf("decode formula range: %v; content=%s", err, snapshot.Content)
		}
		formulas := make([][]string, len(output.Rows))
		for row := range output.Rows {
			formulas[row] = make([]string, len(output.Rows[row]))
			for column := range output.Rows[row] {
				formulas[row][column] = output.Rows[row][column].Formula
			}
		}
		return formulas
	}

	before := readRevision()
	result, err := invoke("xlsx-formula-range-fill")
	if err != nil {
		t.Fatalf("InvokeCapability(spreadsheet.formulas.fill): %v", err)
	}
	var output struct {
		Changed bool               `json:"changed"`
		Sheet   string             `json:"sheet"`
		Bounds  document.CellRange `json:"bounds"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode formula range result: %v; data=%s", err, result.Data)
	}
	if !output.Changed || output.Sheet != "Sheet1" || output.Bounds != bounds {
		t.Fatalf("formula range result=%+v; want changed Sheet1 A1:B2", output)
	}
	if got, want := readRevision(), before+1; got != want {
		t.Fatalf("revision after formula range fill=%d; want %d", got, want)
	}
	want := [][]string{{"SUM(A1:A2)", "SUM(B1:B2)"}, {"SUM(A2:A3)", "SUM(B2:B3)"}}
	if got := inspect(); !equalStringMatrix(got, want) {
		t.Fatalf("formulas after fill=%v; want %v", got, want)
	}

	result, err = invoke("xlsx-formula-range-fill-noop")
	if err != nil {
		t.Fatalf("Invoke identical formula range fill: %v", err)
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode identical formula range result: %v; data=%s", err, result.Data)
	}
	if output.Changed {
		t.Fatalf("identical formula range result=%+v; want no-op", output)
	}
	if got, want := readRevision(), before+1; got != want {
		t.Fatalf("revision after identical formula fill=%d; want %d", got, want)
	}
}

func equalStringMatrix(got, want [][]string) bool {
	if len(got) != len(want) {
		return false
	}
	for row := range got {
		if len(got[row]) != len(want[row]) {
			return false
		}
		for column := range got[row] {
			if got[row][column] != want[row][column] {
				return false
			}
		}
	}
	return true
}
