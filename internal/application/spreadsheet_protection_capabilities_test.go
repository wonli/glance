package application

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

type testSpreadsheetprotectionScopeEnabledArguments struct {
	Scope    string `json:"scope"`
	Enabled  bool   `json:"enabled"`
	Sheet    string `json:"sheet,omitempty"`
	Password string `json:"password,omitempty"`
}

type testSpreadsheetprotectionScopeArguments struct {
	Scope string `json:"scope"`
	Sheet string `json:"sheet,omitempty"`
}

func TestSpreadsheetProtectionCapabilitiesRequireHumanAndRoundTripWithoutLeakingPassword(t *testing.T) {
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
			RequestID: fmt.Sprintf("xlsx-protection-%d", requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "protection-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}
	getProtection := func(scope, sheet string) bool {
		t.Helper()
		arguments := testSpreadsheetprotectionScopeArguments{Scope: scope, Sheet: sheet}
		result, invokeErr := invoke("spreadsheet.protection.get", arguments)
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(spreadsheet.protection.get): %v", invokeErr)
		}
		var output struct {
			Scope   string `json:"scope"`
			Sheet   string `json:"sheet"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode protection status: %v; data=%s", err, result.Data)
		}
		if output.Scope != scope || output.Sheet != sheet {
			t.Fatalf("protection status target = %+v; want scope=%q sheet=%q", output, scope, sheet)
		}
		return output.Enabled
	}
	setProtection := func(scope, sheet string, enabled bool, password string, wantChanged, wantError bool) ConfirmationInfo {
		t.Helper()
		arguments := testSpreadsheetprotectionScopeEnabledArguments{Scope: scope, Enabled: enabled, Sheet: sheet, Password: password}
		before := mustGetDocumentInfo(t, runtime, documentID)
		pending, invokeErr := invoke("spreadsheet.protection.set", arguments)
		if invokeErr != nil {
			t.Fatalf("stage spreadsheet.protection.set: %v", invokeErr)
		}
		if password != "" && strings.Contains(string(pending.Data), password) {
			t.Fatalf("pending response exposed password: %s", pending.Data)
		}
		var staged struct {
			Status         string `json:"status"`
			ConfirmationID string `json:"confirmation_id"`
		}
		if err := json.Unmarshal(pending.Data, &staged); err != nil || staged.Status != "confirmation_required" || staged.ConfirmationID == "" {
			t.Fatalf("protection change did not require human confirmation: %+v; data=%s err=%v", staged, pending.Data, err)
		}
		confirmation, err := runtime.GetConfirmation(ctx, staged.ConfirmationID)
		if err != nil {
			t.Fatalf("GetConfirmation while pending: %v", err)
		}
		confirmationJSON, err := json.Marshal(confirmation)
		if err != nil {
			t.Fatalf("marshal pending confirmation: %v", err)
		}
		if password != "" && strings.Contains(string(confirmationJSON), password) {
			t.Fatalf("pending confirmation exposed password: %s", confirmationJSON)
		}
		if afterStage := mustGetDocumentInfo(t, runtime, documentID); afterStage.Revision != before.Revision || afterStage.Dirty != before.Dirty {
			t.Fatalf("protection changed before human approval: before=%+v after stage=%+v", before, afterStage)
		}
		if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, Operator{ID: "human-reviewer", Kind: OperatorHuman}); err != nil {
			t.Fatalf("human approval: %v", err)
		}
		status := ConfirmationCompleted
		if wantError {
			status = ConfirmationFailed
		}
		completed := waitForConfirmationStatus(t, runtime, staged.ConfirmationID, status)
		completedJSON, err := json.Marshal(completed)
		if err != nil {
			t.Fatalf("marshal completed confirmation: %v", err)
		}
		if password != "" && strings.Contains(string(completedJSON), password) {
			t.Fatalf("completed confirmation exposed password: %s", completedJSON)
		}
		if wantError {
			if completed.Error == "" || password != "" && strings.Contains(completed.Error, password) {
				t.Fatalf("failed confirmation error is empty or exposed password: %+v", completed)
			}
			if afterFailure := mustGetDocumentInfo(t, runtime, documentID); afterFailure.Revision != before.Revision || afterFailure.Dirty != before.Dirty {
				t.Fatalf("failed protection command changed workbook: before=%+v after=%+v", before, afterFailure)
			}
			return completed
		}
		if completed.Result == nil {
			t.Fatalf("approved protection command returned no result: %+v", completed)
		}
		var output struct {
			Scope   string `json:"scope"`
			Sheet   string `json:"sheet"`
			Enabled bool   `json:"enabled"`
			Changed bool   `json:"changed"`
		}
		if err := json.Unmarshal(completed.Result.Data, &output); err != nil {
			t.Fatalf("decode approved protection result: %v; data=%s", err, completed.Result.Data)
		}
		if output.Scope != scope || output.Sheet != sheet || output.Enabled != enabled || output.Changed != wantChanged {
			t.Fatalf("approved protection result = %+v; want scope=%s sheet=%s enabled=%v changed=%v", output, scope, sheet, enabled, wantChanged)
		}
		after := mustGetDocumentInfo(t, runtime, documentID)
		if wantChanged {
			if completed.Result.Revision != before.Revision+1 || after.Revision != completed.Result.Revision || !after.Dirty {
				t.Fatalf("protection mutation state: before=%+v result=%+v after=%+v", before, completed.Result, after)
			}
		} else if completed.Result.Revision != before.Revision || after.Revision != before.Revision || after.Dirty != before.Dirty {
			t.Fatalf("protection no-op changed document state: before=%+v result=%+v after=%+v", before, completed.Result, after)
		}
		return completed
	}

	if getProtection("sheet", "Sheet1") || getProtection("workbook", "") {
		t.Fatal("new workbook unexpectedly starts protected")
	}
	password := "workbook-secret-for-test"
	setProtection("sheet", "Sheet1", true, password, true, false)
	if !getProtection("sheet", "Sheet1") {
		t.Fatal("approved sheet protection was not enabled")
	}
	setProtection("sheet", "Sheet1", true, password, false, false)
	setProtection("sheet", "Sheet1", false, "wrong-secret", false, true)
	if !getProtection("sheet", "Sheet1") {
		t.Fatal("failed unprotect attempt removed sheet protection")
	}

	path := filepath.Join(t.TempDir(), "agent-protection.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs protected workbook: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument protected workbook: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument protected workbook: %v", err)
	}
	documentID = reopened.DocumentID
	if reopened.Dirty || !getProtection("sheet", "Sheet1") {
		t.Fatalf("sheet protection did not survive reopen: document=%+v", reopened)
	}
	setProtection("sheet", "Sheet1", false, password, true, false)
	if getProtection("sheet", "Sheet1") {
		t.Fatal("approved sheet unprotect did not remove protection")
	}
	setProtection("workbook", "", true, "", true, false)
	if !getProtection("workbook", "") {
		t.Fatal("approved workbook protection was not enabled")
	}

	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs workbook protection: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument workbook protection: %v", err)
	}
	reopened, err = runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument workbook protection: %v", err)
	}
	documentID = reopened.DocumentID
	if reopened.Dirty || !getProtection("workbook", "") {
		t.Fatalf("workbook structure protection did not survive reopen: document=%+v", reopened)
	}
	setProtection("workbook", "", false, "", true, false)
	if getProtection("workbook", "") {
		t.Fatal("approved workbook unprotect did not remove protection")
	}
}
