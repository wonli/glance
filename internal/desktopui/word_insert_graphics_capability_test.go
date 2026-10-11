package desktopui

import (
	"context"
	"strings"
	"testing"
	"time"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordShapeAndWordArtRibbonsUseRegisteredTextBoxCommand(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create Word fixture: %v", err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	if err := app.installDocument(loadedDocument{word: doc}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	events := make(chan application.Event, 4)
	cancel := desktop.applicationRuntime.On(context.Background(), application.EventFilter{Types: []string{application.EventDocumentChanged}}, func(event application.Event) {
		events <- event
	})
	t.Cleanup(cancel)

	app.showWordShapesDialog()
	app.modal.Input.SetText("风险说明")
	app.modal.OnAction("round_rect")
	shapeEvent := waitWordCapabilityEvent(t, app, events)
	if shapeEvent.CommandID != "word.text_box.insert" {
		t.Fatalf("Word shape Ribbon command = %q, want registered word.text_box.insert", shapeEvent.CommandID)
	}
	if !containsWordText(wordBlocksForState(app.documentSessionState), "【形状·圆角矩形框】风险说明") {
		t.Fatal("registered shape action did not insert its selected text-box content")
	}

	app.showWordWordArtDialog()
	app.modal.Input.SetText("季度总结")
	app.modal.OnAction("style_neon")
	wordArtEvent := waitWordCapabilityEvent(t, app, events)
	if wordArtEvent.CommandID != "word.text_box.insert" {
		t.Fatalf("WordArt Ribbon command = %q, want registered word.text_box.insert", wordArtEvent.CommandID)
	}
	if !containsWordText(wordBlocksForState(app.documentSessionState), "【艺术字·霓虹光效】 季度总结") {
		t.Fatal("registered WordArt action did not insert its selected text-box content")
	}

	app.showWordChartDialog()
	app.modal.OnAction("column")
	chartEvent := waitWordCapabilityEvent(t, app, events)
	if chartEvent.CommandID != "word.table.insert" {
		t.Fatalf("Word chart Ribbon command = %q, want registered word.table.insert", chartEvent.CommandID)
	}
	if !hasWordTableDimensions(wordBlocksForState(app.documentSessionState), 3, 3) {
		t.Fatal("chart template action did not insert its 3-by-3 source table")
	}

	app.showWordEquationDialog()
	app.modal.OnAction("pythagorean")
	equationEvent := waitWordCapabilityEvent(t, app, events)
	if equationEvent.CommandID != "word.equation.insert" {
		t.Fatalf("Word equation Ribbon command = %q, want registered word.equation.insert", equationEvent.CommandID)
	}
	if !containsWordText(wordBlocksForState(app.documentSessionState), "a² + b² = c²") {
		t.Fatal("registered equation action did not insert its expression as editable text")
	}
}

func waitWordCapabilityEvent(t *testing.T, app *App, events <-chan application.Event) application.Event {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var event application.Event
	for time.Now().Before(deadline) {
		select {
		case event = <-events:
			for time.Now().Before(deadline) {
				if len(app.wordCommandResults) > 0 {
					app.processAsyncResults()
					return event
				}
				time.Sleep(time.Millisecond)
			}
		case <-time.After(time.Millisecond):
			app.processAsyncResults()
		}
	}
	t.Fatal("timed out waiting for registered Word capability event")
	return application.Event{}
}

func containsWordText(blocks []document.WordBlock, want string) bool {
	for _, block := range blocks {
		if block.Paragraph != nil && containsWordParagraphText(*block.Paragraph, want) {
			return true
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					for _, paragraph := range cell.Paragraphs {
						if containsWordParagraphText(paragraph, want) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func containsWordParagraphText(paragraph document.WordParagraph, want string) bool {
	var text strings.Builder
	for _, inline := range paragraph.Inlines {
		if inline.Text != nil {
			text.WriteString(inline.Text.Text)
		}
	}
	return strings.Contains(text.String(), want)
}

func hasWordTableDimensions(blocks []document.WordBlock, rows, columns int) bool {
	for _, block := range blocks {
		if block.Table == nil || len(block.Table.Rows) != rows {
			continue
		}
		valid := true
		for _, row := range block.Table.Rows {
			if len(row.Cells) != columns {
				valid = false
				break
			}
		}
		if valid {
			return true
		}
	}
	return false
}
