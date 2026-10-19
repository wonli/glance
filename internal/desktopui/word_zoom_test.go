package desktopui

import (
	"strings"
	"testing"

	"glance/internal/format/docx"
)

func TestWordRibbonZoomDialogAppliesPercentAndRejectsOutOfRange(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("zoom.docx")
	state.word = word
	app := &App{documentSessionState: state}
	action := findWordViewTestAction(t, "view-zoom")
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("View > Zoom should be enabled for a Word document")
	}

	app.executeWordToolbarAction(action.id)
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "缩放" || !app.modal.HasInput || app.modal.Input.Text() != "100" {
		t.Fatalf("Zoom dialog = visible:%v title:%q input:%q hasInput:%v", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title), app.modal.Input.Text(), app.modal.HasInput)
	}
	app.modal.Input.SetText("125%")
	app.modal.OnAction("confirm")
	if app.modal.Visible || state.wordLayoutZoom != 1.25 {
		t.Fatalf("applied zoom: modal visible=%v zoom=%.2f; want closed and 1.25", app.modal.Visible, state.wordLayoutZoom)
	}

	app.executeWordToolbarAction(action.id)
	app.modal.Input.SetText("200")
	app.modal.OnAction("confirm")
	if !app.modal.Visible || state.wordLayoutZoom != 1.25 || !strings.Contains(app.statusText(), "50–150") {
		t.Fatalf("out-of-range zoom: modal visible=%v zoom=%.2f status=%q", app.modal.Visible, state.wordLayoutZoom, app.statusText())
	}
	app.modal.OnAction("cancel")
	if app.modal.Visible || state.wordLayoutZoom != 1.25 {
		t.Fatalf("cancel changed zoom: modal visible=%v zoom=%.2f", app.modal.Visible, state.wordLayoutZoom)
	}
}

func findWordViewTestAction(t *testing.T, id string) ribbonAction {
	t.Helper()
	for _, action := range wordToolbarActions(wordTabView) {
		if action.id == id {
			return action
		}
	}
	t.Fatalf("Word View action %q not found", id)
	return ribbonAction{}
}
