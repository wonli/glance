package desktopui

import (
	"strings"
	"testing"
	"time"

	"glance/internal/format/docx"
	"glance/internal/i18n"
)

func TestWordMacrosRibbonUsesRegisteredInspectionCapability(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create Word document: %v", err)
	}
	defer doc.Close()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	app.applyLocaleSelection(i18n.SelectionChinese)
	if err := app.installDocument(loadedDocument{word: doc}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	before := app.applicationRequestSeq.Load()
	app.showWordMacrosDialog()
	message := func() string { return app.messageText(app.modal.MessageRef, app.modal.Message) }
	if !app.modal.Visible || app.text(app.modal.TitleRef) != "宏与安全性" || !strings.Contains(message(), "正在检查") {
		t.Fatalf("expected asynchronous macro inspection, got visible=%v title=%q message=%q", app.modal.Visible, app.text(app.modal.TitleRef), message())
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && strings.Contains(message(), "正在检查") {
		app.processAsyncResults()
		time.Sleep(time.Millisecond)
	}
	if strings.Contains(message(), "正在检查") {
		t.Fatalf("timed out waiting for macro inspection: %q", message())
	}
	if app.applicationRequestSeq.Load() <= before {
		t.Fatal("Word macros Ribbon did not invoke a registered application capability")
	}
	if !strings.Contains(message(), "未发现宏部件") || !strings.Contains(message(), "不会执行") {
		t.Fatalf("macro dialog did not report the actual inspection and execution boundary: %q", message())
	}
}
