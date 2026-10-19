package desktopui

import (
	"path/filepath"
	"strings"

	"testing"
	"time"

	"glance/internal/format/docx"
	"glance/internal/i18n"
	"glance/internal/workspace"
)

func TestWordVersionHistoryActionEnabledOnlyForSavedDOCX(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saved.docx")
	writeWordFile(t, path, "saved document")
	app := newAppWithRuntime(nil, newDesktopRuntime())
	if err := app.open(path); err != nil {
		t.Fatal(err)
	}
	action := ribbonAction{id: "version-history"}
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("version history should be enabled for a saved DOCX")
	}
	app.documentSessionState.path = ""
	if app.wordToolbarActionEnabled(action) {
		t.Fatal("version history should be disabled for an unsaved DOCX")
	}
}

func TestWordVersionHistorySemanticDescriptionRequiresSave(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	app.applyLocaleSelection(i18n.SelectionChinese)
	if err := app.installDocument(loadedDocument{word: func() *docx.Document {
		doc, err := docx.NewBlank()
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}()}); err != nil {
		t.Fatal(err)
	}
	description := app.wordToolbarActionDescription(ribbonAction{id: "version-history", label: "版本历史"})
	if !strings.Contains(description, "先保存文档") {
		t.Fatalf("unsaved history action description = %q; want save guidance", description)
	}
}

func TestWordVersionHistoryListShowsMetadataAndOrder(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	app.applyLocaleSelection(i18n.SelectionEnglish)
	store := openWordHistoryTestStore(t)
	pathKey := filepath.Join(t.TempDir(), "history.docx")
	firstAt := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Hour)
	for _, item := range []struct {
		at      time.Time
		source  workspace.WordVersionSource
		payload []byte
	}{
		{at: firstAt, source: workspace.WordVersionOpened, payload: []byte("baseline bytes")},
		{at: secondAt, source: workspace.WordVersionManualSave, payload: []byte("manual bytes, larger")},
	} {
		if _, _, err := store.AddWordVersion(workspace.WordVersionInput{
			PathKey: pathKey, DisplayPath: pathKey, Source: item.source,
			CapturedAt: item.at, Snapshot: item.payload,
		}); err != nil {
			t.Fatal(err)
		}
	}
	versions, err := store.ListWordVersions(pathKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || !versions[0].CreatedAt.After(versions[1].CreatedAt) {
		t.Fatalf("versions not newest-first: %+v", versions)
	}
	labels := []string{app.wordVersionHistoryRowText(versions[0]), app.wordVersionHistoryRowText(versions[1])}
	if !strings.Contains(labels[0], "Manual save") || !strings.Contains(labels[0], "2026") || !strings.Contains(labels[0], "bytes") {
		t.Fatalf("newest row omits source/time/size: %q", labels[0])
	}
	if !strings.Contains(labels[1], "Opened document") {
		t.Fatalf("older row has unexpected metadata: %q", labels[1])
	}
}

func TestWordVersionHistoryLoadFailureIsVisible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history-error.docx")
	writeWordFile(t, path, "document")
	app := newAppWithRuntime(nil, newDesktopRuntime())
	if err := app.open(path); err != nil {
		t.Fatal(err)
	}
	app.openWordVersionHistory()
	select {
	case result := <-app.wordVersionHistoryResults:
		app.wordVersionHistoryResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for history list failure")
	}
	if !app.wordVersionHistoryVisible || app.wordVersionHistoryLoading || app.wordVersionHistoryError == "" {
		t.Fatalf("history failure is not visible: visible=%v loading=%v error=%q", app.wordVersionHistoryVisible, app.wordVersionHistoryLoading, app.wordVersionHistoryError)
	}
}
