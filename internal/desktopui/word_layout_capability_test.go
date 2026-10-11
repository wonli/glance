package desktopui

import (
	"context"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordLayoutAlignRibbonUsesRegisteredParagraphCapability(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create Word document: %v", err)
	}
	t.Cleanup(func() { _ = word.Close() })
	if _, _, err := word.AppendBodyTextParagraph("需要对齐的段落"); err != nil {
		t.Fatalf("append paragraph: %v", err)
	}
	if err := app.installDocument(loadedDocument{word: word}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	state := app.documentSessionState
	paragraphs := allWordParagraphs(state.wordBlocks)
	if len(paragraphs) == 0 {
		t.Fatal("installed Word document has no paragraph")
	}
	state.selectedWordParagraphID = paragraphs[0].ID
	before, err := desktop.applicationRuntime.GetDocument(ctx, state.documentID)
	if err != nil {
		t.Fatalf("get document before Ribbon action: %v", err)
	}
	requestSequence := app.applicationRequestSeq.Load()

	app.showWordLayoutAlignDialog()
	if !app.modal.Visible || app.modal.Title != "排列对齐" {
		t.Fatalf("alignment dialog visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("center")
	app.modal.OnAction("confirm")
	deadline := time.Now().Add(3 * time.Second)
	for state.wordRevisionAtomic.Load() == before.Revision && time.Now().Before(deadline) {
		app.processWordCommandAsyncResult()
		time.Sleep(time.Millisecond)
	}
	if state.wordRevisionAtomic.Load() != before.Revision+1 {
		t.Fatalf("Ribbon action revision = %d, want %d; status=%q", state.wordRevisionAtomic.Load(), before.Revision+1, app.status)
	}
	if app.applicationRequestSeq.Load() != requestSequence+1 {
		t.Fatal("Ribbon alignment did not invoke the registered application capability")
	}
	var aligned bool
	for _, paragraph := range allWordParagraphs(state.wordBlocks) {
		if paragraph.ID == state.selectedWordParagraphID && paragraph.Alignment == document.WordAlignCenter {
			aligned = true
		}
	}
	if !aligned {
		t.Fatalf("Ribbon action did not refresh the centered paragraph projection: %+v", allWordParagraphs(state.wordBlocks))
	}
	after, err := desktop.applicationRuntime.GetDocument(ctx, state.documentID)
	if err != nil || after.Revision != before.Revision+1 {
		t.Fatalf("application runtime revision after Ribbon action = %+v, error=%v", after, err)
	}
}
