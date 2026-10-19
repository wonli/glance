package desktopui

import (
	"testing"

	"glance/internal/i18n"
)

func TestPresentationRibbonLabelsResolveInBothLocalesWithoutChangingActionIDs(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	action := ribbonAction{id: "home_new_slide", label: "新建幻灯片"}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	english := app.localizedPresentationAction(action)
	if english.label != "New Slide" {
		t.Fatalf("English presentation action = %q", english.label)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	chinese := app.localizedPresentationAction(action)
	if chinese.label != "新建幻灯片" {
		t.Fatalf("Chinese presentation action = %q", chinese.label)
	}
	if english.id != action.id || chinese.id != action.id {
		t.Fatalf("locale changed presentation action ID: original=%q en=%q zh=%q", action.id, english.id, chinese.id)
	}

	tabs := app.localizedPresentationRibbonTabs(nil)
	if len(tabs) == 0 || tabs[0].id != "file" || tabs[0].label != "文件" {
		t.Fatalf("Chinese presentation tab = %+v", tabs)
	}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	tabs = app.localizedPresentationRibbonTabs(nil)
	if tabs[0].id != "file" || tabs[0].label != "File" {
		t.Fatalf("English presentation tab = %+v", tabs[0])
	}
}

func TestPresentationStatusUsesCurrentCatalog(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	message := i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}
	app.applyLocaleSelection(i18n.SelectionChinese)
	app.setStatusMessage(message, feedbackWarning, notificationSuppressed)
	if got := app.statusText(); got != "演示文稿正在处理其他操作" {
		t.Fatalf("Chinese presentation status = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := app.statusText(); got != "Another presentation operation is in progress." {
		t.Fatalf("English presentation status = %q", got)
	}
	if app.statusRef.ID != message.ID {
		t.Fatalf("locale switch changed presentation status ID to %q", app.statusRef.ID)
	}
}

func TestPresentationDialogRefsResolveAcrossLocaleSwitch(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	app.showPresentationAnimationAddOptions(&presentationViewState{})
	if app.modal.DialogID != "presentation.animation.add" || app.messageText(app.modal.TitleRef, app.modal.Title) != "Add Entrance Animation" || app.messageText(app.modal.MessageRef, app.modal.Message) != "Choose an entrance effect for the selected element." {
		t.Fatalf("English presentation dialog = id=%q title=%q body=%q", app.modal.DialogID, app.messageText(app.modal.TitleRef, app.modal.Title), app.messageText(app.modal.MessageRef, app.modal.Message))
	}
	if len(app.modal.Actions) != 4 || app.modal.Actions[0].ID != "appear" {
		t.Fatalf("presentation dialog action IDs changed: %+v", app.modal.Actions)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.messageText(app.modal.TitleRef, app.modal.Title); got != "添加进入动画" {
		t.Fatalf("Chinese presentation dialog title = %q", got)
	}
	if got := app.messageText(app.modal.Actions[0].LabelRef, app.modal.Actions[0].Label); got != "出现" {
		t.Fatalf("Chinese presentation dialog action = %q", got)
	}
	if app.modal.DialogID != "presentation.animation.add" || app.modal.Actions[0].ID != "appear" {
		t.Fatal("locale switch changed presentation dialog or action identity")
	}
}

func TestSpreadsheetRibbonLabelsResolveInBothLocales(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := app.localizedSpreadsheetToolbarText("数据"); got != "Data" {
		t.Fatalf("English spreadsheet Data label = %q", got)
	}
	if got := app.localizedSpreadsheetToolbarText("表格"); got != "Table" {
		t.Fatalf("English spreadsheet Table label = %q", got)
	}
	if got := app.localizedSpreadsheetToolbarText("公式"); got != "Formulas" {
		t.Fatalf("English spreadsheet Formulas label = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.localizedSpreadsheetToolbarText("数据"); got != "数据" {
		t.Fatalf("Chinese spreadsheet Data label = %q", got)
	}
}
