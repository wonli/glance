package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"glance/internal/document"
)

type spreadsheetFilterViewHandler struct {
	intents []ViewIntent
}
type handleViewIntentPayload struct {
	WindowId    uint64 `json:"window_id"`
	Operation   string `json:"operation"`
	Sheet       string `json:"sheet"`
	RowsVisible int    `json:"rows_visible"`
	Changed     bool   `json:"changed"`
}

func (h *spreadsheetFilterViewHandler) HandleViewIntent(_ context.Context, intent ViewIntent) (json.RawMessage, error) {
	h.intents = append(h.intents, intent)
	var input struct {
		Operation   string `json:"operation"`
		Sheet       string `json:"sheet"`
		VisibleRows []int  `json:"visible_rows"`
	}
	if err := json.Unmarshal(intent.Arguments, &input); err != nil {
		return nil, err
	}
	return json.Marshal(handleViewIntentPayload{WindowId: intent.WindowID, Operation: input.Operation, Sheet: input.Sheet, RowsVisible: len(input.VisibleRows), Changed: true})
}

type spreadsheetFilterApplyPayload struct {
	Operation string `json:"operation"`
	Sheet     string `json:"sheet"`
}

type spreadsheetFilterApplySetFromRowPayload struct {
	Operation string             `json:"operation"`
	Sheet     string             `json:"sheet"`
	Bounds    document.CellRange `json:"bounds"`
	Column    int                `json:"column"`
	SourceRow int                `json:"source_row"`
}

type spreadsheetFilterApplySetValuePayload struct {
	Operation string             `json:"operation"`
	Sheet     string             `json:"sheet"`
	Bounds    document.CellRange `json:"bounds"`
	Column    int                `json:"column"`
	Value     string             `json:"value"`
}

