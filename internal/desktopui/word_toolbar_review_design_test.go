package desktopui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glance/internal/format/docx"
)

func TestWordEditorRibbonUsesRegisteredReviewCapability(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create Word document: %v", err)
	}
	defer doc.Close()
	if _, _, err := doc.AppendBodyTextParagraph("重复，，标点（未闭合"); err != nil {
		t.Fatalf("append review fixture: %v", err)
	}
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	if err := app.installDocument(loadedDocument{word: doc}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	app.showWordEditorDialog()
	if !app.modal.Visible || !strings.Contains(app.modal.Message, "正在扫描") {
		t.Fatalf("Word Editor Ribbon should return with a loading dialog before scanning completes: visible=%v message=%q", app.modal.Visible, app.modal.Message)
	}
	waitForWordEditorReviewResult(t, app)
	if !strings.Contains(app.modal.Message, "连续重复的中文标点") || !strings.Contains(app.modal.Message, "词典拼写、语法或语义") {
		t.Fatalf("Word Editor Ribbon did not show the registered structured review result: visible=%v message=%q", app.modal.Visible, app.modal.Message)
	}
}

func waitForWordEditorReviewResult(t *testing.T, app *App) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !app.modal.Visible {
			t.Fatal("Word Editor review dialog was closed before the result arrived")
		}
		if !strings.Contains(app.modal.Message, "正在扫描") {
			return
		}
		app.processAsyncResults()
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for Word Editor review result: %q", app.modal.Message)
}

