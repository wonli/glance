package application

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestSpreadsheetCellDateCapabilityTracksRevisionAndNoOp(t *testing.T) {
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
	invoke := func(arguments any) CapabilityInvocationResult {
		t.Helper()
		requestNumber++
		info, err := runtime.GetDocument(ctx, doc.DocumentID)
		if err != nil {
			t.Fatalf("GetDocument: %v", err)
		}
		result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("cell-date-%d", requestNumber), DocumentID: doc.DocumentID,
			CapabilityID: "spreadsheet.cell.date.insert", ExpectedRevision: info.Revision,
			Operator: Operator{ID: "cell-date-test", Kind: OperatorExternalAgent}, Arguments: mustJSON(t, arguments),
		})
		if err != nil {
			t.Fatalf("InvokeCapability(spreadsheet.cell.date.insert): %v", err)
		}
		return result
	}
	before := mustGetDocumentInfo(t, runtime, doc.DocumentID).Revision
	arguments := struct {
		Sheet string `json:"sheet"`
		Cell  string `json:"cell"`
		Date  string `json:"date"`
	}{"Sheet1", "B2", "2026-10-17"}
	result := invoke(arguments)
	var output struct {
		Changed bool   `json:"changed"`
		Date    string `json:"date"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode date result: %v; data=%s", err, result.Data)
	}
	if !output.Changed || output.Date != arguments.Date || result.Revision != before+1 {
		t.Fatalf("date result=%+v revision=%d; want changed, %s, revision %d", output, result.Revision, arguments.Date, before+1)
	}
	result = invoke(arguments)
	if err := json.Unmarshal(result.Data, &output); err != nil || output.Changed {
		t.Fatalf("repeated date insert changed=%t err=%v data=%s; want no-op", output.Changed, err, result.Data)
	}
	if got := mustGetDocumentInfo(t, runtime, doc.DocumentID).Revision; got != before+1 {
		t.Fatalf("revision after repeated date insert=%d; want %d", got, before+1)
	}
}
