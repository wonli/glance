package desktopui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordToolbarTabsActionsAndIcons(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)
	state.word = word
	if tabs := wordToolbarTabs(state); len(tabs) != 2 || tabs[0] != wordTabHome || tabs[1] != wordTabInsert {
		t.Fatalf("Word tabs without an image selection = %#v; want Home and Insert", tabs)
	}
	state.selectedWordImageID = "img-7"
	if tabs := wordToolbarTabs(state); len(tabs) != 3 || tabs[2] != wordTabPicture {
		t.Fatalf("Word tabs with an image selection = %#v; want contextual Picture tab", tabs)
	}

	want := map[string]bool{
		"undo": true, "redo": true, "cut": true, "copy": true, "paste": true,
		"bold": true, "italic": true, "underline": true, "font-color": true,
		"align-left": true, "align-center": true, "align-right": true, "align-justify": true, "save": true,
	}
	seen := make(map[string]bool)
	for _, action := range wordToolbarActions(wordTabHome) {
		seen[action.id] = true
		if action.label == "" || action.icon == "" {
			t.Errorf("Word action %q is missing its accessible label or icon", action.id)
		}
		if _, ok := toolbarIconData(action.icon); !ok {
			t.Errorf("Word action %q references missing icon %q", action.id, action.icon)
		}
		if action.group == "单元格" || action.group == "工作表" || strings.Contains(action.id, "row-") || strings.Contains(action.id, "sheet-") {
			t.Errorf("spreadsheet-only action leaked into Word Home: %#v", action)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("Word Home is missing action %q", id)
		}
	}
	if got := wordToolbarActions(wordTabInsert); len(got) != 1 || got[0].id != "insert-image" {
		t.Fatalf("Word Insert actions = %#v; want image insertion", got)
	}
	if got := wordToolbarActions(wordTabPicture); len(got) != 5 || got[0].id != "replace-image" || got[1].id != "delete-image" || got[2].id != "resize-image" || got[3].id != "rotate-left" || got[4].id != "rotate-right" {
		t.Fatalf("Word Picture actions = %#v; want replace, delete, resize and 90° rotation", got)
	}
}

func TestWordToolbarEnablesCommandsForTheirSelectedWordObjects(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)
	state.word = word
	app := &App{documentSessionState: state}
	findAction := func(tab wordToolbarTab, id string) wordToolbarAction {
		t.Helper()
		for _, action := range wordToolbarActions(tab) {
			if action.id == id {
				return action
			}
		}
		t.Fatalf("Word toolbar action %q not found", id)
		return wordToolbarAction{}
	}
	for _, id := range []string{"bold", "align-center", "copy", "paste", "undo"} {
		if app.wordToolbarActionEnabled(findAction(wordTabHome, id)) {
			t.Errorf("Word action %q should be disabled without its required selection/history", id)
		}
	}
	if app.wordToolbarActionEnabled(findAction(wordTabInsert, "insert-image")) {
		t.Fatal("image insertion should be disabled without a selected paragraph")
	}
	blocks := word.WordBlocks()
	paragraph := blocks[0].Paragraph
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	for _, id := range []string{"bold", "align-center", "copy", "paste"} {
		if !app.wordToolbarActionEnabled(findAction(wordTabHome, id)) {
			t.Errorf("Word action %q should be enabled for selected editable text", id)
		}
	}
	if !app.wordToolbarActionEnabled(findAction(wordTabInsert, "insert-image")) {
		t.Fatal("image insertion should be enabled for a selected paragraph")
	}
	if app.wordToolbarActionEnabled(findAction(wordTabHome, "save")) {
		t.Fatal("save should remain disabled for a clean document")
	}
	if err := word.ReplaceTextRun(state.selectedWordRunID, "Edited"); err != nil {
		t.Fatal(err)
	}
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	if !app.wordToolbarActionEnabled(findAction(wordTabHome, "save")) || !app.wordToolbarActionEnabled(findAction(wordTabHome, "undo")) {
		t.Fatal("dirty Word document should enable save and undo")
	}
	state.selectedWordRunID = ""
	if app.wordToolbarActionEnabled(findAction(wordTabHome, "bold")) || app.wordToolbarActionEnabled(findAction(wordTabHome, "copy")) {
		t.Fatal("run-only Word commands should disable when no text run is selected")
	}

	imageID, err := word.InsertInlineImage(paragraph.ID, "", "image/png", tinyToolbarPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordImageID = imageID
	for _, id := range []string{"replace-image", "delete-image", "resize-image", "rotate-left", "rotate-right"} {
		if !app.wordToolbarActionEnabled(findAction(wordTabPicture, id)) {
			t.Errorf("Picture action %q should be enabled for a selected inline image", id)
		}
	}
	state.selectedWordImageID = ""
	for _, action := range wordToolbarActions(wordTabPicture) {
		if app.wordToolbarActionEnabled(action) {
			t.Errorf("Picture action %q should disable without an image selection", action.id)
		}
	}
}

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

func tinyToolbarPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 20, G: 80, B: 180, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

var _ document.WordHistory = (*docx.Document)(nil)
