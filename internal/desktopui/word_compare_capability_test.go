package desktopui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"glance/internal/application"
	"glance/internal/format/docx"
	"glance/internal/i18n"
)

func TestWordCompareRibbonUsesRegisteredDocumentCompareCapability(t *testing.T) {
	current, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create current Word document: %v", err)
	}
	defer current.Close()
	if _, _, err := current.AppendBodyTextParagraph("当前版本内容"); err != nil {
		t.Fatalf("append current document content: %v", err)
	}

	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	app.applyLocaleSelection(i18n.SelectionChinese)
	if err := app.installDocument(loadedDocument{word: current}); err != nil {
		t.Fatalf("install current Word document: %v", err)
	}

	ctx := context.Background()
	other, err := desktop.applicationRuntime.CreateDocument(ctx, application.FormatDocx)
	if err != nil {
		t.Fatalf("create comparison Word document: %v", err)
	}
	if _, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "word-compare-fixture", DocumentID: other.DocumentID,
		CommandID: "word.paragraph.append", ExpectedRevision: other.Revision,
		Operator:  application.Operator{ID: "word-compare-test", Kind: application.OperatorBuiltInAgent},
		Arguments: json.RawMessage(`{"text":"另一版本内容"}`),
	}); err != nil {
		t.Fatalf("add comparison document content: %v", err)
	}

	beforeRequests := app.applicationRequestSeq.Load()
	app.showWordCompareDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "文档比较" {
		t.Fatalf("expected document comparison dialog, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	if app.modal.CustomLayout == nil || len(app.modal.Actions) != 1 || app.modal.Actions[0].ID != "close" {
		t.Fatalf("comparison dialog should use a scrollable document picker and one close action: custom=%v actions=%+v", app.modal.CustomLayout != nil, app.modal.Actions)
	}
	compareAction := "compare:" + other.DocumentID
	if app.wordCompareDocumentButtons[compareAction] == nil {
		t.Fatalf("comparison dialog does not offer the other open Word document in its picker: %v", app.wordCompareDocumentButtons)
	}

	app.modal.OnAction(compareAction)
	if !app.modal.Visible || !strings.Contains(app.messageText(app.modal.MessageRef, app.modal.Message), "正在比较") {
		t.Fatalf("expected asynchronous comparison progress, got visible=%v message=%q", app.modal.Visible, app.messageText(app.modal.MessageRef, app.modal.Message))
	}
	waitForWordCompareResult(t, app)
	if app.applicationRequestSeq.Load() <= beforeRequests {
		t.Fatal("Word compare did not invoke a registered application capability")
	}
	result := app.messageText(app.modal.MessageRef, app.modal.Message)
	if !strings.Contains(result, "正文块") || !strings.Contains(result, "变化路径") {
		t.Fatalf("comparison result does not show real document differences: %q", result)
	}
}

func waitForWordCompareResult(t *testing.T, app *App) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !app.modal.Visible {
			t.Fatal("Word comparison dialog closed before the result arrived")
		}
		if !strings.Contains(app.messageText(app.modal.MessageRef, app.modal.Message), "正在比较") {
			return
		}
		app.processAsyncResults()
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for Word comparison result: %q", app.messageText(app.modal.MessageRef, app.modal.Message))
}
