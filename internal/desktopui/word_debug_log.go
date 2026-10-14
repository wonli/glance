package desktopui

import (
	"fmt"
	"log"
	"strings"

	"gioui.org/widget"

	"glance/internal/document"
)

func logWordDebug(format string, args ...any) {
	log.Printf("[word-debug] "+format, args...)
}

func wordDebugLayoutRevision(state *documentSessionState) uint64 {
	if state == nil || state.wordLayoutResult == nil {
		return 0
	}
	return state.wordLayoutResult.Revision
}

func wordDebugError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// wordDebugParagraphNeighborhood reports structure and rune counts without
// putting document text into the user's console log.
func wordDebugParagraphNeighborhood(blocks []document.WordBlock, targetRunID string) string {
	type paragraphSummary struct {
		ref  string
		runs []string
	}
	paragraphs := make([]paragraphSummary, 0, len(blocks))
	targetIndex := -1
	appendParagraph := func(location string, paragraph document.WordParagraph) {
		ref := paragraph.ID
		if location != "" {
			ref = location + "/" + ref
		}
		summary := paragraphSummary{ref: ref}
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil {
				continue
			}
			textRun := inline.Text
			summary.runs = append(summary.runs, fmt.Sprintf("%q:%d", textRun.ID, len([]rune(textRun.Text))))
			if textRun.ID == targetRunID {
				targetIndex = len(paragraphs)
			}
		}
		if len(summary.runs) == 0 {
			summary.runs = append(summary.runs, "no-text-run")
		}
		paragraphs = append(paragraphs, summary)
	}
	for blockIndex, block := range blocks {
		switch block.Kind {
		case document.WordParagraphBlock:
			if block.Paragraph != nil {
				appendParagraph(fmt.Sprintf("block=%d", blockIndex), *block.Paragraph)
			}
		case document.WordTableBlock:
			if block.Table == nil {
				continue
			}
			for rowIndex, row := range block.Table.Rows {
				for cellIndex, cell := range row.Cells {
					for _, paragraph := range cell.Paragraphs {
						appendParagraph(fmt.Sprintf("table=%q/row=%d/cell=%q/index=%d", block.ID, rowIndex, cell.ID, cellIndex), paragraph)
					}
				}
			}
		}
	}
	if targetIndex < 0 {
		return fmt.Sprintf("paragraphs=%d target_run=%q missing", len(paragraphs), targetRunID)
	}
	start, end := max(0, targetIndex-2), min(len(paragraphs), targetIndex+3)
	nearby := make([]string, 0, end-start)
	for _, paragraph := range paragraphs[start:end] {
		nearby = append(nearby, fmt.Sprintf("%q{%s}", paragraph.ref, strings.Join(paragraph.runs, ",")))
	}
	return fmt.Sprintf("paragraphs=%d target_index=%d nearby=[%s]", len(paragraphs), targetIndex, strings.Join(nearby, ";"))
}

func wordDebugEditorSelection(editor *widget.Editor) (start, end, length int) {
	if editor == nil {
		return -1, -1, 0
	}
	start, end = editor.Selection()
	return start, end, editor.Len()
}
