package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"glance/internal/document"
)

type testSpreadsheetstructureOperationNewNameArguments struct {
	Operation string `json:"operation"`
	NewName   string `json:"new_name"`
}

func TestSpreadsheetStructureCapabilityRequiresHumanAndRoundTrips(t *testing.T) {
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
	stageCreateSheet := func(name string) (CapabilityInvocationResult, string) {
		t.Helper()
		requestNumber++
		info := mustGetDocumentInfo(t, runtime, documentID)
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "xlsx-structure-" + name, DocumentID: documentID,
			CapabilityID: "spreadsheet.structure.edit", ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "structure-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, testSpreadsheetstructureOperationNewNameArguments{Operation: "create_sheet", NewName: name}),
		})
		if invokeErr != nil {
			t.Fatalf("stage sheet creation %q: %v", name, invokeErr)
		}
		var staged struct {
			Status         string `json:"status"`
			ConfirmationID string `json:"confirmation_id"`
			CapabilityID   string `json:"capability_id"`
		}
		if err := json.Unmarshal(result.Data, &staged); err != nil {
			t.Fatalf("decode staged structure edit: %v; data=%s", err, result.Data)
		}
		if staged.Status != "confirmation_required" || staged.ConfirmationID == "" || staged.CapabilityID != "spreadsheet.structure.edit" {
			t.Fatalf("structure edit was not staged for a human: %+v; data=%s", staged, result.Data)
		}
		return result, staged.ConfirmationID
	}
	assertSheetMissing := func(sheet string) {
		t.Helper()
		session, err := runtime.session(documentID)
		if err != nil {
			t.Fatalf("get spreadsheet session: %v", err)
		}
		reader, ok := session.adapter.(SpreadsheetRangeReaderCapability)
		if !ok {
			t.Fatal("spreadsheet adapter does not expose range reads")
		}
		if _, err := reader.ReadSpreadsheetRange(ctx, sheet, document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}); err == nil {
			t.Fatalf("worksheet %q exists before approval", sheet)
		}
	}

	beforeReject := mustGetDocumentInfo(t, runtime, documentID)
	_, rejectID := stageCreateSheet("Rejected")
	if staged := mustGetDocumentInfo(t, runtime, documentID); staged.Revision != beforeReject.Revision || staged.Dirty != beforeReject.Dirty {
		t.Fatalf("Agent request mutated workbook before human decision: before=%+v staged=%+v", beforeReject, staged)
	}
	assertSheetMissing("Rejected")
	if err := runtime.ResolveConfirmation(ctx, rejectID, false, Operator{ID: "human-reviewer", Kind: OperatorHuman}); err != nil {
		t.Fatalf("reject sheet creation: %v", err)
	}
	waitForConfirmationStatus(t, runtime, rejectID, ConfirmationRejected)
	if rejected := mustGetDocumentInfo(t, runtime, documentID); rejected.Revision != beforeReject.Revision || rejected.Dirty != beforeReject.Dirty {
		t.Fatalf("rejected sheet creation mutated workbook: before=%+v after=%+v", beforeReject, rejected)
	}
	assertSheetMissing("Rejected")

	beforeApprove := mustGetDocumentInfo(t, runtime, documentID)
	_, approveID := stageCreateSheet("Agent Report")
	if err := runtime.ResolveConfirmation(ctx, approveID, true, Operator{ID: "human-reviewer", Kind: OperatorHuman}); err != nil {
		t.Fatalf("approve sheet creation: %v", err)
	}
	completed := waitForConfirmationStatus(t, runtime, approveID, ConfirmationCompleted)
	if completed.Result == nil || completed.Result.Revision != beforeApprove.Revision+1 {
		t.Fatalf("approved structure result = %+v; want one revision", completed)
	}
	var result struct {
		Changed   bool   `json:"changed"`
		Operation string `json:"operation"`
		NewName   string `json:"new_name"`
	}
	if err := json.Unmarshal(completed.Result.Data, &result); err != nil {
		t.Fatalf("decode approved structure result: %v; data=%s", err, completed.Result.Data)
	}
	if !result.Changed || result.Operation != "create_sheet" || result.NewName != "Agent Report" {
		t.Fatalf("approved structure result = %+v; data=%s", result, completed.Result.Data)
	}
	created := mustGetDocumentInfo(t, runtime, documentID)
	if created.Revision != beforeApprove.Revision+1 || !created.Dirty {
		t.Fatalf("approved sheet creation state: before=%+v after=%+v", beforeApprove, created)
	}
	session, err := runtime.session(documentID)
	if err != nil {
		t.Fatalf("get created workbook session: %v", err)
	}
	reader, ok := session.adapter.(SpreadsheetRangeReaderCapability)
	if !ok {
		t.Fatal("spreadsheet adapter does not expose range reads")
	}
	if _, err := reader.ReadSpreadsheetRange(ctx, "Agent Report", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}); err != nil {
		t.Fatalf("read newly created worksheet: %v", err)
	}

	path := filepath.Join(t.TempDir(), "agent-report.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after structure edit: %v", err)
	}
	documentID = reopened.DocumentID
	if reopened.Dirty {
		t.Fatalf("reopened workbook is dirty: %+v", reopened)
	}
	session, err = runtime.session(documentID)
	if err != nil {
		t.Fatalf("get reopened workbook session: %v", err)
	}
	reader, ok = session.adapter.(SpreadsheetRangeReaderCapability)
	if !ok {
		t.Fatal("reopened spreadsheet adapter does not expose range reads")
	}
	if _, err := reader.ReadSpreadsheetRange(ctx, "Agent Report", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}); err != nil {
		t.Fatalf("read worksheet after reopen: %v", err)
	}
}