func TestWordReviewRestrictProtection(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("failed to create blank docx: %v", err)
	}
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	app := newAppWithRuntime(nil, runtime)
	if err := app.installDocument(loadedDocument{word: doc}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	state := app.documentSessionState
	state.wordToolbarTab = wordTabReview

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
	word, err := docx.Open(filepath.Join("..", "..", "testdata", "office-corpus", "docx", "floating-image-wrap-vml.docx"))
	if err != nil {
		t.Fatal(err)
	}
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	app := newAppWithRuntime(nil, runtime)
	if err := app.installDocument(loadedDocument{word: word}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	state := app.documentSessionState
	state.wordToolbarTab = wordTabReview
	for _, block := range word.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Image != nil && inline.Image.Floating != nil {
				state.selectedWordImageID = inline.Image.ID
				state.selectedWordParagraphID = block.Paragraph.ID
				break
			}
		}
		if state.selectedWordImageID != "" {
			break
		}
	}
	if state.selectedWordImageID == "" {
		t.Fatal("floating image fixture has no projected floating image")
	}
	if app.wordToolbarActionEnabled(ribbonAction{id: "layout-wrap"}) {
		t.Fatal("layout-wrap should be disabled for the read-only legacy VML floating image")
	}
	waitAndProcessWordCommand := func(operationPrefix string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case result := <-app.wordCommandResults:
				app.wordCommandResults <- result
				app.processWordCommandAsyncResult()
				if strings.HasPrefix(result.operation, operationPrefix) {
					return
				}
			default:
				time.Sleep(time.Millisecond)
			}
		}
		t.Fatalf("timed out waiting for Word command %q", operationPrefix)
	}

	// 1. Verify availability of all new actions
	actions := []string{
		"design-watermark",
		"review-track", "review-accept", "review-reject",
		"review-editor", "editor",
		"review-translate", "review-compare",
		"layout-align",
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
	waitAndProcessWordCommand("设置文档水印")

	// 3. Test Track Revisions Toggle
	if wordToolbarActionActive(state, ribbonAction{id: "review-track"}) {
		t.Fatal("expected track revisions to be inactive initially")
	}
	app.toggleWordTrackRevisions()
	waitAndProcessWordCommand("开启修订跟踪")
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
	waitAndProcessWordCommand("接受全部修订")

	// 5. Test Reject Revisions Dialog
	app.rejectWordRevisions()
	if !app.modal.Visible || app.modal.Title != "拒绝全部修订" {
		t.Fatalf("expected 拒绝全部修订 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")
	waitAndProcessWordCommand("拒绝全部修订")

	// 6. Test Document Editor Dialog
	app.showWordEditorDialog()
	if !app.modal.Visible || app.modal.Title != "文档编辑器" {
		t.Fatalf("expected 文档编辑器 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	waitForWordEditorReviewResult(t, app)
	if !strings.Contains(app.modal.Message, "启发式评分") || !strings.Contains(app.modal.Message, "词典拼写、语法或语义") {
		t.Fatalf("expected editor report to describe its heuristic scope, got: %q", app.modal.Message)
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

	// 10. Legacy VML floats are read-only, so their wrap action stays disabled.
	// Inline-image position is tested separately with an inline fixture.
	app.showWordLayoutPositionDialog()
	if app.modal.Visible {
		t.Fatalf("floating image unexpectedly opened 对象位置 modal: %q", app.modal.Title)
	}
}

func TestWordToolbarLayoutPositionUsesEditableInlineImage(t *testing.T) {
	word, err := docx.Open(filepath.Join("..", "..", "testdata", "office-corpus", "docx", "rotated-inline-image.docx"))
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	targetParagraphID, _, err := word.AppendBodyTextParagraph("移动到此处")
	if err != nil {
		t.Fatalf("append target paragraph: %v", err)
	}
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	if err := app.installDocument(loadedDocument{word: word}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	state := app.documentSessionState
	state.setWordBlocks(word.WordBlocks())
	var imageID string
	for _, paragraph := range allWordParagraphs(state.wordBlocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Image != nil && inline.Image.Editable && inline.Image.WrapMode == "inline" {
				imageID = inline.Image.ID
				break
			}
		}
		if imageID != "" {
			break
		}
	}
	if imageID == "" {
		t.Fatal("inline image fixture has no editable inline image")
	}
	state.selectedWordImageID = imageID
	state.selectedWordParagraphID = targetParagraphID
	if !app.wordToolbarActionEnabled(ribbonAction{id: "layout-position"}) {
		t.Fatal("layout-position should be enabled for an editable inline image")
	}
	app.showWordLayoutPositionDialog()
	if !app.modal.Visible || app.modal.Title != "对象位置" {
		t.Fatalf("expected 对象位置 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("cancel")
}

func TestWordToolbar_RemainingActionsBatch(t *testing.T) {
	word, err := docx.Open(filepath.Join("..", "..", "testdata", "office-corpus", "docx", "floating-image-wrap.docx"))
	if err != nil {
		t.Fatal(err)
	}
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	if err := app.installDocument(loadedDocument{word: word}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	state := app.documentSessionState
	state.setWordBlocks(word.WordBlocks())

	paragraphs := allWordParagraphs(state.wordBlocks)
	if len(paragraphs) == 0 {
		t.Fatal("expected at least one paragraph")
	}
	state.selectedWordParagraphID = paragraphs[0].ID
	if len(paragraphs[0].Inlines) > 0 && paragraphs[0].Inlines[0].Text != nil {
		state.selectedWordRunID = paragraphs[0].Inlines[0].Text.ID
	}
	waitAndProcessWordCommand := func(operationPrefix string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case result := <-app.wordCommandResults:
				app.wordCommandResults <- result
				app.processAsyncResults()
				if strings.HasPrefix(result.operation, operationPrefix) {
					if result.err != nil {
						t.Fatalf("Word command %q failed: %v", operationPrefix, result.err)
					}
					return
				}
			default:
				time.Sleep(time.Millisecond)
			}
		}
		t.Fatalf("timed out waiting for Word command %q", operationPrefix)
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
	waitAndProcessWordCommand("插入艺术字")
	if app.modal.Visible {
		t.Fatal("modal should be dismissed after style_gold")
	}

	// 2. insert-equation
	app.executeWordToolbarAction("insert-equation")
	if !app.modal.Visible || app.modal.Title != "插入公式" {
		t.Fatalf("expected 插入公式 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("pythagorean")
	waitAndProcessWordCommand("插入公式文本")

	// 3. insert-shapes
	app.executeWordToolbarAction("insert-shapes")
	if !app.modal.Visible || app.modal.Title != "插入形状" {
		t.Fatalf("expected 插入形状 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("流程框说明")
	app.modal.OnAction("flowchart")
	waitAndProcessWordCommand("插入形状")

	// 4. insert-chart
	app.executeWordToolbarAction("insert-chart")
	if !app.modal.Visible || app.modal.Title != "插入图表" {
		t.Fatalf("expected 插入图表 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("column")
	waitAndProcessWordCommand("插入图表基础数据表")

	// 5. review-new-comment
	app.executeWordToolbarAction("review-new-comment")
	if !app.modal.Visible || app.modal.Title != "新建批注" {
		t.Fatalf("expected 新建批注 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("核对数据")
	app.modal.OnAction("confirm")
	waitAndProcessWordCommand("添加批注")

	// 6. review-delete-comment
	app.executeWordToolbarAction("review-delete-comment")
	waitAndProcessWordCommand("读取 Word 批注")
	if !app.modal.Visible || app.modal.Title != "删除批注" {
		t.Fatalf("expected 删除批注 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")
	waitAndProcessWordCommand("删除批注")

	// 7. design-themes
	app.executeWordToolbarAction("design-themes")
	if !app.modal.Visible || app.modal.Title != "文档主题" {
		t.Fatalf("expected 文档主题 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("theme_business")
	waitAndProcessWordCommand("应用文档主题")

	// 8. design-colors
	app.executeWordToolbarAction("design-colors")
	if !app.modal.Visible || app.modal.Title != "主题配色" {
		t.Fatalf("expected 主题配色 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("scheme_mint")
	waitAndProcessWordCommand("应用主题配色")

	// 9. design-fonts
	app.executeWordToolbarAction("design-fonts")
	if !app.modal.Visible || app.modal.Title != "主题字体" {
		t.Fatalf("expected 主题字体 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("font_yahei")
	waitAndProcessWordCommand("设置主题字体")

	// 10. design-effects
	app.executeWordToolbarAction("design-effects")
	if !app.modal.Visible || app.modal.Title != "主题效果" {
		t.Fatalf("expected 主题效果 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("moderate")
	waitAndProcessWordCommand("设置主题效果")

	// 11. designer
	app.executeWordToolbarAction("designer")
	if !app.modal.Visible || app.modal.Title != "设计器" {
		t.Fatalf("expected 设计器 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("scheme_gov")
	waitAndProcessWordCommand("设计器应用版式")

	// 12. dictate
	app.executeWordToolbarAction("dictate")
	if !app.modal.Visible || app.modal.Title != "语音听写" {
		t.Fatalf("expected 语音听写 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("语音输入的一段话")
	app.modal.OnAction("confirm")
	waitAndProcessWordCommand("语音听写录入")

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
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && strings.Contains(app.modal.Message, "正在检查") {
		app.processAsyncResults()
		time.Sleep(time.Millisecond)
	}
	if strings.Contains(app.modal.Message, "正在检查") {
		t.Fatalf("timed out waiting for macro inspection: %q", app.modal.Message)
	}
	app.modal.OnAction("enable_alert")
}
