package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpreadsheetDefinedNameCapabilitiesRoundTripAndNoop(t *testing.T) {
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
	operator := Operator{ID: "xlsx-defined-name-test", Kind: OperatorHuman}
	comment := strings.Repeat("工", 100)
	nameArgs := map[string]string{"name": "PayrollTotal", "ref": "Sheet1!$A$1:$A$10", "comment": comment}

	list := func(documentID string, revision uint64, requestID string) ([]spreadsheetDefinedNameTestItem, error) {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: documentID, CapabilityID: "spreadsheet.defined_names.get",
			ExpectedRevision: revision, Operator: operator, Arguments: mustJSON(t, map[string]any{}),
		})
		if invokeErr != nil {
			return nil, invokeErr
		}
		var data struct {
			Names []spreadsheetDefinedNameTestItem `json:"names"`
		}
		if err := json.Unmarshal(result.Data, &data); err != nil {
			return nil, err
		}
		return data.Names, nil
	}

	initial, err := list(doc.DocumentID, doc.Revision, "xlsx-defined-names-initial")
	if err != nil || len(initial) != 0 {
		t.Fatalf("initial defined names=%+v error=%v", initial, err)
	}
	set, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-defined-name-set", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.defined_names.set",
		ExpectedRevision: doc.Revision, Operator: operator, Arguments: mustJSON(t, nameArgs),
	})
	if err != nil || set.Revision != doc.Revision+1 || !containsJSONText(set.Data, `"changed":true`) {
		t.Fatalf("set defined name: result=%s revision=%d error=%v", set.Data, set.Revision, err)
	}
	namesBeforeRepeat, err := list(doc.DocumentID, set.Revision, "xlsx-defined-names-before-repeat")
	if err != nil {
		t.Fatalf("list defined names before repeat: %v", err)
	}
	setNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-defined-name-set-noop", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.defined_names.set",
		ExpectedRevision: set.Revision, Operator: operator, Arguments: mustJSON(t, nameArgs),
	})
	if err != nil || setNoOp.Revision != set.Revision || !containsJSONText(setNoOp.Data, `"changed":false`) {
		t.Fatalf("identical defined name should be a no-op: previous=%+v result=%s revision=%d error=%v", namesBeforeRepeat, setNoOp.Data, setNoOp.Revision, err)
	}
	names, err := list(doc.DocumentID, set.Revision, "xlsx-defined-names-after-set")
	if err != nil || len(names) != 1 || names[0].Name != "PayrollTotal" || names[0].Ref != "Sheet1!$A$1:$A$10" || names[0].Comment != comment {
		t.Fatalf("defined names after set=%+v error=%v", names, err)
	}
	stateBeforeInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid defined name: %v", err)
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-defined-name-invalid", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.defined_names.set",
		ExpectedRevision: set.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"name": "Bad Name", "ref": "Sheet1!$A$1"}),
	}); err == nil {
		t.Fatal("defined name accepted an invalid name")
	}
	stateAfterInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterInvalid.Revision != stateBeforeInvalid.Revision || stateAfterInvalid.Dirty != stateBeforeInvalid.Dirty {
		t.Fatalf("invalid defined name changed workbook state: before=%+v after=%+v error=%v", stateBeforeInvalid, stateAfterInvalid, err)
	}
	oversizedComment := map[string]string{"name": "PayrollTotal", "ref": "Sheet1!$A$1:$A$10", "comment": strings.Repeat("超", 256)}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-defined-name-comment-too-long", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.defined_names.set",
		ExpectedRevision: set.Revision, Operator: operator, Arguments: mustJSON(t, oversizedComment),
	}); err == nil {
		t.Fatal("defined name accepted a comment above its Unicode character limit")
	}
	if afterLongComment := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterLongComment.Revision != stateBeforeInvalid.Revision || afterLongComment.Dirty != stateBeforeInvalid.Dirty {
		t.Fatalf("oversized defined-name comment changed workbook state: before=%+v after=%+v", stateBeforeInvalid, afterLongComment)
	}

	firstPath := filepath.Join(t.TempDir(), "spreadsheet-defined-name-added.xlsx")
	firstSaved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, firstPath)
	if err != nil || firstSaved.SourcePath != canonicalPath(firstPath) || firstSaved.Dirty {
		t.Fatalf("save workbook with defined name: saved=%+v error=%v", firstSaved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close workbook before defined-name reopen: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, firstPath)
	if err != nil {
		t.Fatalf("reopen workbook with defined name: %v", err)
	}
	persisted, err := list(reopened.DocumentID, reopened.Revision, "xlsx-defined-names-after-reopen")
	if err != nil || len(persisted) != 1 || persisted[0].Name != "PayrollTotal" || persisted[0].Ref != "Sheet1!$A$1:$A$10" || persisted[0].Comment != comment {
		t.Fatalf("defined name did not persist after reopen: names=%+v error=%v", persisted, err)
	}

	deleteNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-defined-name-delete-noop", DocumentID: reopened.DocumentID, CapabilityID: "spreadsheet.defined_names.delete",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"name": "MissingName"}),
	})
	if err != nil || deleteNoOp.Revision != reopened.Revision || !containsJSONText(deleteNoOp.Data, `"changed":false`) {
		t.Fatalf("deleting a missing defined name should be a no-op: result=%s revision=%d error=%v", deleteNoOp.Data, deleteNoOp.Revision, err)
	}
	deleted, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-defined-name-delete", DocumentID: reopened.DocumentID, CapabilityID: "spreadsheet.defined_names.delete",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"name": "PayrollTotal"}),
	})
	if err != nil || deleted.Revision != reopened.Revision+1 || !containsJSONText(deleted.Data, `"changed":true`) {
		t.Fatalf("delete defined name: result=%s revision=%d error=%v", deleted.Data, deleted.Revision, err)
	}
	deletedNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-defined-name-delete-repeat", DocumentID: reopened.DocumentID, CapabilityID: "spreadsheet.defined_names.delete",
		ExpectedRevision: deleted.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"name": "PayrollTotal"}),
	})
	if err != nil || deletedNoOp.Revision != deleted.Revision || !containsJSONText(deletedNoOp.Data, `"changed":false`) {
		t.Fatalf("repeat defined-name delete should be a no-op: result=%s revision=%d error=%v", deletedNoOp.Data, deletedNoOp.Revision, err)
	}
	empty, err := list(reopened.DocumentID, deleted.Revision, "xlsx-defined-names-after-delete")
	if err != nil || len(empty) != 0 {
		t.Fatalf("defined names after delete=%+v error=%v", empty, err)
	}

	secondPath := filepath.Join(t.TempDir(), "spreadsheet-defined-name-deleted.xlsx")
	secondSaved, err := runtime.SaveDocumentAs(ctx, reopened.DocumentID, secondPath)
	if err != nil || secondSaved.SourcePath != canonicalPath(secondPath) || secondSaved.Dirty {
		t.Fatalf("save workbook after deleting defined name: saved=%+v error=%v", secondSaved, err)
	}
	if err := runtime.CloseDocument(ctx, reopened.DocumentID); err != nil {
		t.Fatalf("close workbook before delete reopen: %v", err)
	}
	deletedReopen, err := runtime.OpenDocument(ctx, secondPath)
	if err != nil {
		t.Fatalf("reopen workbook after defined-name deletion: %v", err)
	}
	deletedPersisted, err := list(deletedReopen.DocumentID, deletedReopen.Revision, "xlsx-defined-names-after-delete-reopen")
	if err != nil || len(deletedPersisted) != 0 {
		t.Fatalf("deleted defined name reappeared after reopen: names=%+v error=%v", deletedPersisted, err)
	}
}

type spreadsheetDefinedNameTestItem struct {
	Name    string `json:"name"`
	Ref     string `json:"ref"`
	Scope   string `json:"scope"`
	Comment string `json:"comment"`
}
