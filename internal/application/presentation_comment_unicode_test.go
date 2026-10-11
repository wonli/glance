package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"glance/internal/format/pptx"
)

func TestPresentationCommentUsesUnicodeCharacterLimitsAndRoundTrips(t *testing.T) {
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
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID})
	if err != nil {
		t.Fatalf("inspect presentation: %v", err)
	}
	var presentation struct {
		Slides []struct {
			ID uint32 `json:"slide_id"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(snapshot.Content, &presentation); err != nil || len(presentation.Slides) == 0 || presentation.Slides[0].ID == 0 {
		t.Fatalf("decode slide ID: %v; content=%s", err, snapshot.Content)
	}
	slideID := presentation.Slides[0].ID
	text := strings.Repeat("薪", 8192)
	author := strings.Repeat("财", 256)
	operator := Operator{ID: "long-pptx-comment-agent", Kind: OperatorExternalAgent}
	added, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "long-pptx-comment-add", DocumentID: doc.DocumentID, CapabilityID: "presentation.comment.add",
		ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "x_pt": 72, "y_pt": 72, "author": author, "text": text}),
	})
	if err != nil {
		t.Fatalf("add schema-sized Unicode presentation comment: %v", err)
	}
	var addResult struct {
		Changed   bool   `json:"changed"`
		CommentID string `json:"comment_id"`
	}
	if err := json.Unmarshal(added.Data, &addResult); err != nil || !addResult.Changed || addResult.CommentID == "" {
		t.Fatalf("add result=%s error=%v", added.Data, err)
	}
	state, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || state.Revision != doc.Revision+1 || !state.Dirty {
		t.Fatalf("document state after comment=%+v error=%v", state, err)
	}
	listed, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "long-pptx-comment-list", DocumentID: doc.DocumentID, CapabilityID: "presentation.comments.list",
		ExpectedRevision: state.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "limit": 1}),
	})
	if err != nil {
		t.Fatalf("list long presentation comment: %v", err)
	}
	var list struct {
		Comments []struct {
			CommentID     string `json:"comment_id"`
			Author        string `json:"author"`
			Text          string `json:"text"`
			TextTruncated bool   `json:"text_truncated"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(listed.Data, &list); err != nil || len(list.Comments) != 1 {
		t.Fatalf("decode long presentation comment list: %v; data=%s", err, listed.Data)
	}
	preview := list.Comments[0]
	if preview.CommentID != addResult.CommentID || preview.Author != author || !preview.TextTruncated || len(preview.Text) > maxPresentationCommentPreview || !utf8.ValidString(preview.Text) {
		t.Fatalf("presentation comment preview=%+v; want bounded valid UTF-8 preview", preview)
	}

	path := filepath.Join(t.TempDir(), "unicode-presentation-comment.pptx")
	if saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil || saved.Dirty {
		t.Fatalf("save Unicode presentation comment: saved=%+v error=%v", saved, err)
	}
	reopened, err := pptx.Open(path)
	if err != nil {
		t.Fatalf("reopen Unicode presentation comment: %v", err)
	}
	defer reopened.Close()
	comments, _, err := reopened.ListPresentationComments(slideID, 0, 1)
	if err != nil || len(comments) != 1 {
		t.Fatalf("read reopened comments: count=%d error=%v", len(comments), err)
	}
	if comments[0].ID != addResult.CommentID || comments[0].Author != author || comments[0].Text != text || comments[0].SlideID != slideID {
		t.Fatalf("reopened presentation comment does not preserve full Unicode content: id=%q author-runes=%d text-runes=%d slide=%d", comments[0].ID, utf8.RuneCountInString(comments[0].Author), utf8.RuneCountInString(comments[0].Text), comments[0].SlideID)
	}
}
