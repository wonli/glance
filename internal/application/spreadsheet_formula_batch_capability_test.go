package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"glance/internal/document"
)

func TestSpreadsheetFormulaBatchCapabilityIsAtomicAndRevisionChecked(t *testing.T) {
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
	invoke := func(edits []document.CellFormulaEdit) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		info, getErr := runtime.GetDocument(ctx, doc.DocumentID)
		if getErr != nil {
			t.Fatalf("GetDocument before formula batch: %v", getErr)
		}
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("xlsx-formula-batch-%d", requestNumber), DocumentID: doc.DocumentID,
			CapabilityID: "spreadsheet.formulas.set", ExpectedRevision: info.Revision,
			Operator: Operator{ID: "formula-batch-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, struct {
				Sheet string                     `json:"sheet"`
				Edits []document.CellFormulaEdit `json:"edits"`
			}{"Sheet1", edits}),
		})
	}
	inspectFormulas := func(bounds document.CellRange) [][]struct {
		Formula string `json:"formula"`
	} {
		t.Helper()
		snapshot, inspectErr := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Sheet: "Sheet1", Range: &bounds})
		if inspectErr != nil {
			t.Fatalf("InspectDocument formula batch: %v", inspectErr)
		}
		var output struct {
			Rows [][]struct {
				Formula string `json:"formula"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(snapshot.Content, &output); err != nil {
			t.Fatalf("decode formula query: %v; content=%s", err, snapshot.Content)
		}
		return output.Rows
	}
	readRevision := func() uint64 {
		t.Helper()
		info, getErr := runtime.GetDocument(ctx, doc.DocumentID)
		if getErr != nil {
			t.Fatalf("GetDocument: %v", getErr)
		}
		return info.Revision
	}

	beforeRevision := readRevision()
	edits := []document.CellFormulaEdit{
		{Row: 1, Column: 1, Formula: "=SUM(1,2)"},
		{Row: 1, Column: 2, Formula: "=AVERAGE(1,2)"},
	}
	result, err := invoke(edits)
	if err != nil {
		t.Fatalf("InvokeCapability(spreadsheet.formulas.set): %v", err)
	}
	var output struct {
		Changed        bool   `json:"changed"`
		Sheet          string `json:"sheet"`
		RequestedCount int    `json:"requested_count"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode formula batch result: %v; data=%s", err, result.Data)
	}
	if !output.Changed || output.Sheet != "Sheet1" || output.RequestedCount != len(edits) {
		t.Fatalf("formula batch output=%+v; want changed, Sheet1, requested_count=%d", output, len(edits))
	}
	if got := readRevision(); got != beforeRevision+1 {
		t.Fatalf("formula batch revision=%d; want one advance from %d", got, beforeRevision)
	}
	rows := inspectFormulas(document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2})
	if len(rows) != 1 || len(rows[0]) != 2 || rows[0][0].Formula != "SUM(1,2)" || rows[0][1].Formula != "AVERAGE(1,2)" {
		t.Fatalf("stored batch formulas=%+v; want SUM and AVERAGE", rows)
	}

	result, err = invoke(edits)
	if err != nil {
		t.Fatalf("invoke identical formula batch: %v", err)
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode no-op formula batch result: %v", err)
	}
	if output.Changed || readRevision() != beforeRevision+1 {
		t.Fatalf("identical formula batch changed document: output=%+v revision=%d", output, readRevision())
	}

	beforeInvalidRevision := readRevision()
	invalidBatches := [][]document.CellFormulaEdit{
		{{Row: 2, Column: 1, Formula: "=SUM(1,2)"}, {Row: 2, Column: 1, Formula: "=SUM(3,4)"}},
		{{Row: 3, Column: 1, Formula: "=" + strings.Repeat("😀", 4096)}},
		{{Row: 1048577, Column: 1, Formula: "=SUM(1,2)"}},
	}
	for _, invalid := range invalidBatches {
		if _, err := invoke(invalid); err == nil {
			t.Fatalf("invalid formula batch unexpectedly succeeded: %+v", invalid)
		}
		if got := readRevision(); got != beforeInvalidRevision {
			t.Fatalf("invalid formula batch advanced revision to %d; want %d", got, beforeInvalidRevision)
		}
	}
	rows = inspectFormulas(document.CellRange{StartRow: 2, StartColumn: 1, EndRow: 3, EndColumn: 1})
	if len(rows) != 2 || rows[0][0].Formula != "" || rows[1][0].Formula != "" {
		t.Fatalf("invalid batches partially wrote formulas: %+v", rows)
	}
}
