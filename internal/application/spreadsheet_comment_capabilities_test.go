package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpreadsheetCommentCapabilitiesRoundTripAndNoop(t *testing.T) {
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
	operator := Operator{ID: "xlsx-comments-test", Kind: OperatorHuman}
	const sheet = "Sheet1"

	list := func(documentID string, revision uint64, requestID string) ([]spreadsheetCommentTestItem, error) {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: documentID, CapabilityID: "spreadsheet.comments.get",
			ExpectedRevision: revision, Operator: operator,
			Arguments: mustJSON(t, map[string]string{"sheet": sheet}),
		})
		if invokeErr != nil {
			return nil, invokeErr
		}
		var data struct {
			Comments []spreadsheetCommentTestItem `json:"comments"`
		}
		if err := json.Unmarshal(result.Data, &data); err != nil {
			return nil, err
		}
		return data.Comments, nil
	}

	initialComments, err := list(doc.DocumentID, doc.Revision, "xlsx-comments-initial")
	if err != nil || len(initialComments) != 0 {
		t.Fatalf("initial comments=%+v error=%v", initialComments, err)
	}
	added, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-comment-add", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.comment.add",
		ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": sheet, "cell": "a1", "author": "财务 Agent", "text": "核对小计公式"}),
	})
	if err != nil || added.Revision != doc.Revision+1 || !containsJSONText(added.Data, `"changed":true`) || !containsJSONText(added.Data, `"cell":"A1"`) {
		t.Fatalf("add spreadsheet comment: result=%s revision=%d error=%v", added.Data, added.Revision, err)
	}
	comments, err := list(doc.DocumentID, added.Revision, "xlsx-comments-after-add")
	if err != nil || len(comments) != 1 || comments[0].Cell != "A1" || comments[0].Author != "财务 Agent" || comments[0].Text != "核对小计公式" {
		t.Fatalf("comments after add=%+v error=%v", comments, err)
	}

	addNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-comment-add-noop", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.comment.add",
		ExpectedRevision: added.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": sheet, "cell": "A1", "author": "财务 Agent", "text": "核对小计公式"}),
	})
	if err != nil || addNoOp.Revision != added.Revision || !containsJSONText(addNoOp.Data, `"changed":false`) {
		t.Fatalf("repeating identical comment add should be a no-op: result=%s revision=%d error=%v", addNoOp.Data, addNoOp.Revision, err)
	}

	stateBeforeInvalidAdd, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid comment add: %v", err)
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-comment-add-invalid-sheet", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.comment.add",
		ExpectedRevision: added.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": "不存在的工作表", "cell": "A1", "text": "不应写入"}),
	}); err == nil {
		t.Fatal("spreadsheet comment add accepted an unknown worksheet")
	}
	stateAfterInvalidAdd, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterInvalidAdd.Revision != stateBeforeInvalidAdd.Revision || stateAfterInvalidAdd.Dirty != stateBeforeInvalidAdd.Dirty {
		t.Fatalf("invalid comment target changed workbook state: before=%+v after=%+v error=%v", stateBeforeInvalidAdd, stateAfterInvalidAdd, err)
	}

	firstPath := filepath.Join(t.TempDir(), "spreadsheet-comment-added.xlsx")
	firstSaved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, firstPath)
	if err != nil || firstSaved.SourcePath != canonicalPath(firstPath) || firstSaved.Dirty {
		t.Fatalf("save workbook with comment: saved=%+v error=%v", firstSaved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close workbook before comment reopen: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, firstPath)
	if err != nil {
		t.Fatalf("reopen workbook with comment: %v", err)
	}
	persisted, err := list(reopened.DocumentID, reopened.Revision, "xlsx-comments-after-reopen")
	if err != nil || len(persisted) != 1 || persisted[0].Author != "财务 Agent" || persisted[0].Text != "核对小计公式" {
		t.Fatalf("comment did not persist after reopen: comments=%+v error=%v", persisted, err)
	}

	deleted, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-comment-delete", DocumentID: reopened.DocumentID, CapabilityID: "spreadsheet.comment.delete",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": sheet, "cell": "A1"}),
	})
	if err != nil || deleted.Revision != reopened.Revision+1 || !containsJSONText(deleted.Data, `"changed":true`) {
		t.Fatalf("delete spreadsheet comment: result=%s revision=%d error=%v", deleted.Data, deleted.Revision, err)
	}
	deleteNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-comment-delete-noop", DocumentID: reopened.DocumentID, CapabilityID: "spreadsheet.comment.delete",
		ExpectedRevision: deleted.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": sheet, "cell": "A1"}),
	})
	if err != nil || deleteNoOp.Revision != deleted.Revision || !containsJSONText(deleteNoOp.Data, `"changed":false`) {
		t.Fatalf("deleting an absent comment should be a no-op: result=%s revision=%d error=%v", deleteNoOp.Data, deleteNoOp.Revision, err)
	}
	remaining, err := list(reopened.DocumentID, deleted.Revision, "xlsx-comments-after-delete")
	if err != nil || len(remaining) != 0 {
		t.Fatalf("comments after delete=%+v error=%v", remaining, err)
	}

	secondPath := filepath.Join(t.TempDir(), "spreadsheet-comment-deleted.xlsx")
	secondSaved, err := runtime.SaveDocumentAs(ctx, reopened.DocumentID, secondPath)
	if err != nil || secondSaved.SourcePath != canonicalPath(secondPath) || secondSaved.Dirty {
		t.Fatalf("save workbook after comment deletion: saved=%+v error=%v", secondSaved, err)
	}
	if err := runtime.CloseDocument(ctx, reopened.DocumentID); err != nil {
		t.Fatalf("close workbook before delete reopen: %v", err)
	}
	deletedReopen, err := runtime.OpenDocument(ctx, secondPath)
	if err != nil {
		t.Fatalf("reopen workbook after comment deletion: %v", err)
	}
	deletedPersisted, err := list(deletedReopen.DocumentID, deletedReopen.Revision, "xlsx-comments-after-delete-reopen")
	if err != nil || len(deletedPersisted) != 0 {
		t.Fatalf("deleted spreadsheet comment reappeared after reopen: comments=%+v error=%v", deletedPersisted, err)
	}
}

