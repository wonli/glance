package desktopui

import (
	"errors"
	"strings"
	"testing"

	"glance/internal/i18n"
)

func TestOpenFailureShowsModalDialog(t *testing.T) {
	app := &App{
		openResults: make(chan openResult, 1),
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	app.openResults <- openResult{
		err: errors.New("read DOCX snapshot: zip: not a valid zip file"),
	}
	app.processAsyncResults()

	if !app.modal.Visible {
		t.Fatal("expected modal dialog to be visible on open failure")
	}
	if app.modal.DialogID != "document.open.failure" || app.text(app.modal.TitleRef) != "无法打开文档" {
		t.Fatalf("modal ID=%q title=%q; want document.open.failure / 无法打开文档", app.modal.DialogID, app.text(app.modal.TitleRef))
	}
	if !strings.Contains(app.text(app.modal.MessageRef), "文档格式校验失败") {
		t.Fatalf("modal message = %q; want friendly format validation failure message", app.text(app.modal.MessageRef))
	}
	if len(app.modal.Actions) != 1 || app.text(app.modal.Actions[0].LabelRef) != "关闭" {
		t.Fatalf("modal actions = %+v; want 1 close action", app.modal.Actions)
	}

	// Trigger action to dismiss
	app.modal.OnAction("close")
	if app.modal.Visible {
		t.Fatal("clicking close should dismiss the modal dialog")
	}
}
