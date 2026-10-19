package desktopui

import (
	"errors"
	"strings"
	"testing"

	"glance/internal/i18n"
)

func TestLocalizedErrorKeepsSourceDetailsOutOfPrimaryMessage(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	source := errors.New("failed to parse /tmp/reports/private.docx: corrupt XML")
	app.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, source, notificationToast)
	primary := app.text(app.feedback.messageRef)
	if strings.Contains(primary, source.Error()) || strings.Contains(primary, "/tmp/reports/private.docx") {
		t.Fatalf("primary localized error exposed source diagnostics: %q", primary)
	}
	if app.feedback.detail != source.Error() {
		t.Fatalf("stored diagnostic detail=%q; want %q", app.feedback.detail, source.Error())
	}
	app.showFeedbackErrorDetails()
	if app.modal.DialogID != "common.error-details" {
		t.Fatalf("error details dialog ID = %q", app.modal.DialogID)
	}
	detail, ok := app.modal.MessageRef.Args["Details"].(string)
	if !ok || !strings.Contains(detail, "/tmp/reports/private.docx") {
		t.Fatalf("on-demand diagnostic detail = %#v", app.modal.MessageRef.Args["Details"])
	}
}

func TestInitialStatusResolvesOnlyWhenReadForRendering(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime(), initialStatus: i18n.Message{ID: "status.initial.open_document"}}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	english := app.statusText()
	app.applyLocaleSelection(i18n.SelectionChinese)
	chinese := app.statusText()
	if english == chinese || !strings.Contains(english, ".xlsx") || !strings.Contains(chinese, ".xlsx") {
		t.Fatalf("initial status did not resolve dynamically: en=%q zh=%q", english, chinese)
	}
}
