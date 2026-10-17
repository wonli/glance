package desktopui

import (
	"testing"

	"glance/internal/document"
)

func TestWordCommentTargetArgumentsCaptureSelectedPageTextRange(t *testing.T) {
	first := document.WordTextRun{ID: "run-a", Text: "甲豆", Editable: true}
	second := document.WordTextRun{ID: "run-b", Text: "豆乙", Editable: true}
	paragraph := document.WordParagraph{ID: "paragraph", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &first},
		{Kind: document.WordTextInline, Text: &second},
	}}
	state := newDocumentSessionState("comment-selection.docx")
	state.wordBlocks = []document.WordBlock{{ID: paragraph.ID, Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	state.wordPageTextSelection = &wordPageTextSelection{
		anchor:   wordPageCaretPosition{runID: first.ID, runeOffset: 1},
		focus:    wordPageCaretPosition{runID: second.ID, runeOffset: 1},
		runOrder: map[string]int{first.ID: 0, second.ID: 1},
	}

	args, selectedText, hasSelection, ok := wordCommentTargetArguments(state)
	if !ok || !hasSelection || selectedText != "豆豆" {
		t.Fatalf("comment selection target = %+v, text=%q selected=%v ok=%v", args, selectedText, hasSelection, ok)
	}
	if args.RunId != first.ID || args.StartRunId != first.ID || args.StartOffset == nil || *args.StartOffset != 1 || args.EndRunId != second.ID || args.EndOffset == nil || *args.EndOffset != 1 {
		t.Fatalf("comment range arguments = %+v; want run-a[1] through run-b[1]", args)
	}
}

func TestWordCommentTargetArgumentsPreserveWholeRunFallback(t *testing.T) {
	run := document.WordTextRun{ID: "run", Text: "正文", Editable: true}
	paragraph := document.WordParagraph{ID: "paragraph", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &run}}}
	state := newDocumentSessionState("comment-selection.docx")
	state.wordBlocks = []document.WordBlock{{ID: paragraph.ID, Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	state.selectedWordRunID = run.ID

	args, _, hasSelection, ok := wordCommentTargetArguments(state)
	if !ok || hasSelection || args.RunId != run.ID || args.StartOffset != nil || args.EndOffset != nil {
		t.Fatalf("whole-run fallback = %+v selected=%v ok=%v", args, hasSelection, ok)
	}
}

func TestWordCommentTargetArgumentsRejectCrossParagraphSelection(t *testing.T) {
	first := document.WordTextRun{ID: "run-a", Text: "甲", Editable: true}
	second := document.WordTextRun{ID: "run-b", Text: "乙", Editable: true}
	firstParagraph := document.WordParagraph{ID: "paragraph-a", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &first}}}
	secondParagraph := document.WordParagraph{ID: "paragraph-b", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &second}}}
	state := newDocumentSessionState("comment-selection.docx")
	state.wordBlocks = []document.WordBlock{
		{ID: firstParagraph.ID, Kind: document.WordParagraphBlock, Paragraph: &firstParagraph},
		{ID: secondParagraph.ID, Kind: document.WordParagraphBlock, Paragraph: &secondParagraph},
	}
	state.wordPageTextSelection = &wordPageTextSelection{
		anchor:   wordPageCaretPosition{runID: first.ID, runeOffset: 0},
		focus:    wordPageCaretPosition{runID: second.ID, runeOffset: 1},
		runOrder: map[string]int{first.ID: 0, second.ID: 1},
	}

	if _, _, _, ok := wordCommentTargetArguments(state); ok {
		t.Fatal("cross-paragraph comment selection should be rejected before showing the dialog")
	}
}
