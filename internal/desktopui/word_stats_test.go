package desktopui

import (
	"strings"
	"testing"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestReviewWordCountShowsDetailsForDraftsAndTables(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("sample.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{
		{Kind: document.WordParagraphBlock, Paragraph: &document.WordParagraph{ID: "p1", Inlines: []document.WordInline{
			{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r1", Text: "旧值", Editable: true}},
		}}},
		{Kind: document.WordParagraphBlock, Paragraph: &document.WordParagraph{ID: "p2", Inlines: []document.WordInline{
			{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r2", Text: "中文"}},
		}}},
		{Kind: document.WordTableBlock, Table: &document.WordTable{Rows: []document.WordTableRow{{Cells: []document.WordTableCell{{ID: "c1", Paragraphs: []document.WordParagraph{{ID: "p3", Inlines: []document.WordInline{
			{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r3", Text: "表格"}},
		}}}}}}}}},
	}
	state.wordRunDrafts = map[string]string{"r1": "你好 world"}
	app := &App{documentSessionState: state}

	if !app.wordToolbarActionEnabled(wordActionByID("review-word-count")) {
		t.Fatal("Review > Word Count should be enabled for a readable Word document")
	}
	app.executeWordToolbarAction("review-word-count")
	if !app.modal.Visible {
		t.Fatal("Review > Word Count should open a details dialog")
	}
	for _, want := range []string{"字符数（不含空白）：11", "字符数（含空白）：12", "段落数：3", "表格数：1"} {
		if !strings.Contains(app.modal.Message, want) {
			t.Errorf("word-count details do not include %q: %q", want, app.modal.Message)
		}
	}
}

func TestWordStatusCountIncludesWhitespace(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	state.wordBlocks = []document.WordBlock{{
		Kind: document.WordParagraphBlock,
		Paragraph: &document.WordParagraph{ID: "p1", Inlines: []document.WordInline{{
			Kind: document.WordTextInline,
			Text: &document.WordTextRun{ID: "r1", Text: "甲 乙\n丙"},
		}}},
	}}

	if got := wordStatusWordCount(state); got != "5 个字" {
		t.Fatalf("Word status count = %q; want the 5 characters including spaces and line breaks", got)
	}
}

func wordActionByID(id string) ribbonAction {
	for _, action := range wordReviewActions {
		if action.id == id {
			return action
		}
	}
	return ribbonAction{id: id}
}
