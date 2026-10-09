package desktopui

import (
	"strings"
	"testing"
	"time"

	"glance/internal/format/docx"
)

func TestWordReviewRestrictProtection(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("failed to create blank docx: %v", err)
	}
	defer doc.Close()

	state := newDocumentSessionState("test.docx")
	state.word = doc
	state.wordBlocks = cloneWordBlockSnapshot(doc.WordBlocks())
	state.wordToolbarTab = wordTabReview

	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 4),
	}

	var restrictAction ribbonAction
	for _, a := range wordToolbarActions(wordTabReview) {
		if a.id == "review-restrict" {
			restrictAction = a
			break
		}
	}
	if restrictAction.id == "" {
		t.Fatal("review-restrict action not found in wordTabReview")
	}

	if !app.wordToolbarActionEnabled(restrictAction) {
		t.Fatal("review-restrict should be enabled")
	}
	if wordToolbarActionActive(state, restrictAction) {
		t.Fatal("review-restrict should initially be inactive")
	}

	// 1. Show protection dialog -> Click "protect"
	app.showWordDocumentProtectionDialog()
	if !app.modal.Visible || app.modal.Title != "限制编辑" {
		t.Fatalf("expected modal dialog for review-restrict, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}

	app.modal.OnAction("protect")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("protect command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for protect command result")
	}

	// State should now be protected
	if !state.wordProtected {
		t.Fatal("expected state.wordProtected to be true")
	}
	if !wordToolbarActionActive(state, restrictAction) {
		t.Fatal("review-restrict should now be active")
	}
	prot, err := doc.GetWordDocumentProtection()
	if err != nil || !prot {
		t.Fatalf("expected doc to be protected, got %v (err=%v)", prot, err)
	}

	// 2. Show unprotect dialog -> Click "unprotect"
	app.showWordDocumentProtectionDialog()
	if !app.modal.Visible || app.modal.Title != "限制编辑" {
		t.Fatalf("expected modal dialog for unprotect, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}

	app.modal.OnAction("unprotect")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("unprotect command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for unprotect command result")
	}

	if state.wordProtected {
		t.Fatal("expected state.wordProtected to be false after unprotect")
	}
	if wordToolbarActionActive(state, restrictAction) {
		t.Fatal("review-restrict should be inactive after unprotect")
	}
	prot, err = doc.GetWordDocumentProtection()
	if err != nil || prot {
		t.Fatalf("expected doc to be unprotected, got %v (err=%v)", prot, err)
	}
}

func TestWordToolbar_WatermarkTrackRevisionsEditorAndLayout(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("word-review-design-layout.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{documentSessionState: state}

	// 1. Verify availability of all new actions
	actions := []string{
		"design-watermark",
		"review-track", "review-accept", "review-reject",
		"review-editor", "editor",
		"review-translate", "review-compare",
		"layout-position", "layout-wrap", "layout-align",
	}
	for _, id := range actions {
		if !app.wordToolbarActionEnabled(ribbonAction{id: id}) {
			t.Fatalf("expected action %q to be enabled", id)
		}
	}

	// 2. Test Watermark Dialog
	app.showWordWatermarkDialog()
	if !app.modal.Visible || app.modal.Title != "文档水印" {
		t.Fatalf("expected 文档水印 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("绝密文档")
	app.modal.OnAction("set")
	// Give background goroutine a moment to complete
	time.Sleep(50 * time.Millisecond)

	// 3. Test Track Revisions Toggle
	if wordToolbarActionActive(state, ribbonAction{id: "review-track"}) {
		t.Fatal("expected track revisions to be inactive initially")
	}
	app.toggleWordTrackRevisions()
	time.Sleep(50 * time.Millisecond)
	if !state.wordTrackRevisions {
		t.Fatal("expected state.wordTrackRevisions to be true after toggle")
	}
	if !wordToolbarActionActive(state, ribbonAction{id: "review-track"}) {
		t.Fatal("expected track revisions action to be active after toggle")
	}

	// 4. Test Accept Revisions Dialog
	app.acceptWordRevisions()
	if !app.modal.Visible || app.modal.Title != "接受全部修订" {
		t.Fatalf("expected 接受全部修订 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")
	time.Sleep(50 * time.Millisecond)

	// 5. Test Reject Revisions Dialog
	app.rejectWordRevisions()
	if !app.modal.Visible || app.modal.Title != "拒绝全部修订" {
		t.Fatalf("expected 拒绝全部修订 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")
	time.Sleep(50 * time.Millisecond)

	// 6. Test Document Editor Dialog
	app.showWordEditorDialog()
	if !app.modal.Visible || app.modal.Title != "文档编辑器" {
		t.Fatalf("expected 文档编辑器 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	if !strings.Contains(app.modal.Message, "综合评分") {
		t.Fatalf("expected editor report to mention 综合评分, got: %q", app.modal.Message)
	}
	app.modal.OnAction("close")

	// 7. Test Document Translate Dialog
	app.showWordTranslateDialog()
	if !app.modal.Visible || app.modal.Title != "翻译" {
		t.Fatalf("expected 翻译 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("Translated Text")
	app.modal.OnAction("confirm")
	time.Sleep(50 * time.Millisecond)

	// 8. Test Document Compare Dialog
	app.showWordCompareDialog()
	if !app.modal.Visible || app.modal.Title != "文档比较" {
		t.Fatalf("expected 文档比较 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	if !strings.Contains(app.modal.Message, "版本") {
		t.Fatalf("expected compare dialog to mention 版本, got: %q", app.modal.Message)
	}
	app.modal.OnAction("close")

	// 9. Test Layout Align Dialog
	app.showWordLayoutAlignDialog()
	if !app.modal.Visible || app.modal.Title != "排列对齐" {
		t.Fatalf("expected 排列对齐 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("center")
	app.modal.OnAction("confirm")
	time.Sleep(50 * time.Millisecond)

	// 10. Test Layout Wrap Dialog
	app.showWordLayoutWrapDialog()
	if !app.modal.Visible || app.modal.Title != "文字环绕" {
		t.Fatalf("expected 文字环绕 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("close")

	// 11. Test Layout Position Dialog
	app.showWordLayoutPositionDialog()
	if !app.modal.Visible || app.modal.Title != "对象位置" {
		t.Fatalf("expected 对象位置 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("close")
}

func TestWordToolbar_RemainingActionsBatch(t *testing.T) {
	state := newDocumentSessionState("batch_test.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	state.setWordBlocks(word.WordBlocks())

	paragraphs := allWordParagraphs(state.wordBlocks)
	if len(paragraphs) == 0 {
		t.Fatal("expected at least one paragraph")
	}
	state.selectedWordParagraphID = paragraphs[0].ID
	if len(paragraphs[0].Inlines) > 0 && paragraphs[0].Inlines[0].Text != nil {
		state.selectedWordRunID = paragraphs[0].Inlines[0].Text.ID
	}

	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 1),
	}

	actions := []string{
		"insert-wordart",
		"insert-equation",
		"insert-shapes",
		"insert-chart",
		"review-new-comment",
		"review-delete-comment",
		"design-themes",
		"design-colors",
		"design-fonts",
		"design-effects",
		"designer",
		"dictate",
		"add-ins",
		"view-macros",
	}

	for _, id := range actions {
		if !app.wordToolbarActionEnabled(ribbonAction{id: id}) {
			t.Fatalf("expected action %q to be enabled", id)
		}
	}

	// 1. insert-wordart
	app.executeWordToolbarAction("insert-wordart")
	if !app.modal.Visible || app.modal.Title != "插入艺术字" {
		t.Fatalf("expected 插入艺术字 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("测试艺术字")
	app.modal.OnAction("style_gold")
	if app.modal.Visible {
		t.Fatal("modal should be dismissed after style_gold")
	}

	// 2. insert-equation
	app.executeWordToolbarAction("insert-equation")
	if !app.modal.Visible || app.modal.Title != "插入公式" {
		t.Fatalf("expected 插入公式 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("pythagorean")
	select {
	case res := <-app.wordCommandResults:
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(time.Second):
	}

	// 3. insert-shapes
	app.executeWordToolbarAction("insert-shapes")
	if !app.modal.Visible || app.modal.Title != "插入形状" {
		t.Fatalf("expected 插入形状 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("流程框说明")
	app.modal.OnAction("flowchart")

	// 4. insert-chart
	app.executeWordToolbarAction("insert-chart")
	if !app.modal.Visible || app.modal.Title != "插入图表" {
		t.Fatalf("expected 插入图表 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("column")

	// 5. review-new-comment
	app.executeWordToolbarAction("review-new-comment")
	if !app.modal.Visible || app.modal.Title != "新建批注" {
		t.Fatalf("expected 新建批注 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("核对数据")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.wordCommandResults:
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(time.Second):
	}

	// 6. review-delete-comment
	app.executeWordToolbarAction("review-delete-comment")
	if !app.modal.Visible || app.modal.Title != "删除批注" {
		t.Fatalf("expected 删除批注 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")
	select {
	case res := <-app.wordCommandResults:
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(time.Second):
	}

	// 7. design-themes
	app.executeWordToolbarAction("design-themes")
	if !app.modal.Visible || app.modal.Title != "文档主题" {
		t.Fatalf("expected 文档主题 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("theme_business")
	select {
	case res := <-app.wordCommandResults:
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(time.Second):
	}

	// 8. design-colors
	app.executeWordToolbarAction("design-colors")
	if !app.modal.Visible || app.modal.Title != "主题配色" {
		t.Fatalf("expected 主题配色 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("scheme_mint")

	// 9. design-fonts
	app.executeWordToolbarAction("design-fonts")
	if !app.modal.Visible || app.modal.Title != "主题字体" {
		t.Fatalf("expected 主题字体 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("font_yahei")

	// 10. design-effects
	app.executeWordToolbarAction("design-effects")
	if !app.modal.Visible || app.modal.Title != "主题效果" {
		t.Fatalf("expected 主题效果 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("moderate")

	// 11. designer
	app.executeWordToolbarAction("designer")
	if !app.modal.Visible || app.modal.Title != "设计器" {
		t.Fatalf("expected 设计器 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("scheme_gov")
	select {
	case res := <-app.wordCommandResults:
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(time.Second):
	}

	// 12. dictate
	app.executeWordToolbarAction("dictate")
	if !app.modal.Visible || app.modal.Title != "语音听写" {
		t.Fatalf("expected 语音听写 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("语音输入的一段话")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.wordCommandResults:
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(time.Second):
	}

	// 13. add-ins
	app.executeWordToolbarAction("add-ins")
	if !app.modal.Visible || app.modal.Title != "加载项与扩展" {
		t.Fatalf("expected 加载项与扩展 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("close")

	// 14. view-macros
	app.executeWordToolbarAction("view-macros")
	if !app.modal.Visible || app.modal.Title != "宏与安全性" {
		t.Fatalf("expected 宏与安全性 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("enable_alert")
}
