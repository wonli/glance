package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordAllParagraphSpacingCapabilityRoundTripsAndNoops(t *testing.T) {
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
	operator := Operator{ID: "spacing-agent", Kind: OperatorBuiltInAgent}
	revision := doc.Revision
	for index, text := range []string{"第一段", "第二段"} {
		appended, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: "spacing-paragraph-" + string(rune('1'+index)), DocumentID: doc.DocumentID,
			CommandID: "word.paragraph.append", ExpectedRevision: revision, Operator: operator,
			Arguments: mustJSON(t, map[string]any{"text": text}),
		})
		if err != nil {
			t.Fatalf("append paragraph %d: %v", index+1, err)
		}
		revision = appended.Revision
	}
	compact, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "set-all-paragraph-spacing", DocumentID: doc.DocumentID,
		CommandID: "word.paragraph.spacing_all.set", ExpectedRevision: revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"before_points": 0, "after_points": 4}),
	})
	if err != nil {
		t.Fatalf("set all paragraph spacing: %v", err)
	}
	var compactData struct {
		Changed        bool    `json:"changed"`
		BeforePoints   float64 `json:"before_points"`
		AfterPoints    float64 `json:"after_points"`
		ParagraphCount int     `json:"paragraph_count"`
	}
	if err := json.Unmarshal(compact.Data, &compactData); err != nil || !compactData.Changed || compactData.BeforePoints != 0 || compactData.AfterPoints != 4 || compactData.ParagraphCount < 3 {
		t.Fatalf("all paragraph spacing result = %+v, error=%v, data=%s", compactData, err, compact.Data)
	}
	noop, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "set-all-paragraph-spacing-noop", DocumentID: doc.DocumentID,
		CommandID: "word.paragraph.spacing_all.set", ExpectedRevision: compact.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"before_points": 0, "after_points": 4}),
	})
	if err != nil {
		t.Fatalf("repeat all paragraph spacing: %v", err)
	}
	var noopData struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(noop.Data, &noopData); err != nil || noopData.Changed || noop.Revision != compact.Revision {
		t.Fatalf("no-op spacing result = %+v, error=%v, data=%s", noop, err, noop.Data)
	}
	stateAfterNoop := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if stateAfterNoop.Revision != compact.Revision || !stateAfterNoop.Dirty {
		t.Fatalf("all-paragraph spacing no-op changed revision/dirty state: %+v", stateAfterNoop)
	}
	path := filepath.Join(t.TempDir(), "spacing-all.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if saved.Dirty {
		t.Fatalf("saved all-paragraph spacing document remains dirty: %+v", saved)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen spacing DOCX: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	paragraphs := applicationWordParagraphs(reopened.WordBlocks())
	if len(paragraphs) < 3 {
		t.Fatalf("reopened paragraphs = %d; want at least 3", len(paragraphs))
	}
	for index, paragraph := range paragraphs {
		if paragraph.Layout.BeforePT != 0 || paragraph.Layout.AfterPT != 4 {
			t.Errorf("paragraph %d spacing = %g/%g pt; want 0/4", index, paragraph.Layout.BeforePT, paragraph.Layout.AfterPT)
		}
	}
}

func applicationWordParagraphs(blocks []document.WordBlock) []document.WordParagraph {
	var paragraphs []document.WordParagraph
	for _, block := range blocks {
		if block.Paragraph != nil {
			paragraphs = append(paragraphs, *block.Paragraph)
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					paragraphs = append(paragraphs, cell.Paragraphs...)
				}
			}
		}
	}
	return paragraphs
}
