package desktopui

import (
	"strings"
	"testing"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordToolbar_TableAndPageBorder(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	state := newDocumentSessionState("test.docx")
	state.word = doc
	state.wordBlocks = doc.WordBlocks()
	state.selectedWordParagraphID = state.wordBlocks[0].Paragraph.ID
	app := &App{documentSessionState: state}

	// 1. Verify availability
	if !app.wordToolbarActionEnabled(ribbonAction{id: "insert-table"}) {
		t.Fatal("expected insert-table to be available")
	}
	if !app.wordToolbarActionEnabled(ribbonAction{id: "design-page-border"}) {
		t.Fatal("expected design-page-border to be available")
	}

	// 2. Insert Table Dialog & Execution
	app.showWordInsertTableDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入表格" {
		t.Fatalf("expected 插入表格 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("3x3")
	if !strings.Contains(app.statusText(), "已成功插入 3 行 3 列表格") {
		t.Fatalf("unexpected status after insert table: %q", app.statusText())
	}

	// Verify table in document
	blocks := doc.WordBlocks()
	foundTable := false
	for _, block := range blocks {
		if block.Kind == document.WordTableBlock && block.Table != nil {
			foundTable = true
			if len(block.Table.Rows) != 3 {
				t.Fatalf("expected 3 rows, got %d", len(block.Table.Rows))
			}
		}
	}
	if !foundTable {
		t.Fatal("inserted table not found in document blocks")
	}

	// Verify table is immediately visible in the active session without closing/reopening
	sessionBlocks := wordBlocksForState(state)
	foundInSession := false
	for _, block := range sessionBlocks {
		if block.Kind == document.WordTableBlock && block.Table != nil {
			foundInSession = true
			if len(block.Table.Rows) != 3 {
				t.Fatalf("expected 3 rows in session blocks, got %d", len(block.Table.Rows))
			}
		}
	}
	if !foundInSession {
		t.Fatal("inserted table not immediately visible in session state word blocks")
	}

	// 3. Page Border Dialog & Execution
	app.showWordPageBorderDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "页面边框" {
		t.Fatalf("expected 页面边框 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("double")
	if !strings.Contains(app.statusText(), "已设置页面边框") {
		t.Fatalf("unexpected status after set page border: %q", app.statusText())
	}

	// 4. Clear Page Border
	app.showWordPageBorderDialog()
	app.modal.OnAction("none")
	if !strings.Contains(app.statusText(), "已清除页面边框") {
		t.Fatalf("unexpected status after clear page border: %q", app.statusText())
	}
}

func TestWordToolbar_CoverPageAndLanguage(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	state := newDocumentSessionState("test.docx")
	state.word = doc
	state.wordBlocks = doc.WordBlocks()
	state.selectedWordParagraphID = state.wordBlocks[0].Paragraph.ID
	app := &App{documentSessionState: state}

	// 1. Verify availability
	if !app.wordToolbarActionEnabled(ribbonAction{id: "cover"}) {
		t.Fatal("expected cover to be enabled")
	}
	if !app.wordToolbarActionEnabled(ribbonAction{id: "review-language"}) {
		t.Fatal("expected review-language to be enabled")
	}

	// 2. Insert Cover Page Dialog & Execution
	app.showWordCoverPageDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入封面" {
		t.Fatalf("expected 插入封面 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("项目年度规划")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.statusText(), "已成功在文档首页插入标准封面页") {
		t.Fatalf("unexpected status after insert cover page: %q", app.statusText())
	}

	// 3. Set Language Dialog & Execution
	app.showWordLanguageDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "校对语言" {
		t.Fatalf("expected 语言设置 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("en-US")
	if !strings.Contains(app.statusText(), "已将全文档的校对语言设置为 en-US") {
		t.Fatalf("unexpected status after set language: %q", app.statusText())
	}
}

func TestWordToolbar_BookmarkLinkAndTextBox(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("word-insert.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{documentSessionState: state}

	// 1. Verify actions are enabled
	for _, id := range []string{"insert-bookmark", "insert-link", "insert-textbox"} {
		if !app.wordToolbarActionEnabled(ribbonAction{id: id}) {
			t.Fatalf("expected action %q to be enabled", id)
		}
	}

	// 2. Test Insert Bookmark Dialog & Execution
	app.showWordBookmarkDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入书签" {
		t.Fatalf("expected 插入书签 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("Test_Bookmark")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.statusText(), "已成功插入书签") {
		t.Fatalf("unexpected status after insert bookmark: %q", app.statusText())
	}

	// 3. Test Insert Hyperlink Dialog & Execution
	app.showWordHyperlinkDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入超链接" {
		t.Fatalf("expected 插入超链接 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	if len(app.modal.Actions) != 2 || app.messageText(app.modal.Actions[0].LabelRef, app.modal.Actions[0].Label) != "插入" || app.messageText(app.modal.Actions[1].LabelRef, app.modal.Actions[1].Label) != "取消" {
		t.Fatalf("expected modal actions [插入, 取消], got %+v", app.modal.Actions)
	}
	app.modal.Input.SetText("https://golang.org")
	app.modal.SecondInput.SetText("Go官网")
	app.modal.OnAction("confirm")

	if !strings.Contains(app.statusText(), "已成功插入超链接") {
		t.Fatalf("unexpected status after insert link: %q", app.statusText())
	}

	// 4. Test Insert Text Box Dialog & Execution
	app.showWordTextBoxDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入文本框" {
		t.Fatalf("expected 插入文本框 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("侧边栏重要提示")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.statusText(), "已成功插入文本框") {
		t.Fatalf("unexpected status after insert textbox: %q", app.statusText())
	}
}

func TestWordToolbar_HeaderFooterPageNumberAndSideBySide(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("word-header-footer.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{documentSessionState: state}

	// 1. Verify availability
	for _, id := range []string{"insert-header", "insert-footer", "insert-page-number", "view-side-by-side"} {
		if !app.wordToolbarActionEnabled(ribbonAction{id: id}) {
			t.Fatalf("expected action %q to be enabled", id)
		}
	}

	// 2. Set Header
	app.showWordHeaderDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入页眉" {
		t.Fatalf("expected 插入页眉 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("测试页眉内容")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.statusText(), "已成功设置页眉") {
		t.Fatalf("unexpected status after set header: %q", app.statusText())
	}

	// 3. Set Footer
	app.showWordFooterDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入页脚" {
		t.Fatalf("expected 插入页脚 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("测试页脚内容")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.statusText(), "已成功设置页脚") {
		t.Fatalf("unexpected status after set footer: %q", app.statusText())
	}

	// 4. Set Page Number
	app.showWordPageNumberDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入页码" {
		t.Fatalf("expected 插入页码 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("center")
	if !strings.Contains(app.statusText(), "已成功在页脚插入页码") {
		t.Fatalf("unexpected status after set page number: %q", app.statusText())
	}

	// 5. Toggle Side by Side View
	app.toggleWordSideBySideView()
	if !strings.Contains(app.statusText(), "并排查看") {
		t.Fatalf("unexpected status after toggle side by side: %q", app.statusText())
	}
}
