package application

import (
	"context"
	"encoding/json"
	"testing"
)

func TestWordEditorReviewReturnsBoundedStructuredFindings(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	command, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "append-review-fixture", DocumentID: doc.DocumentID,
		CommandID: "word.paragraph.append", ExpectedRevision: doc.Revision,
		Operator:  Operator{ID: "review-test", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "这句话，，括号未闭合（"}),
	})
	if err != nil {
		t.Fatalf("append Word fixture: %v", err)
	}
	before, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before review: %v", err)
	}
	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "review-word-editor", DocumentID: doc.DocumentID,
		CapabilityID: "word.review.editor.analyze", ExpectedRevision: command.Revision,
		Operator:  Operator{ID: "review-test", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]int{"start_block": 0, "max_blocks": 100}),
	})
	if err != nil {
		t.Fatalf("InvokeCapability(word.review.editor.analyze): %v", err)
	}
	var output struct {
		Score             int  `json:"heuristic_score"`
		Paragraphs        int  `json:"paragraphs"`
		FindingsTruncated bool `json:"findings_truncated"`
		HasMore           bool `json:"has_more"`
		Findings          []struct {
			Code        string `json:"code"`
			BlockIndex  int    `json:"block_index"`
			BlockID     string `json:"block_id"`
			ParagraphID string `json:"paragraph_id"`
			RunID       string `json:"run_id"`
			Detail      string `json:"detail"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode review result: %v; data=%s", err, result.Data)
	}
	if output.Score != 90 || output.Paragraphs != 2 || output.FindingsTruncated || output.HasMore || len(output.Findings) != 2 {
		t.Fatalf("review result = %+v", output)
	}
	if output.Findings[0].Code != "repeated_punctuation" || output.Findings[1].Code != "unbalanced_delimiters" {
		t.Fatalf("review findings = %+v", output.Findings)
	}
	for _, finding := range output.Findings {
		if finding.BlockID == "" || finding.ParagraphID == "" || finding.RunID == "" || finding.Detail == "" {
			t.Fatalf("finding lacks stable target/details: %+v", finding)
		}
	}
	after, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument after review: %v", err)
	}
	if after.Revision != before.Revision || after.Dirty != before.Dirty {
		t.Fatalf("read-only review changed document: before=%+v after=%+v", before, after)
	}
}
