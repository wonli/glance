package application

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestSpreadsheetCellTimeCapabilityTracksRevisionAndNoOp(t *testing.T) {
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
	invoke := func() CapabilityInvocationResult {
		t.Helper()
		requestNumber++
		info, err := runtime.GetDocument(ctx, doc.DocumentID)
		if err != nil {
			t.Fatalf("GetDocument: %v", err)
		}
		result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("cell-time-%d", requestNumber), DocumentID: doc.DocumentID,
			CapabilityID: "spreadsheet.cell.time.insert", ExpectedRevision: info.Revision,
			Operator: Operator{ID: "cell-time-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, struct {
				Sheet string `json:"sheet"`
				Cell  string `json:"cell"`
				Time  string `json:"time"`
			}{"Sheet1", "B2", "13:47"}),
		})
		if err != nil {
			t.Fatalf("InvokeCapability(spreadsheet.cell.time.insert): %v", err)
		}
		return result
	}
	before := mustGetDocumentInfo(t, runtime, doc.DocumentID).Revision
	result := invoke()
	var output struct {
		Changed bool   `json:"changed"`
		Time    string `json:"time"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode time result: %v; data=%s", err, result.Data)
	}
	if !output.Changed || output.Time != "13:47" || result.Revision != before+1 {
		t.Fatalf("time result=%+v revision=%d; want changed, 13:47, revision %d", output, result.Revision, before+1)
	}
	result = invoke()
	if err := json.Unmarshal(result.Data, &output); err != nil || output.Changed {
		t.Fatalf("repeated time insert changed=%t err=%v data=%s; want no-op", output.Changed, err, result.Data)
	}
	if got := mustGetDocumentInfo(t, runtime, doc.DocumentID).Revision; got != before+1 {
		t.Fatalf("revision after repeated time insert=%d; want %d", got, before+1)
	}
}
