package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestWordCommentCapabilitiesAddListDeleteRoundTrip(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument(DOCX): %v", err)
	}
	operator := Operator{ID: "word-comment-test", Kind: OperatorHuman}

	anchor, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "word-comment-anchor", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"text": "本周完成工资表核对。"}),
	})
	if err != nil {
		t.Fatalf("append comment anchor: %v", err)
	}
	var target struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(anchor.Data, &target); err != nil || target.RunID == "" {
		t.Fatalf("paragraph append result = %s, error=%v", anchor.Data, err)
	}

	added, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-comment-add", DocumentID: doc.DocumentID, CapabilityID: "word.comment.add",
		ExpectedRevision: anchor.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"run_id": target.RunID, "author": "财务", "text": "请复核社保扣款。"}),
	})
	if err != nil {
		t.Fatalf("add Word comment: %v", err)
	}
	var addResult struct {
		Changed   bool   `json:"changed"`
		CommentID string `json:"comment_id"`
		RunID     string `json:"run_id"`
	}
	if err := json.Unmarshal(added.Data, &addResult); err != nil || !addResult.Changed || addResult.CommentID == "" || addResult.RunID != target.RunID {
		t.Fatalf("add result = %s, error=%v", added.Data, err)
	}

	duplicate, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-comment-add-duplicate", DocumentID: doc.DocumentID, CapabilityID: "word.comment.add",
		ExpectedRevision: added.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"run_id": target.RunID, "author": "财务", "text": "请复核社保扣款。"}),
	})
	if err != nil {
		t.Fatalf("repeat Word comment: %v", err)
	}
	var duplicateResult struct {
		Changed   bool   `json:"changed"`
		CommentID string `json:"comment_id"`
	}
	if err := json.Unmarshal(duplicate.Data, &duplicateResult); err != nil || duplicateResult.Changed || duplicateResult.CommentID != addResult.CommentID || duplicate.Revision != added.Revision {
		t.Fatalf("duplicate add = %+v, revision=%d, error=%v", duplicateResult, duplicate.Revision, err)
	}
	state, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || state.Revision != added.Revision || !state.Dirty {
		t.Fatalf("duplicate add changed document state: state=%+v error=%v", state, err)
	}

	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-comment-invalid-run", DocumentID: doc.DocumentID, CapabilityID: "word.comment.add",
		ExpectedRevision: state.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"run_id": "missing-run", "text": "不应写入"}),
	})
	if err == nil {
		t.Fatal("Word comment accepted a run_id absent from the document")
	}
	unchanged, getErr := runtime.GetDocument(ctx, doc.DocumentID)
	if getErr != nil || unchanged.Revision != state.Revision || unchanged.Dirty != state.Dirty {
		t.Fatalf("invalid target changed document state: state=%+v error=%v", unchanged, getErr)
	}

	listed, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-comment-list", DocumentID: doc.DocumentID, CapabilityID: "word.comments.list",
		ExpectedRevision: state.Revision, Operator: operator, Arguments: mustJSON(t, map[string]int{"limit": 10}),
	})
	if err != nil {
		t.Fatalf("list Word comments: %v", err)
	}
	var list struct {
		Total    int `json:"total"`
		Comments []struct {
			ID     string `json:"id"`
			Author string `json:"author"`
			Text   string `json:"text"`
			RunID  string `json:"run_id"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(listed.Data, &list); err != nil || list.Total != 1 || len(list.Comments) != 1 {
		t.Fatalf("list result = %s, error=%v", listed.Data, err)
	}
	if got := list.Comments[0]; got.ID != addResult.CommentID || got.Author != "财务" || got.Text != "请复核社保扣款。" || got.RunID != target.RunID {
		t.Fatalf("listed comment = %+v", got)
	}

	path := filepath.Join(t.TempDir(), "word-comments.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil || saved.Dirty {
		t.Fatalf("save commented DOCX: saved=%+v error=%v", saved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close before comment round trip: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen commented DOCX: %v", err)
	}
	listAfterReopen, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-comment-list-after-reopen", DocumentID: reopened.DocumentID, CapabilityID: "word.comments.list",
		ExpectedRevision: reopened.Revision, Operator: operator, Arguments: mustJSON(t, map[string]int{"limit": 10}),
	})
	if err != nil || !json.Valid(listAfterReopen.Data) || !containsJSONText(listAfterReopen.Data, addResult.CommentID) {
		t.Fatalf("comment missing after DOCX reopen: result=%s error=%v", listAfterReopen.Data, err)
	}
	stateBeforeInvalidDelete := mustGetDocumentInfo(t, runtime, reopened.DocumentID)
	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-comment-delete-invalid", DocumentID: reopened.DocumentID, CapabilityID: "word.comment.delete",
		ExpectedRevision: stateBeforeInvalidDelete.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"comment_id": "missing-comment"}),
	})
	if err == nil {
		t.Fatal("Word comment delete accepted an ID absent from word.comments.list")
	}
	if afterInvalidDelete := mustGetDocumentInfo(t, runtime, reopened.DocumentID); afterInvalidDelete.Revision != stateBeforeInvalidDelete.Revision || afterInvalidDelete.Dirty != stateBeforeInvalidDelete.Dirty {
		t.Fatalf("invalid Word comment delete changed document state: before=%+v after=%+v", stateBeforeInvalidDelete, afterInvalidDelete)
	}

	deleted, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-comment-delete", DocumentID: reopened.DocumentID, CapabilityID: "word.comment.delete",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"comment_id": addResult.CommentID}),
	})
	if err != nil {
		t.Fatalf("delete Word comment: %v", err)
	}
	var deleteResult struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(deleted.Data, &deleteResult); err != nil || !deleteResult.Changed || deleted.Revision != reopened.Revision+1 {
		t.Fatalf("delete result=%s revision=%d error=%v", deleted.Data, deleted.Revision, err)
	}
	stateAfterDelete := mustGetDocumentInfo(t, runtime, reopened.DocumentID)
	if stateAfterDelete.Revision != deleted.Revision || !stateAfterDelete.Dirty {
		t.Fatalf("Word comment delete state=%+v; want dirty revision %d", stateAfterDelete, deleted.Revision)
	}
	deletedList, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-comment-list-after-delete", DocumentID: reopened.DocumentID, CapabilityID: "word.comments.list",
		ExpectedRevision: deleted.Revision, Operator: operator, Arguments: mustJSON(t, map[string]int{"limit": 10}),
	})
	if err != nil || !containsJSONText(deletedList.Data, `"total":0`) {
		t.Fatalf("comments remain after delete: result=%s error=%v", deletedList.Data, err)
	}
	saved, err = runtime.SaveDocument(ctx, reopened.DocumentID)
	if err != nil {
		t.Fatalf("save deleted comment: %v", err)
	}
	if saved.Dirty {
		t.Fatalf("saved deleted-comment DOCX remains dirty: %+v", saved)
	}
	if err := runtime.CloseDocument(ctx, reopened.DocumentID); err != nil {
		t.Fatalf("close after delete: %v", err)
	}
	reopened, err = runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen deleted-comment DOCX: %v", err)
	}
	finalList, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-comment-final-list", DocumentID: reopened.DocumentID, CapabilityID: "word.comments.list",
		ExpectedRevision: reopened.Revision, Operator: operator, Arguments: mustJSON(t, map[string]int{"limit": 10}),
	})
	if err != nil || !containsJSONText(finalList.Data, `"total":0`) {
		t.Fatalf("deleted comment returned after save/reopen: result=%s error=%v", finalList.Data, err)
	}
}

func TestWordCommentCapabilityAnchorsExplicitTextSelection(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument(DOCX): %v", err)
	}
	operator := Operator{ID: "word-comment-selection-test", Kind: OperatorHuman}
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "word-comment-selection-anchor", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"text": "甲豆豆乙"}),
	})
	if err != nil {
		t.Fatalf("append text for selected comment: %v", err)
	}
	var target struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(appended.Data, &target); err != nil || target.RunID == "" {
		t.Fatalf("paragraph append result = %s, error=%v", appended.Data, err)
	}
	added, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-comment-selection-add", DocumentID: doc.DocumentID, CapabilityID: "word.comment.add",
		ExpectedRevision: appended.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{
			"run_id": target.RunID, "start_run_id": target.RunID, "start_offset": 1,
			"end_run_id": target.RunID, "end_offset": 3, "text": "请复核豆豆。",
		}),
	})
	if err != nil {
		t.Fatalf("add selected-range Word comment: %v", err)
	}
	var result struct {
		Changed   bool   `json:"changed"`
		CommentID string `json:"comment_id"`
		RunID     string `json:"run_id"`
	}
	if err := json.Unmarshal(added.Data, &result); err != nil || !result.Changed || result.CommentID == "" || result.RunID != target.RunID {
		t.Fatalf("selection comment result = %s, error=%v", added.Data, err)
	}
	listed, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-comment-selection-list", DocumentID: doc.DocumentID, CapabilityID: "word.comments.list",
		ExpectedRevision: added.Revision, Operator: operator, Arguments: mustJSON(t, map[string]int{"limit": 10}),
	})
	if err != nil {
		t.Fatalf("list selected-range comment: %v", err)
	}
	if !containsJSONText(listed.Data, result.CommentID) || !containsJSONText(listed.Data, "请复核豆豆。") {
		t.Fatalf("selected-range comment missing from listing: %s", listed.Data)
	}
}

func TestWordThemeEffectsAndAddinsCapabilitiesRoundTrip(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, filepath.Join("..", "..", "testdata", "office-corpus", "docx", "sample.docx"))
	if err != nil {
		t.Fatalf("OpenDocument(themed DOCX fixture): %v", err)
	}
	operator := Operator{ID: "word-effects-test", Kind: OperatorHuman}

	listing, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-addins-list", DocumentID: doc.DocumentID, CapabilityID: "word.addins.list",
		ExpectedRevision: doc.Revision, Operator: operator, Arguments: mustJSON(t, map[string]string{"category": "command"}),
	})
	if err != nil {
		t.Fatalf("list Word capabilities: %v", err)
	}
	if !containsJSONText(listing.Data, "word.theme.effects.set") || !containsJSONText(listing.Data, "word.comment.add") || !containsJSONText(listing.Data, "effect_profile") {
		t.Fatalf("Word capability listing omitted usable operation documentation or schema: %s", listing.Data)
	}
	listingState, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || listingState.Revision != doc.Revision || listingState.Dirty != doc.Dirty {
		t.Fatalf("capability listing changed document state: state=%+v error=%v", listingState, err)
	}

	changed, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-effects-set", DocumentID: doc.DocumentID, CapabilityID: "word.theme.effects.set",
		ExpectedRevision: doc.Revision, Operator: operator, Arguments: mustJSON(t, map[string]string{"effect_profile": "subtle"}),
	})
	if err != nil {
		t.Fatalf("set Word theme effects: %v", err)
	}
	var effectResult struct {
		Changed  bool   `json:"changed"`
		Previous string `json:"previous_profile"`
		Profile  string `json:"effect_profile"`
	}
	if err := json.Unmarshal(changed.Data, &effectResult); err != nil || !effectResult.Changed || effectResult.Profile != "subtle" || changed.Revision != doc.Revision+1 {
		t.Fatalf("set effects result=%s revision=%d error=%v", changed.Data, changed.Revision, err)
	}
	noOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-effects-noop", DocumentID: doc.DocumentID, CapabilityID: "word.theme.effects.set",
		ExpectedRevision: changed.Revision, Operator: operator, Arguments: mustJSON(t, map[string]string{"effect_profile": "subtle"}),
	})
	if err != nil || noOp.Revision != changed.Revision || !containsJSONText(noOp.Data, `"changed":false`) {
		t.Fatalf("same Word theme effect was not a no-op: result=%s revision=%d error=%v", noOp.Data, noOp.Revision, err)
	}
	noOpState, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || noOpState.Revision != changed.Revision || !noOpState.Dirty {
		t.Fatalf("Word theme no-op changed revision/dirty state: state=%+v error=%v", noOpState, err)
	}
	beforeInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid profile: %v", err)
	}
	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-effects-invalid", DocumentID: doc.DocumentID, CapabilityID: "word.theme.effects.set",
		ExpectedRevision: beforeInvalid.Revision, Operator: operator, Arguments: mustJSON(t, map[string]string{"effect_profile": "dramatic"}),
	})
	if !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("unsupported Word effect profile error=%v, want invalid arguments", err)
	}
	afterInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || afterInvalid.Revision != beforeInvalid.Revision || afterInvalid.Dirty != beforeInvalid.Dirty {
		t.Fatalf("invalid effect profile changed document state: state=%+v error=%v", afterInvalid, err)
	}

	path := filepath.Join(t.TempDir(), "word-effects.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil || saved.Dirty {
		t.Fatalf("save themed DOCX: saved=%+v error=%v", saved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close before theme round trip: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen themed DOCX: %v", err)
	}
	noOp, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-effects-after-reopen", DocumentID: reopened.DocumentID, CapabilityID: "word.theme.effects.set",
		ExpectedRevision: reopened.Revision, Operator: operator, Arguments: mustJSON(t, map[string]string{"effect_profile": "subtle"}),
	})
	if err != nil || noOp.Revision != reopened.Revision || !containsJSONText(noOp.Data, `"changed":false`) {
		t.Fatalf("saved Word effect was not retained after reopen: result=%s revision=%d error=%v", noOp.Data, noOp.Revision, err)
	}
	reopenedState, err := runtime.GetDocument(ctx, reopened.DocumentID)
	if err != nil || reopenedState.Revision != reopened.Revision || reopenedState.Dirty {
		t.Fatalf("Word theme no-op after reopen changed clean state: state=%+v error=%v", reopenedState, err)
	}
}

func containsJSONText(data json.RawMessage, text string) bool {
	return json.Valid(data) && bytes.Contains(data, []byte(text))
}
