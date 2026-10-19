package desktopui

import (
	"testing"
	"time"

	"glance/internal/i18n"
)

func TestWorkspaceShellUsesCurrentCatalog(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	english := []string{
		app.text(i18n.Message{ID: "home.nav.recent"}),
		app.text(i18n.Message{ID: "home.nav.favorites"}),
		app.text(i18n.Message{ID: "home.nav.assistant"}),
		app.text(i18n.Message{ID: "home.nav.settings"}),
		app.text(i18n.Message{ID: "home.folder.title"}),
		app.text(i18n.Message{ID: "home.recent.empty"}),
	}
	if english[0] != "Recent" || english[1] != "Favorites" || english[2] != "AI assistant" || english[3] != "Settings" || english[4] != "Folders" || english[5] != "No recent files yet" {
		t.Fatalf("English home shell = %v", english)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.text(i18n.Message{ID: "home.nav.recent"}); got != "最近" {
		t.Fatalf("Chinese recent navigation label = %q", got)
	}
}

func TestHomeFeedbackUsesStableMessages(t *testing.T) {
	if got := homeRecentMenuActions[2].id; got != "copy-path" {
		t.Fatalf("copy-path command ID changed to %q", got)
	}
	message, ok := homeRecentMenuFeedbackMessage("copy-path")
	if !ok || message.ID != "home.feedback.path_copied" {
		t.Fatalf("copy-path feedback message = %+v, found=%v", message, ok)
	}
	app := &App{desktopRuntime: newDesktopRuntime()}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	app.setStatusMessage(message, feedbackSuccess, notificationToast)
	english := app.text(app.feedback.messageRef)
	app.applyLocaleSelection(i18n.SelectionChinese)
	chinese := app.text(app.feedback.messageRef)
	if english != "Path copied" || chinese != "路径已复制" {
		t.Fatalf("copy feedback did not resolve by locale: en=%q zh=%q", english, chinese)
	}
}

func TestHomeFilterLabelsResolveWithoutChangingFilterIDs(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime(), homeTypeFilter: homeFilterAll}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := app.homeFilterLabel(homeFilterDocuments); got != "Documents" {
		t.Fatalf("English documents filter = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.homeFilterLabel(homeFilterDocuments); got != "文档" {
		t.Fatalf("Chinese documents filter = %q", got)
	}
	if got := app.homeTypeFilter; got != homeFilterAll {
		t.Fatalf("filter state changed when rendering another locale: %q", got)
	}
	if !homeTypeMatches("/tmp/预算.xlsx", homeFilterSpreadsheets) {
		t.Fatal("stable spreadsheet filter ID does not match an XLSX path")
	}
}

func TestHomeSearchPlaceholderUsesCurrentCatalog(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := app.homeSearchPlaceholder(); got != "Search file names…" {
		t.Fatalf("English search placeholder = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.homeSearchPlaceholder(); got != "搜索文件名..." {
		t.Fatalf("Chinese search placeholder = %q", got)
	}
}

func TestHomeMetadataLabelsUseCurrentCatalog(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime(), homeTypeFilter: homeFilterAll}
	now := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.Local)
	app.applyLocaleSelection(i18n.SelectionEnglish)
	if got := homeTableColumnLabel(app, "name"); got != "Name" {
		t.Fatalf("English name column = %q", got)
	}
	if got := app.homeRecentCountText(); got != "Recent · 0 files" {
		t.Fatalf("English recent count = %q", got)
	}
	if got := app.formatHomeModifiedAt(now.AddDate(0, 0, -1), now); got != "Yesterday" {
		t.Fatalf("English modified date = %q", got)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := homeTableColumnLabel(app, "name"); got != "名称" {
		t.Fatalf("Chinese name column = %q", got)
	}
	if got := app.homeRecentCountText(); got != "最近  0 个文件" {
		t.Fatalf("Chinese recent count = %q", got)
	}
	if got := app.formatHomeModifiedAt(now.AddDate(0, 0, -1), now); got != "昨天" {
		t.Fatalf("Chinese modified date = %q", got)
	}
}

func TestAsyncSaveResultUsesCurrentCatalogAndKeepsPathAsData(t *testing.T) {
	app := New(nil, "")
	t.Cleanup(func() { close(app.shutdown) })
	app.applyLocaleSelection(i18n.SelectionEnglish)
	app.saveResults <- saveResult{path: "/tmp/report.xlsx"}
	app.processAsyncResults()
	if app.statusRef.ID != "document.save.status.saved" || app.statusText() != "Saved /tmp/report.xlsx" {
		t.Fatalf("English save status = %q (ref %q)", app.statusText(), app.statusRef.ID)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.statusText(); got != "已保存 /tmp/report.xlsx" {
		t.Fatalf("Chinese save status = %q", got)
	}
}
