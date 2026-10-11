package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"glance/internal/format/docx"
)

func TestWordCommentUsesUnicodeCharacterLimitsAndRoundTrips(t *testing.T) {
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
	operator := Operator{ID: "long-word-comment-agent", Kind: OperatorExternalAgent}
	anchor, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "long-word-comment-anchor", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"text": "员工工资核对"}),
	})
	if err != nil {
		t.Fatalf("append comment anchor: %v", err)
	}
	var target struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(anchor.Data, &target); err != nil || target.RunID == "" {
		t.Fatalf("decode comment target: %v; data=%s", err, anchor.Data)
	}
	text := strings.Repeat("薪", 32768)
	author := strings.Repeat("财", 256)
	added, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "long-word-comment-add", DocumentID: doc.DocumentID, CapabilityID: "word.comment.add",
		ExpectedRevision: anchor.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"run_id": target.RunID, "author": author, "text": text}),
	})
	if err != nil {
		t.Fatalf("add schema-sized Unicode Word comment: %v", err)
	}
	var addResult struct {
		Changed   bool   `json:"changed"`
		CommentID string `json:"comment_id"`
	}
	if err := json.Unmarshal(added.Data, &addResult); err != nil || !addResult.Changed || addResult.CommentID == "" {
		t.Fatalf("add result=%s error=%v", added.Data, err)
	}
	state, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || state.Revision != anchor.Revision+1 || !state.Dirty {
		t.Fatalf("document state after comment=%+v error=%v", state, err)
	}

	listed, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "long-word-comment-list", DocumentID: doc.DocumentID, CapabilityID: "word.comments.list",
		ExpectedRevision: state.Revision, Operator: operator, Arguments: json.RawMessage(`{"limit":1}`),
	})
	if err != nil {
		t.Fatalf("list long Word comment: %v", err)
	}
	var list struct {
		Comments []struct {
			ID            string `json:"id"`
			Author        string `json:"author"`
			Text          string `json:"text"`
			TextTruncated bool   `json:"text_truncated"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(listed.Data, &list); err != nil || len(list.Comments) != 1 {
		t.Fatalf("decode long comment list: %v; data=%s", err, listed.Data)
	}
	listedComment := list.Comments[0]
	if listedComment.ID != addResult.CommentID || listedComment.Author != author || !listedComment.TextTruncated || len(listedComment.Text) > maxWordCommentPreviewBytes || !utf8.ValidString(listedComment.Text) {
		t.Fatalf("long comment preview=%+v; want bounded valid UTF-8 preview", listedComment)
	}

	path := filepath.Join(t.TempDir(), "unicode-comment.docx")
	if saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil || saved.Dirty {
		t.Fatalf("save Unicode comment: saved=%+v error=%v", saved, err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen Unicode comment: %v", err)
	}
	defer reopened.Close()
	comments, err := reopened.WordComments()
	if err != nil || len(comments) != 1 {
		t.Fatalf("read reopened comments: count=%d error=%v", len(comments), err)
	}
	if comments[0].ID != addResult.CommentID || comments[0].Author != author || comments[0].Text != text || comments[0].RunID != target.RunID {
		t.Fatalf("reopened comment does not preserve full Unicode content: id=%q author-runes=%d text-runes=%d run=%q", comments[0].ID, utf8.RuneCountInString(comments[0].Author), utf8.RuneCountInString(comments[0].Text), comments[0].RunID)
	}
}
