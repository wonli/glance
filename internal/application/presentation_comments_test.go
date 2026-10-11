package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestPresentationCommentCapabilitySavesAndReopensWithoutLosingSlideContent(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	doc, err := runtime.CreateDocument(ctx, FormatPPTX)
	if err != nil {
		t.Fatalf("CreateDocument(PPTX): %v", err)
	}
	initial, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID})
	if err != nil {
		t.Fatalf("inspect new presentation: %v", err)
	}
	var before struct {
		SlideCount int `json:"slide_count"`
		Slides     []struct {
			ID uint32 `json:"slide_id"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(initial.Content, &before); err != nil {
		t.Fatalf("decode new presentation: %v", err)
	}
	if before.SlideCount != 1 || len(before.Slides) != 1 || before.Slides[0].ID == 0 {
		t.Fatalf("new presentation slides = %+v (count %d)", before.Slides, before.SlideCount)
	}
	slideID := before.Slides[0].ID
	operator := Operator{ID: "pptx-comment-compatibility", Kind: OperatorBuiltInAgent}
	revision := doc.Revision

	textBox, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-comment-seed-content", DocumentID: doc.DocumentID,
		CapabilityID: "presentation.element.textbox.create", ExpectedRevision: revision,
		Operator: operator,
		Arguments: mustJSON(t, map[string]any{
			"slide_id": slideID, "x": 72, "y": 72, "width": 240, "height": 48, "text": "季度成本摘要",
		}),
	})
	if err != nil {
		t.Fatalf("seed slide content: %v", err)
	}
	revision = textBox.Revision
	beforeInvalidAdd := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-comment-add-invalid-slide", DocumentID: doc.DocumentID,
		CapabilityID: "presentation.comment.add", ExpectedRevision: revision,
		Operator: operator,
		Arguments: mustJSON(t, map[string]any{
			"slide_id": slideID + 100000, "x_pt": 96, "y_pt": 120,
			"author": "财务同事", "text": "不应写入。",
		}),
	})
	if err == nil {
		t.Fatal("presentation comment accepted a nonexistent slide ID")
	}
	if afterInvalidAdd := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterInvalidAdd.Revision != beforeInvalidAdd.Revision || afterInvalidAdd.Dirty != beforeInvalidAdd.Dirty {
		t.Fatalf("invalid PPTX comment target changed document state: before=%+v after=%+v", beforeInvalidAdd, afterInvalidAdd)
	}

	added, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-comment-add", DocumentID: doc.DocumentID,
		CapabilityID: "presentation.comment.add", ExpectedRevision: revision,
		Operator: operator,
		Arguments: mustJSON(t, map[string]any{
			"slide_id": slideID, "x_pt": 96, "y_pt": 120,
			"author": "财务同事", "text": "请复核本季度的成本分类。",
		}),
	})
	if err != nil {
		t.Fatalf("add presentation comment: %v", err)
	}
	var addedData struct {
		Changed   bool   `json:"changed"`
		CommentID string `json:"comment_id"`
		SlideID   uint32 `json:"slide_id"`
	}
	if err := json.Unmarshal(added.Data, &addedData); err != nil || !addedData.Changed || addedData.CommentID == "" || addedData.SlideID != slideID {
		t.Fatalf("add comment result = %s, error=%v", added.Data, err)
	}

	path := filepath.Join(t.TempDir(), "presentation-comments.pptx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if saved.Dirty {
		t.Fatalf("saved presentation remained dirty: %+v", saved)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close saved presentation before reopening: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen saved presentation: %v", err)
	}
	if reopened.Dirty || reopened.SourcePath != canonicalPath(path) {
		t.Fatalf("reopened presentation metadata = %+v", reopened)
	}

	content, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: reopened.DocumentID})
	if err != nil {
		t.Fatalf("inspect reopened presentation: %v", err)
	}
	var after struct {
		SlideCount int `json:"slide_count"`
		Slides     []struct {
			ID       uint32 `json:"slide_id"`
			Elements []struct {
				Text string `json:"text"`
			} `json:"elements"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(content.Content, &after); err != nil {
		t.Fatalf("decode reopened presentation: %v", err)
	}
	if after.SlideCount != before.SlideCount || len(after.Slides) != 1 || after.Slides[0].ID != slideID {
		t.Fatalf("reopened slide structure = %+v (count %d), want original slide %d", after.Slides, after.SlideCount, slideID)
	}
	var preservedText bool
	for _, element := range after.Slides[0].Elements {
		preservedText = preservedText || strings.Contains(element.Text, "季度成本摘要")
	}
	if !preservedText {
		t.Fatalf("reopened slide lost preexisting content: %+v", after.Slides[0].Elements)
	}

	listed, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-comment-list-after-reopen", DocumentID: reopened.DocumentID,
		CapabilityID: "presentation.comments.list", ExpectedRevision: reopened.Revision,
		Operator: operator, Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "limit": 20}),
	})
	if err != nil {
		t.Fatalf("list comments after reopen: %v", err)
	}
	var list struct {
		Total    int `json:"total"`
		Comments []struct {
			CommentID string  `json:"comment_id"`
			SlideID   uint32  `json:"slide_id"`
			Author    string  `json:"author"`
			Text      string  `json:"text"`
			XPT       float64 `json:"x_pt"`
			YPT       float64 `json:"y_pt"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(listed.Data, &list); err != nil {
		t.Fatalf("decode reopened comments: %v; data=%s", err, listed.Data)
	}
	if list.Total != 1 || len(list.Comments) != 1 {
		t.Fatalf("reopened comment list = %+v", list)
	}
	comment := list.Comments[0]
	if comment.CommentID != addedData.CommentID || comment.SlideID != slideID || comment.Author != "财务同事" || comment.Text != "请复核本季度的成本分类。" || comment.XPT != 96 || comment.YPT != 120 {
		t.Fatalf("reopened comment = %+v", comment)
	}

	deleteOperator := Operator{ID: "pptx-comment-delete-human", Kind: OperatorHuman}
	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-comment-delete-invalid", DocumentID: reopened.DocumentID,
		CapabilityID: "presentation.comment.delete", ExpectedRevision: reopened.Revision,
		Operator:  deleteOperator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "comment_id": "missing-comment"}),
	})
	if err == nil {
		t.Fatal("comment deletion accepted an ID absent from presentation.comments.list")
	}
	unchanged, err := runtime.GetDocument(ctx, reopened.DocumentID)
	if err != nil || unchanged.Revision != reopened.Revision || unchanged.Dirty {
		t.Fatalf("invalid comment target changed document state: state=%+v error=%v", unchanged, err)
	}

	deleted, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-comment-delete", DocumentID: reopened.DocumentID,
		CapabilityID: "presentation.comment.delete", ExpectedRevision: reopened.Revision,
		Operator:  deleteOperator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "comment_id": addedData.CommentID}),
	})
	if err != nil || !containsJSONText(deleted.Data, `"changed":true`) || deleted.Revision != reopened.Revision+1 {
		t.Fatalf("delete presentation comment result=%s revision=%d error=%v", deleted.Data, deleted.Revision, err)
	}
	stateAfterDelete := mustGetDocumentInfo(t, runtime, reopened.DocumentID)
	if stateAfterDelete.Revision != deleted.Revision || !stateAfterDelete.Dirty {
		t.Fatalf("presentation comment delete state=%+v; want dirty revision %d", stateAfterDelete, deleted.Revision)
	}
	deletedList, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-comment-list-after-delete", DocumentID: reopened.DocumentID,
		CapabilityID: "presentation.comments.list", ExpectedRevision: deleted.Revision,
		Operator: deleteOperator, Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "limit": 20}),
	})
	if err != nil || !containsJSONText(deletedList.Data, `"total":0`) {
		t.Fatalf("deleted presentation comment remains in live list: result=%s error=%v", deletedList.Data, err)
	}
	savedAfterDelete, err := runtime.SaveDocument(ctx, reopened.DocumentID)
	if err != nil {
		t.Fatalf("save deleted presentation comment: %v", err)
	}
	if savedAfterDelete.Dirty {
		t.Fatalf("saved deleted-comment PPTX remains dirty: %+v", savedAfterDelete)
	}
	if err := runtime.CloseDocument(ctx, reopened.DocumentID); err != nil {
		t.Fatalf("close after deleting presentation comment: %v", err)
	}
	reopened, err = runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen deleted-comment presentation: %v", err)
	}
	finalList, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-comment-final-list", DocumentID: reopened.DocumentID,
		CapabilityID: "presentation.comments.list", ExpectedRevision: reopened.Revision,
		Operator: deleteOperator, Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "limit": 20}),
	})
	if err != nil || !containsJSONText(finalList.Data, `"total":0`) {
		t.Fatalf("deleted presentation comment returned after save/reopen: result=%s error=%v", finalList.Data, err)
	}
}
