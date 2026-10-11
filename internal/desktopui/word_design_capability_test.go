package desktopui

import (
	"context"
	"testing"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordDesignAndTranslateRibbonsUseRegisteredCapabilities(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create Word fixture: %v", err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	_, runID, err := doc.AppendBodyTextParagraph("原文内容")
	if err != nil {
		t.Fatalf("append Word fixture paragraph: %v", err)
	}

	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	if err := app.installDocument(loadedDocument{word: doc}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	app.documentSessionState.selectedWordRunID = runID
	events := make(chan application.Event, 4)
	cancel := desktop.applicationRuntime.On(context.Background(), application.EventFilter{
		Types: []string{application.EventDocumentChanged}, DocumentID: app.documentID,
	}, func(event application.Event) { events <- event })
	t.Cleanup(cancel)

	app.showWordThemesDialog()
	app.modal.OnAction("theme_business")
	if event := waitWordCapabilityEvent(t, app, events); event.CommandID != "word.paragraph.spacing_all.set" {
		t.Fatalf("Word theme Ribbon command = %q, want word.paragraph.spacing_all.set", event.CommandID)
	}
	paragraphs := allWordParagraphs(doc.WordBlocks())
	if len(paragraphs) == 0 || paragraphs[0].Layout.BeforePT != 2 || paragraphs[0].Layout.AfterPT != 10 {
		t.Fatalf("Word theme spacing = %+v; want 2pt before and 10pt after", paragraphs)
	}

	app.showWordColorsDialog()
	app.modal.OnAction("scheme_mint")
	if event := waitWordCapabilityEvent(t, app, events); event.CommandID != "word.page.background.set" {
		t.Fatalf("Word colors Ribbon command = %q, want word.page.background.set", event.CommandID)
	}
	background, err := doc.GetPageBackgroundColor()
	if err != nil || background != "E8F5E9" {
		t.Fatalf("Word page background = %q, error=%v; want E8F5E9", background, err)
	}

	app.showWordFontsDialog()
	app.modal.OnAction("font_song")
	if event := waitWordCapabilityEvent(t, app, events); event.CommandID != "word.text_run.font_family.set" {
		t.Fatalf("Word fonts Ribbon command = %q, want word.text_run.font_family.set", event.CommandID)
	}
	if got := wordRunStyle(doc.WordBlocks(), runID).FontFamily; got != "SimSun" {
		t.Fatalf("Word run font family = %q; want SimSun", got)
	}

	app.showWordTranslateDialog()
	app.modal.Input.SetText("译文内容")
	app.modal.OnAction("confirm")
	if event := waitWordCapabilityEvent(t, app, events); event.CommandID != "word.text_run.replace" {
		t.Fatalf("Word translate Ribbon command = %q, want word.text_run.replace", event.CommandID)
	}
	if !containsWordText(doc.WordBlocks(), "译文内容") {
		t.Fatal("Word translate action did not replace the selected run through the registered command")
	}

	app.showWordDesignerDialog()
	app.modal.OnAction("scheme_gov")
	if event := waitWordCapabilityEvent(t, app, events); event.CommandID != "word.paragraph.spacing_all.set" {
		t.Fatalf("Word designer Ribbon command = %q, want word.paragraph.spacing_all.set", event.CommandID)
	}
	paragraphs = allWordParagraphs(doc.WordBlocks())
	if len(paragraphs) == 0 || paragraphs[0].Layout.BeforePT != 0 || paragraphs[0].Layout.AfterPT != 8 {
		t.Fatalf("Word designer spacing = %+v; want 0pt before and 8pt after", paragraphs)
	}
}

func wordRunStyle(blocks []document.WordBlock, runID string) document.WordTextStyle {
	for _, block := range blocks {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == runID {
				return inline.Text.Style
			}
		}
	}
	return document.WordTextStyle{}
}

func TestWordDictationRibbonUsesRegisteredTextInsertCapability(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create Word fixture: %v", err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	paragraphID, runID, err := doc.AppendBodyTextParagraph("会议记录")
	if err != nil {
		t.Fatalf("append Word fixture paragraph: %v", err)
	}

	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	if err := app.installDocument(loadedDocument{word: doc}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	app.documentSessionState.selectedWordRunID = runID
	app.documentSessionState.selectedWordParagraphID = paragraphID
	events := make(chan application.Event, 2)
	cancel := desktop.applicationRuntime.On(context.Background(), application.EventFilter{
		Types: []string{application.EventDocumentChanged}, DocumentID: app.documentID,
	}, func(event application.Event) { events <- event })
	t.Cleanup(cancel)

	app.showWordDictateDialog()
	app.modal.Input.SetText("完成本周汇总")
	app.modal.OnAction("confirm")
	if event := waitWordCapabilityEvent(t, app, events); event.CommandID != "word.text.insert" {
		t.Fatalf("Word dictation Ribbon command = %q, want word.text.insert", event.CommandID)
	}
	if !containsWordText(doc.WordBlocks(), "会议记录 完成本周汇总") {
		t.Fatalf("Word dictation did not append to selected text run: %+v", doc.WordBlocks())
	}

	app.documentSessionState.selectedWordRunID = ""
	app.showWordDictateDialog()
	app.modal.Input.SetText("下周继续跟进")
	app.modal.OnAction("confirm")
	if event := waitWordCapabilityEvent(t, app, events); event.CommandID != "word.text.insert" {
		t.Fatalf("Word dictation fallback command = %q, want word.text.insert", event.CommandID)
	}
	if !containsWordText(doc.WordBlocks(), "下周继续跟进") {
		t.Fatalf("Word dictation did not insert its no-selection text box: %+v", doc.WordBlocks())
	}
}
