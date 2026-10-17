package desktopui

import (
	"errors"
	"strings"
	"testing"
)

func TestOpenFailureShowsModalDialog(t *testing.T) {
	app := &App{
		openResults: make(chan openResult, 1),
	}
	app.openResults <- openResult{
		err: errors.New("read DOCX snapshot: zip: not a valid zip file"),
	}
	app.processAsyncResults()

	if !app.modal.Visible {
		t.Fatal("expected modal dialog to be visible on open failure")
	}
	if app.modal.Title != "无法打开文档" {
		t.Fatalf("modal title = %q; want '无法打开文档'", app.modal.Title)
	}
	if !strings.Contains(app.modal.Message, "文档格式校验失败") {
		t.Fatalf("modal message = %q; want friendly format validation failure message", app.modal.Message)
	}
	if len(app.modal.Actions) != 1 || app.modal.Actions[0].Label != "我知道了" {
		t.Fatalf("modal actions = %+v; want 1 action with label '我知道了'", app.modal.Actions)
	}

	// Trigger action to dismiss
	app.modal.OnAction("close")
	if app.modal.Visible {
		t.Fatal("clicking close should dismiss the modal dialog")
	}
}
