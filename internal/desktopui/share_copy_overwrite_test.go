package desktopui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glance/internal/format/xlsx"
)

func TestShareCopyRejectsExtensionlessSourcePath(t *testing.T) {
	t.Run("XLSX", func(t *testing.T) {
		sourcePath := filepath.Join(t.TempDir(), "source.xlsx")
		book := xlsx.NewBlank()
		defer book.Close()
		if err := book.SaveAsDocument(sourcePath); err != nil {
			t.Fatal(err)
		}
		state := newDocumentSessionState(sourcePath)
		state.book = book
		results := make(chan spreadsheetShareCopyResult, 1)
		app := &App{
			documentSessionState:        state,
			spreadsheetShareCopyResults: results,
			shutdown:                    make(chan struct{}),
		}

		app.startSpreadsheetShareCopyFile(
			&spreadsheetShareCopyPickerRequest{state: state, historyState: book.HistoryState()},
			strings.TrimSuffix(sourcePath, ".xlsx"),
		)
		want := "共享副本不能覆盖当前 XLSX"
		if app.status != want {
			select {
			case result := <-results:
				app.applySpreadsheetShareCopyResult(result)
			case <-time.After(3 * time.Second):
				t.Fatal("unexpected XLSX share-copy operation did not finish")
			}
			t.Fatalf("extensionless source selection status = %q, want %q", app.status, want)
		}
		if state.formatRunning.Load() {
			t.Fatal("extensionless source selection started an XLSX export")
		}
		if _, err := os.Stat(sourcePath); err != nil {
			t.Fatalf("source XLSX disappeared after rejection: %v", err)
		}
	})

	t.Run("DOCX", func(t *testing.T) {
		sourcePath := filepath.Join(t.TempDir(), "source.docx")
		word := newWordShareFixture(t)
		if err := word.SaveAsDocument(sourcePath); err != nil {
			t.Fatal(err)
		}
		state := newDocumentSessionState(sourcePath)
		state.word = word
		state.wordRevisionAtomic.Store(4)
		app := newWordShareTestApp(state)

		app.startWordShareCopyFile(
			&wordShareCopyPickerRequest{state: state, revision: 4, draftEpoch: state.wordDraftEpoch.Load()},
			strings.TrimSuffix(sourcePath, ".docx"),
		)
		want := "共享副本不能覆盖当前 DOCX"
		if app.status != want {
			waitWordShareResult(t, app)
			t.Fatalf("extensionless source selection status = %q, want %q", app.status, want)
		}
		if state.formatRunning.Load() {
			t.Fatal("extensionless source selection started a DOCX export")
		}
		if _, err := os.Stat(sourcePath); err != nil {
			t.Fatalf("source DOCX disappeared after rejection: %v", err)
		}
	})
}
