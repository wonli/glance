package desktopui

import (
	"errors"
	"strings"
	"testing"

	"glance/internal/i18n"
	"glance/internal/wordlayout"
)

func TestWordRibbonLabelsResolveInBothLocalesWithoutChangingActionIDs(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	englishTab := app.text(wordToolbarTabMessage(wordTabHome))
	english := app.localizedWordAction(ribbonAction{id: "review-track", label: "修订"})
	if englishTab != "Home" || english.label != "Track Changes" {
		t.Fatalf("English Word ribbon labels: tab=%q action=%q", englishTab, english.label)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	chineseTab := app.text(wordToolbarTabMessage(wordTabHome))
	chinese := app.localizedWordAction(ribbonAction{id: "review-track", label: "修订"})
	if chineseTab != "开始" || chinese.label != "修订" {
		t.Fatalf("Chinese Word ribbon labels: tab=%q action=%q", chineseTab, chinese.label)
	}
	if english.id != chinese.id || english.id != "review-track" {
		t.Fatalf("locale changed Word action identity: English=%q Chinese=%q", english.id, chinese.id)
	}
}

func TestWordStaticStatusUsesCurrentCatalog(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	message := i18n.Message{ID: "word.status.inline.inserted_a_hyperlink_508d90"}
	app.applyLocaleSelection(i18n.SelectionChinese)
	app.setStatusMessage(message, feedbackSuccess, notificationToast)
	if got := app.statusText(); got != "已成功插入超链接" {
		t.Fatalf("Chinese Word status = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := app.statusText(); got != "Inserted a hyperlink." {
		t.Fatalf("English Word status = %q", got)
	}
	if app.statusRef.ID != message.ID {
		t.Fatalf("locale switch changed Word status ID to %q", app.statusRef.ID)
	}
}

func TestWordPaginationStatusUsesCurrentLocaleAndPreservesPageState(t *testing.T) {
	state := newDocumentSessionState("draft.docx")
	state.wordLayoutResult = &wordlayout.Result{Pages: []wordlayout.PageScene{{Index: 0}, {Index: 1}}, CanPreview: true}
	state.wordLayoutActivePage = 1
	app := &App{desktopRuntime: newDesktopRuntime(), documentSessionState: state}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := app.localizedWordPaginationStatusText(state); got != "Page 2 of 2" {
		t.Fatalf("English pagination status = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.localizedWordPaginationStatusText(state); got != "第 2 页，共 2 页" {
		t.Fatalf("Chinese pagination status = %q", got)
	}
	if app.documentSessionState != state || state.wordLayoutActivePage != 1 || len(state.wordLayoutResult.Pages) != 2 {
		t.Fatal("locale switch changed Word pagination state")
	}
}

func TestWordDialogDraftAndActionIDSurviveLocaleSwitch(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	app.modal.ShowInputMessage("word.insert.bookmark", i18n.Message{ID: "word.insert.bookmark.title"},
		i18n.Message{ID: "word.insert.bookmark.body"}, "Bookmark1",
		i18n.Message{ID: "word.insert.bookmark.hint"}, nil,
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.confirm"}},
	)
	app.modal.Input.SetText("chapter_start")
	app.applyLocaleSelection(i18n.SelectionChinese)
	if app.modal.DialogID != "word.insert.bookmark" || app.modal.Input.Text() != "chapter_start" {
		t.Fatalf("locale switch changed Word dialog ID or draft: id=%q draft=%q", app.modal.DialogID, app.modal.Input.Text())
	}
	if len(app.modal.Actions) != 1 || app.modal.Actions[0].ID != "confirm" {
		t.Fatalf("locale switch changed Word dialog action identity: %+v", app.modal.Actions)
	}
}

func TestWordAsyncReviewResultUsesLocaleAtCompletion(t *testing.T) {
	state := newDocumentSessionState("review.docx")
	app := &App{
		desktopRuntime:          newDesktopRuntime(),
		documentSessionState:    state,
		documentGeneration:      9,
		wordEditorReviewRequest: 4,
	}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	app.modal.ShowMessage(dialogIDWordEditorReview, i18n.Message{ID: "word.review.editor.title"}, i18n.Message{ID: "word.review.editor.scanning"}, nil,
		modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
	app.applyLocaleSelection(i18n.SelectionChinese)
	app.applyWordEditorReviewResult(wordEditorReviewAsyncResult{
		state: state, generation: 9, request: 4, revision: state.wordRevisionAtomic.Load(),
		data: []byte(`{"heuristic_score":80,"scanned_blocks":2,"total_blocks":2,"characters_with_whitespace":20,"characters_without_whitespace":18,"paragraphs":1,"tables":0,"findings":[],"findings_truncated":false,"has_more":false}`),
	})
	message := app.messageText(app.modal.MessageRef, app.modal.Message)
	if !strings.Contains(message, "文档编辑器启发式评分") || !strings.Contains(message, "轻量检查不执行") {
		t.Fatalf("late Word review result did not use the current locale: %q", message)
	}
	if app.modal.DialogID != dialogIDWordEditorReview || len(app.modal.Actions) != 1 || app.modal.Actions[0].ID != "close" {
		t.Fatalf("async result changed modal identity or action ID: %+v", app.modal)
	}
}

func TestWordCompareStaleResultUsesCurrentLocale(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime(), wordCompareRequest: 7}
	app.modal.ShowMessage(dialogIDWordCompare, i18n.Message{ID: "word.compare.dialog.title"}, i18n.Message{ID: "word.compare.dialog.body.running"}, nil)
	app.applyLocaleSelection(i18n.SelectionEnglish)
	app.applyWordCompareResult(wordCompareAsyncResult{request: 7})
	if app.modal.MessageRef.ID != "word.compare.result.stale_current_document" {
		t.Fatalf("stale compare result ref = %+v", app.modal.MessageRef)
	}
	if got := app.messageText(app.modal.MessageRef, app.modal.Message); got != "The active document changed. The comparison result was discarded." {
		t.Fatalf("English stale comparison message = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.messageText(app.modal.MessageRef, app.modal.Message); got != "当前文档已切换，比较结果已丢弃。" {
		t.Fatalf("Chinese stale comparison message = %q", got)
	}
	if app.modal.DialogID != dialogIDWordCompare {
		t.Fatalf("stale comparison changed modal identity to %q", app.modal.DialogID)
	}
}

func TestWordCompareErrorKeepsLocalizedPrimaryAndSourceDetails(t *testing.T) {
	state := newDocumentSessionState("compare.docx")
	app := &App{desktopRuntime: newDesktopRuntime(), wordCompareRequest: 9, documentSessionState: state, documentGeneration: 4}
	app.modal.ShowMessage(dialogIDWordCompare, i18n.Message{ID: "word.compare.dialog.title"}, i18n.Message{ID: "word.compare.dialog.body.running"}, nil)
	app.applyLocaleSelection(i18n.SelectionEnglish)
	source := errors.New("failed to parse /tmp/reports/private.docx")
	app.applyWordCompareResult(wordCompareAsyncResult{
		request: 9, err: source, state: state, generation: 4,
		leftDocumentID: state.documentID, leftRevision: state.wordRevisionAtomic.Load(),
	})
	primary := app.messageText(app.modal.MessageRef, app.modal.Message)
	if primary != "The document comparison failed." || strings.Contains(primary, source.Error()) {
		t.Fatalf("English comparison error primary = %q", primary)
	}
	if app.feedback.detail != source.Error() {
		t.Fatalf("comparison diagnostic detail = %q; want %q", app.feedback.detail, source.Error())
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.messageText(app.modal.MessageRef, app.modal.Message); got != "文档比较失败。" {
		t.Fatalf("Chinese comparison error primary = %q", got)
	}
	if app.feedback.detail != source.Error() {
		t.Fatalf("locale change altered comparison diagnostic detail: %q", app.feedback.detail)
	}
}

func TestWordActionAvailabilityDescriptionUsesCurrentLocale(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	action := ribbonAction{id: "version-history", label: "版本历史"}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := app.wordToolbarActionDescription(action); got != "Version History unavailable: Save the document first." {
		t.Fatalf("English version-history description = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.wordToolbarActionDescription(action); got != "先保存文档后才能使用版本历史。" {
		t.Fatalf("Chinese version-history description = %q", got)
	}
}
