package desktopui

import (
	"strings"
	"testing"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/i18n"
)

func TestPDFAsyncNoticeUsesCurrentLocale(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	t.Cleanup(app.desktopRuntime.shutdown)
	view := &pdfViewState{}
	view.accept(pdfPageResult{notice: i18n.Message{ID: "pdf.status.print_sent", Args: i18n.Args{"Revision": 3}}})
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := app.text(view.messageRef); got != "Sent PDF revision r3 to the system print service" {
		t.Fatalf("English async PDF notice = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.text(view.messageRef); got != "已将 PDF 修订版本 r3 发送到系统打印服务" {
		t.Fatalf("Chinese async PDF notice = %q", got)
	}
}

func TestFilePickerLabelsUseCurrentLocale(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	t.Cleanup(app.desktopRuntime.shutdown)
	app.applyLocaleSelection(i18n.SelectionEnglish)
	english := app.filePickerLabels()
	if english.OpenDocument != "Choose an Office, PDF, JSON, Markdown or CSV document" || english.UntitledFile != "Untitled" || english.ImageFilter != "Images" {
		t.Fatalf("English file picker labels = %+v", english)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	chinese := app.filePickerLabels()
	if chinese.OpenDocument != "选择 Office、PDF、JSON、Markdown 或 CSV 文档" || chinese.UntitledFile != "未命名" || chinese.ImageFilter != "图片" {
		t.Fatalf("Chinese file picker labels = %+v", chinese)
	}
}

func TestWordNavigationGroupUsesStableLocalizedMessage(t *testing.T) {
	entries, _, _ := wordNavigationEntries(nil, nil, "", nil)
	if entries[0].labelMessageID != "word.navigation.group.headings" || entries[1].labelMessageID != "word.navigation.empty.headings" {
		t.Fatalf("empty navigation group message IDs = %q, %q", entries[0].labelMessageID, entries[1].labelMessageID)
	}
	app := &App{desktopRuntime: newDesktopRuntime()}
	t.Cleanup(app.desktopRuntime.shutdown)
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := app.textID(entries[0].labelMessageID); got != "Document headings" {
		t.Fatalf("English Word navigation group = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.textID(entries[0].labelMessageID); got != "文档标题" {
		t.Fatalf("Chinese Word navigation group = %q", got)
	}
}

func TestAgentConfirmationCopyUsesCurrentLocaleAndStableCapabilityID(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	t.Cleanup(app.desktopRuntime.shutdown)
	info := application.ConfirmationInfo{CapabilityID: "word.text.insert", Status: application.ConfirmationPending}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := app.agentLightweightPromptText(info); got != "Allow the AI to add content to the current document?" {
		t.Fatalf("English confirmation prompt = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.agentLightweightPromptText(info); got != "是否允许 AI 向当前文档添加内容？" {
		t.Fatalf("Chinese confirmation prompt = %q", got)
	}
	line := agentChatLine{confirmation: &application.ConfirmationInfo{CapabilityID: "word.text.insert", Status: application.ConfirmationPending}}
	if got := app.agentChatLineDisplayText(line); !strings.Contains(got, "等待批准") || !strings.Contains(got, "向当前文档添加内容") {
		t.Fatalf("localized confirmation chat line = %q", got)
	}
}

func TestWordFontSizeDisplayUsesCurrentLocale(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	t.Cleanup(app.desktopRuntime.shutdown)
	state := &documentSessionState{
		sharedDocumentSession: &sharedDocumentSession{wordBlocks: []document.WordBlock{{ID: "block", Kind: document.WordParagraphBlock, Paragraph: &document.WordParagraph{
			ID: "paragraph", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run", Text: "text", Style: document.WordTextStyle{FontSize: 12}, Editable: true}}},
		}}}},
		selectedWordRunID: "run",
	}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := app.localizedSelectedWordFontSizeDisplay(state); got != "12" {
		t.Fatalf("English font size display = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.localizedSelectedWordFontSizeDisplay(state); got != "小四" {
		t.Fatalf("Chinese font size display = %q", got)
	}
}