func TestSpreadsheetCommentUsesUnicodeCharacterLimits(t *testing.T) {
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
	operator := Operator{ID: "xlsx-comment-unicode-test", Kind: OperatorHuman}
	author := strings.Repeat("财", 100)
	commentText := strings.Repeat("核", 11000)
	added, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-comment-unicode-add", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.comment.add",
		ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": "Sheet1", "cell": "A1", "author": author, "text": commentText}),
	})
	if err != nil || added.Revision != doc.Revision+1 || !containsJSONText(added.Data, `"changed":true`) {
		t.Fatalf("add Unicode comment: result=%s revision=%d error=%v", added.Data, added.Revision, err)
	}

	path := filepath.Join(t.TempDir(), "spreadsheet-unicode-comment.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("CloseDocument: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	listed, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-comment-unicode-list", DocumentID: reopened.DocumentID, CapabilityID: "spreadsheet.comments.get",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": "Sheet1"}),
	})
	if err != nil {
		t.Fatalf("list Unicode comment: %v", err)
	}
	var result struct {
		Comments []spreadsheetCommentTestItem `json:"comments"`
	}
	if err := json.Unmarshal(listed.Data, &result); err != nil || len(result.Comments) != 1 || result.Comments[0].Author != author || result.Comments[0].Text != commentText {
		t.Fatalf("Unicode comment after reopen=%+v error=%v", result.Comments, err)
	}

	beforeInvalid, err := runtime.GetDocument(ctx, reopened.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before oversized comment: %v", err)
	}
	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-comment-unicode-too-long", DocumentID: reopened.DocumentID, CapabilityID: "spreadsheet.comment.add",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"sheet": "Sheet1", "cell": "A1", "author": strings.Repeat("甲", 256), "text": strings.Repeat("超", 32768)}),
	})
	if err == nil {
		t.Fatal("spreadsheet.comment.add accepted values above its Unicode character limits")
	}
	afterInvalid, getErr := runtime.GetDocument(ctx, reopened.DocumentID)
	if getErr != nil || afterInvalid.Revision != beforeInvalid.Revision || afterInvalid.Dirty != beforeInvalid.Dirty {
		t.Fatalf("oversized comment changed workbook state: before=%+v after=%+v error=%v", beforeInvalid, afterInvalid, getErr)
	}
}

type spreadsheetCommentTestItem struct {
	Cell   string `json:"cell"`
	Author string `json:"author"`
	Text   string `json:"text"`
}
