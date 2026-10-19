package desktopui

import (
	"testing"
	"time"

	"glance/internal/i18n"
)

func TestFeedbackKindAndNotificationDoNotDependOnText(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	app.applyLocaleSelection(i18n.SelectionChinese)
	app.setStatusMessage(i18n.Message{ID: "common.feedback.operation_failed"}, feedbackError, notificationToast)
	if app.feedback.kind != feedbackError || app.feedback.policy != notificationToast {
		t.Fatalf("Chinese feedback metadata = kind %v, policy %v", app.feedback.kind, app.feedback.policy)
	}
	chineseText := app.text(app.feedback.messageRef)
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := app.text(app.feedback.messageRef); got == chineseText {
		t.Fatalf("feedback did not localize after switching from Chinese: %q", got)
	}
	if app.feedback.kind != feedbackError || app.feedback.policy != notificationToast {
		t.Fatalf("English feedback metadata changed with text: kind %v, policy %v", app.feedback.kind, app.feedback.policy)
	}

	app.setStatusMessage(i18n.Message{ID: "common.feedback.operation_failed"}, feedbackWarning, notificationSuppressed)
	if app.feedback.policy != notificationSuppressed || app.feedback.kind != feedbackWarning {
		t.Fatalf("suppressed feedback metadata = kind %v, policy %v", app.feedback.kind, app.feedback.policy)
	}
}

func TestFeedbackExpirationAndNotificationPolicyAreExplicit(t *testing.T) {
	var state feedbackState
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	state.showMessage(i18n.Message{ID: "common.feedback.operation_failed"}, feedbackSuccess, notificationToast, "", now)
	if state.kind != feedbackSuccess || !state.visible(now.Add(time.Second)) {
		t.Fatal("explicit success feedback missing")
	}
	until := state.until
	if state.visible(until) {
		t.Fatal("toast did not expire")
	}
	state.showMessage(i18n.Message{ID: "common.feedback.operation_failed"}, feedbackError, notificationSuppressed, "", until)
	if state.visible(until) {
		t.Fatal("suppressed feedback became a toast")
	}
	state.dismiss()
	if state.visible(until) {
		t.Fatal("dismissed toast remains visible")
	}
}

func TestFeedbackRepeatsCompletedOperationAfterDismissal(t *testing.T) {
	a := New(nil, "")
	now := time.Now()
	a.setStatusMessage(i18n.Message{ID: "json.status.valid"}, feedbackSuccess, notificationToast)
	a.feedback.dismiss()
	a.setStatusMessage(i18n.Message{ID: "json.status.valid"}, feedbackSuccess, notificationToast)
	if !a.feedback.visible(now.Add(time.Second)) {
		t.Fatal("repeated operation completion was mistaken for repeated frame")
	}
}
