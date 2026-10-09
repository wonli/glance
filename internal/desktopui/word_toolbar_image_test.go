package desktopui

import (
	"testing"
	"time"

	"glance/internal/format/docx"
)

func TestRotateSelectedWordImageRunsAsyncAndKeepsSelection(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	imageID, err := word.InsertInlineImage(paragraph.ID, "", "image/png", tinyToolbarPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("rotate.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordImageID = imageID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	app.rotateSelectedWordImage(90)
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("rotate selected image: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Word image rotation")
	}
	rotated := findWordImage(state.wordBlocks, imageID)
	if rotated == nil || rotated.RotationDegrees != 90 {
		t.Fatalf("selected image after rotation = %#v; want 90 degrees", rotated)
	}
	if state.selectedWordImageID != imageID {
		t.Fatalf("selected image after rotation = %q; want %q", state.selectedWordImageID, imageID)
	}
	if !word.CanUndo() {
		t.Fatal("rotating an image did not add an undo step")
	}
}