func TestSpreadsheetFilterCapabilityReadsExplicitRangeAndRoutesWindowIntent(t *testing.T) {
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
	invoke := func(capabilityID string, arguments any, view ViewContext) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		info := mustGetDocumentInfo(t, runtime, documentID)
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("xlsx-filter-%d", requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision, View: view,
			Operator:  Operator{ID: "filter-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}
	seed := []spreadsheetCellEditFixture{
		{Row: 1, Column: 1, Value: "员工"}, {Row: 1, Column: 2, Value: "团队"},
		{Row: 2, Column: 1, Value: "甲"}, {Row: 2, Column: 2, Value: "东区"},
		{Row: 3, Column: 1, Value: "乙"}, {Row: 3, Column: 2, Value: "西区"},
		{Row: 4, Column: 1, Value: "丙"}, {Row: 4, Column: 2, Value: "东区"},
	}
	seedInfo := mustGetDocumentInfo(t, runtime, documentID)
	if _, err := invoke("spreadsheet.cells.set", spreadsheetCellsSetPayload{Sheet: "Sheet1", Edits: seed}, ViewContext{}); err != nil {
		t.Fatalf("seed filter fixture: %v", err)
	}
	seeded := mustGetDocumentInfo(t, runtime, documentID)
	handler := &spreadsheetFilterViewHandler{}
	unregister := runtime.RegisterViewIntentHandler(41, handler)
	t.Cleanup(unregister)
	view := ViewContext{WindowID: 41, DocumentID: documentID, Revision: seeded.Revision}
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 4, EndColumn: 2}

	_, err = invoke("spreadsheet.filter.apply", spreadsheetFilterApplySetValuePayload{Operation: "set_value", Sheet: "Sheet1", Bounds: bounds, Column: 2, Value: "东区"}, ViewContext{})
	if !errors.Is(err, ErrNoAttachedView) {
		t.Fatalf("filter without attached view error=%v; want ErrNoAttachedView", err)
	}
	if len(handler.intents) != 0 {
		t.Fatalf("no-view request was routed: %+v", handler.intents)
	}

	before := mustGetDocumentInfo(t, runtime, documentID)
	setValueResult, err := invoke("spreadsheet.filter.apply", spreadsheetFilterApplySetValuePayload{Operation: "set_value", Sheet: "Sheet1", Bounds: bounds, Column: 2, Value: "东区"}, view)
	if err != nil {
		t.Fatalf("InvokeCapability(spreadsheet.filter.apply set_value): %v", err)
	}
	assertFilterIntent := func(index int, wantOperation string, wantVisible []int) {
		t.Helper()
		if len(handler.intents) <= index {
			t.Fatalf("filter intent %d was not routed; intents=%+v", index, handler.intents)
		}
		intent := handler.intents[index]
		if intent.WindowID != 41 || intent.DocumentID != documentID || intent.Revision != before.Revision || intent.Action != "spreadsheet.filter.apply" {
			t.Fatalf("filter intent context = %+v", intent)
		}
		var payload struct {
			Operation   string             `json:"operation"`
			Sheet       string             `json:"sheet"`
			Bounds      document.CellRange `json:"bounds"`
			Column      int                `json:"column"`
			Value       string             `json:"value"`
			VisibleRows []int              `json:"visible_rows"`
		}
		if err := json.Unmarshal(intent.Arguments, &payload); err != nil {
			t.Fatalf("decode filter intent: %v; arguments=%s", err, intent.Arguments)
		}
		if payload.Operation != wantOperation || payload.Sheet != "Sheet1" {
			t.Fatalf("filter intent target = %+v", payload)
		}
		if wantOperation == "set" && (payload.Bounds != bounds || payload.Column != 2 || payload.Value != "东区" || !equalIntSlice(payload.VisibleRows, wantVisible)) {
			t.Fatalf("filter intent predicate = %+v; want bounds=%+v column=2 value=东区 visible=%v", payload, bounds, wantVisible)
		}
	}
	assertFilterIntent(0, "set", []int{2, 4})
	var setOutput struct {
		WindowID    uint64 `json:"window_id"`
		Operation   string `json:"operation"`
		Sheet       string `json:"sheet"`
		RowsVisible int    `json:"rows_visible"`
		Changed     bool   `json:"changed"`
	}
	if err := json.Unmarshal(setValueResult.Data, &setOutput); err != nil {
		t.Fatalf("decode filter result: %v; data=%s", err, setValueResult.Data)
	}
	if setOutput.WindowID != 41 || setOutput.Operation != "set" || setOutput.Sheet != "Sheet1" || setOutput.RowsVisible != 2 || !setOutput.Changed || setValueResult.Revision != before.Revision {
		t.Fatalf("filter result=%+v revision=%d; before=%+v", setOutput, setValueResult.Revision, before)
	}

	fromRowResult, err := invoke("spreadsheet.filter.apply", spreadsheetFilterApplySetFromRowPayload{Operation: "set_from_row", Sheet: "Sheet1", Bounds: bounds, Column: 2, SourceRow: 2}, view)
	if err != nil {
		t.Fatalf("InvokeCapability(spreadsheet.filter.apply set_from_row): %v", err)
	}
	assertFilterIntent(1, "set", []int{2, 4})
	if fromRowResult.Revision != before.Revision {
		t.Fatalf("set_from_row changed document revision: before=%d result=%d", before.Revision, fromRowResult.Revision)
	}

	if _, err := invoke("spreadsheet.filter.apply", spreadsheetFilterApplySetFromRowPayload{Operation: "set_from_row", Sheet: "Sheet1", Bounds: bounds, Column: 2, SourceRow: 1}, view); err == nil {
		t.Fatal("filter accepted header row as its criteria source")
	}
	if len(handler.intents) != 2 {
		t.Fatalf("invalid filter request reached the view: %+v", handler.intents)
	}
	clearResult, err := invoke("spreadsheet.filter.apply", spreadsheetFilterApplyPayload{Operation: "clear", Sheet: "Sheet1"}, view)
	if err != nil {
		t.Fatalf("InvokeCapability(spreadsheet.filter.apply clear): %v", err)
	}
	assertFilterIntent(2, "clear", nil)
	if clearResult.Revision != before.Revision {
		t.Fatalf("clear filter changed document revision: before=%d result=%d", before.Revision, clearResult.Revision)
	}
	if after := mustGetDocumentInfo(t, runtime, documentID); after.Revision != before.Revision || after.Dirty != before.Dirty || before.Revision != seedInfo.Revision+1 {
		t.Fatalf("window filtering mutated workbook: seed=%+v before=%+v after=%+v", seedInfo, before, after)
	}
}

func equalIntSlice(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
