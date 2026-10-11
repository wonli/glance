package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSpreadsheetHyperlinkCapabilitiesRoundTripAndNoop(t *testing.T) {
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
	operator := Operator{ID: "xlsx-hyperlink-test", Kind: OperatorHuman}
	const sheet = "Sheet1"
	const target = "https://example.org/payroll-policy"

	read := func(documentID string, revision uint64, cell, requestID string) (spreadsheetHyperlinkTestResult, error) {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: documentID, CapabilityID: "spreadsheet.hyperlink.get",
			ExpectedRevision: revision, Operator: operator,
			Arguments: mustJSON(t, map[string]string{"sheet": sheet, "cell": cell}),
		})
		if invokeErr != nil {
			return spreadsheetHyperlinkTestResult{}, invokeErr
		}
		var value spreadsheetHyperlinkTestResult
		if err := json.Unmarshal(result.Data, &value); err != nil {
			return spreadsheetHyperlinkTestResult{}, err
		}
		return value, nil
	}

	initial, err := read(doc.DocumentID, doc.Revision, "A1", "xlsx-hyperlink-initial")
	if err != nil || initial.Exists || initial.Target != "" {
		t.Fatalf("initial hyperlink=%+v error=%v", initial, err)
	}
	set, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-hyperlink-set", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.hyperlink.set",
		ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": sheet, "cell": "a1", "target": target}),
	})
	if err != nil || set.Revision != doc.Revision+1 || !containsJSONText(set.Data, `"changed":true`) || !containsJSONText(set.Data, `"cell":"A1"`) {
		t.Fatalf("set hyperlink: result=%s revision=%d error=%v", set.Data, set.Revision, err)
	}
	link, err := read(doc.DocumentID, set.Revision, "A1", "xlsx-hyperlink-after-set")
	if err != nil || !link.Exists || link.Target != target {
		t.Fatalf("hyperlink after set=%+v error=%v", link, err)
	}
	setNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-hyperlink-set-noop", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.hyperlink.set",
		ExpectedRevision: set.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": sheet, "cell": "A1", "target": target}),
	})
	if err != nil || setNoOp.Revision != set.Revision || !containsJSONText(setNoOp.Data, `"changed":false`) {
		t.Fatalf("repeating identical hyperlink set should be a no-op: result=%s revision=%d error=%v", setNoOp.Data, setNoOp.Revision, err)
	}

	stateBeforeInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid hyperlink: %v", err)
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-hyperlink-invalid-sheet", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.hyperlink.set",
		ExpectedRevision: set.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": "不存在的工作表", "cell": "A1", "target": target}),
	}); err == nil {
		t.Fatal("hyperlink set accepted an unknown worksheet")
	}
	stateAfterInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterInvalid.Revision != stateBeforeInvalid.Revision || stateAfterInvalid.Dirty != stateBeforeInvalid.Dirty {
		t.Fatalf("invalid hyperlink target changed workbook state: before=%+v after=%+v error=%v", stateBeforeInvalid, stateAfterInvalid, err)
	}
	removeNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-hyperlink-remove-noop", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.hyperlink.remove",
		ExpectedRevision: set.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": sheet, "cell": "B2"}),
	})
	if err != nil || removeNoOp.Revision != set.Revision || !containsJSONText(removeNoOp.Data, `"changed":false`) {
		t.Fatalf("removing absent hyperlink should be a no-op: result=%s revision=%d error=%v", removeNoOp.Data, removeNoOp.Revision, err)
	}

	firstPath := filepath.Join(t.TempDir(), "spreadsheet-hyperlink-added.xlsx")
	firstSaved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, firstPath)
	if err != nil || firstSaved.SourcePath != canonicalPath(firstPath) || firstSaved.Dirty {
		t.Fatalf("save workbook with hyperlink: saved=%+v error=%v", firstSaved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close workbook before hyperlink reopen: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, firstPath)
	if err != nil {
		t.Fatalf("reopen workbook with hyperlink: %v", err)
	}
	persisted, err := read(reopened.DocumentID, reopened.Revision, "A1", "xlsx-hyperlink-after-reopen")
	if err != nil || !persisted.Exists || persisted.Target != target {
		t.Fatalf("hyperlink did not persist after reopen: link=%+v error=%v", persisted, err)
	}
	removed, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-hyperlink-remove", DocumentID: reopened.DocumentID, CapabilityID: "spreadsheet.hyperlink.remove",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": sheet, "cell": "A1"}),
	})
	if err != nil || removed.Revision != reopened.Revision+1 || !containsJSONText(removed.Data, `"changed":true`) {
		t.Fatalf("remove hyperlink: result=%s revision=%d error=%v", removed.Data, removed.Revision, err)
	}
	removedNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-hyperlink-remove-repeat", DocumentID: reopened.DocumentID, CapabilityID: "spreadsheet.hyperlink.remove",
		ExpectedRevision: removed.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": sheet, "cell": "A1"}),
	})
	if err != nil || removedNoOp.Revision != removed.Revision || !containsJSONText(removedNoOp.Data, `"changed":false`) {
		t.Fatalf("repeating hyperlink removal should be a no-op: result=%s revision=%d error=%v", removedNoOp.Data, removedNoOp.Revision, err)
	}
	absent, err := read(reopened.DocumentID, removed.Revision, "A1", "xlsx-hyperlink-after-remove")
	if err != nil || absent.Exists || absent.Target != "" {
		t.Fatalf("hyperlink after removal=%+v error=%v", absent, err)
	}

	secondPath := filepath.Join(t.TempDir(), "spreadsheet-hyperlink-removed.xlsx")
	secondSaved, err := runtime.SaveDocumentAs(ctx, reopened.DocumentID, secondPath)
	if err != nil || secondSaved.SourcePath != canonicalPath(secondPath) || secondSaved.Dirty {
		t.Fatalf("save workbook after hyperlink removal: saved=%+v error=%v", secondSaved, err)
	}
	if err := runtime.CloseDocument(ctx, reopened.DocumentID); err != nil {
		t.Fatalf("close workbook before delete reopen: %v", err)
	}
	deletedReopen, err := runtime.OpenDocument(ctx, secondPath)
	if err != nil {
		t.Fatalf("reopen workbook after hyperlink removal: %v", err)
	}
	deletedLink, err := read(deletedReopen.DocumentID, deletedReopen.Revision, "A1", "xlsx-hyperlink-after-remove-reopen")
	if err != nil || deletedLink.Exists {
		t.Fatalf("removed hyperlink reappeared after reopen: link=%+v error=%v", deletedLink, err)
	}
}

type spreadsheetHyperlinkTestResult struct {
	Exists  bool   `json:"exists"`
	Target  string `json:"target"`
	Tooltip string `json:"tooltip"`
}
