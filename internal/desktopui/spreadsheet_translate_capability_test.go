package desktopui

import (
	"context"
	"testing"
	"time"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetTranslateRibbonUsesRegisteredBatchCellCommand(t *testing.T) {
	book := xlsx.NewBlank()
	t.Cleanup(func() { _ = book.Close() })
	sheet := book.SheetNames()[0]
	if err := book.SetCellText(sheet, "A1", "Hello World"); err != nil {
		t.Fatalf("write translation fixture: %v", err)
	}

	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	if err := app.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatalf("install workbook: %v", err)
	}
	state := app.documentSessionState
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	events := make(chan application.Event, 1)
	cancel := desktop.applicationRuntime.On(context.Background(), application.EventFilter{
		Types: []string{application.EventDocumentChanged}, DocumentID: state.documentID,
	}, func(event application.Event) { events <- event })
	t.Cleanup(cancel)

	app.showSpreadsheetTranslateDialog()
	app.modal.SecondInput.SetText("你好，世界")
	app.modal.OnAction("confirm")
	eventDeadline := time.After(3 * time.Second)
	for {
		select {
		case event := <-events:
			if event.CommandID == "" {
				continue
			}
			if event.CommandID != "spreadsheet.cells.set" || event.Source != application.OperatorHuman {
				t.Fatalf("spreadsheet translation event = %+v; want human spreadsheet.cells.set", event)
			}
			goto commandEventReceived
		case <-eventDeadline:
			t.Fatal("spreadsheet translation did not invoke the registered cell command")
		}
	}

commandEventReceived:
	completionDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(completionDeadline) {
		app.processAsyncResults()
		if !state.formatRunning.Load() {
			rows, err := book.ReadRange(sheet, document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1})
			if err != nil || len(rows) == 0 || len(rows[0]) == 0 || rows[0][0].Value != "你好，世界" {
				t.Fatalf("translated cell = %v, error=%v; want 你好，世界", rows, err)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("spreadsheet translation command did not complete")
}
