package xlsx

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

type spreadsheetTimeWriter interface {
	SetCellTimeChanged(sheet, cell string, value time.Time) (bool, error)
}

func TestSetCellTimeChangedWritesStaticTimePreservesStyleAndUndoesOnce(t *testing.T) {
	value := time.Date(2026, time.October, 17, 13, 47, 59, 0, time.Local)
	wantSerial := float64(13*60+47) / float64(24*60)
	book := NewBlank()
	defer book.Close()
	writer, ok := any(book).(spreadsheetTimeWriter)
	if !ok {
		t.Fatal("XLSX workbook does not implement static time insertion")
	}
	changed, err := writer.SetCellTimeChanged("Sheet1", "C3", value)
	if err != nil || !changed {
		t.Fatalf("SetCellTimeChanged changed=%t err=%v; want true, nil", changed, err)
	}
	window, err := book.Window("Sheet1", 3, 3, 1, 1)
	if err != nil {
		t.Fatalf("Window(C3): %v", err)
	}
	if got := window.Cells[0][0].Style.NumberFormat; got != "h:mm" {
		t.Fatalf("C3 default time format=%q; want built-in h:mm", got)
	}
	if got := window.Cells[0][0].Value; got != "13:47" {
		t.Fatalf("C3 displayed time=%q; want minute-precision 13:47", got)
	}
	raw, err := book.file.GetCellValue("Sheet1", "C3", excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatalf("raw time serial: %v", err)
	}
	serial, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.Abs(serial-wantSerial) > 1e-12 {
		t.Fatalf("C3 serial=%q parsed=%v err=%v; want %v", raw, serial, err, wantSerial)
	}
	snapshot, err := book.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	reopened, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatalf("OpenSnapshot: %v", err)
	}
	defer reopened.Close()
	reopenedWindow, err := reopened.Window("Sheet1", 3, 3, 1, 1)
	if err != nil || reopenedWindow.Cells[0][0].Value != "13:47" || reopenedWindow.Cells[0][0].Style.NumberFormat != "h:mm" {
		t.Fatalf("reopened C3=%+v err=%v; want static 13:47 with h:mm", reopenedWindow.Cells[0][0], err)
	}
	if changed, err := writer.SetCellTimeChanged("Sheet1", "C3", value); err != nil || changed {
		t.Fatalf("repeated time insert changed=%t err=%v; want no-op", changed, err)
	}
	if changed, err := book.Undo(); err != nil || !changed {
		t.Fatalf("Undo time insert changed=%t err=%v; want one history entry", changed, err)
	}
	if raw, err := book.file.GetCellValue("Sheet1", "C3", excelize.Options{RawCellValue: true}); err != nil || raw != "" {
		t.Fatalf("C3 after undo raw=%q err=%v; want empty", raw, err)
	}

	t.Run("existing style is preserved", func(t *testing.T) {
		styled := NewBlank()
		defer styled.Close()
		numberFormat, bold := "yyyy-mm-dd", true
		bounds := document.CellRange{StartRow: 2, StartColumn: 4, EndRow: 2, EndColumn: 4}
		if err := styled.SetCellFormats("Sheet1", bounds, document.CellFormatPatch{NumberFormat: &numberFormat, Bold: &bold}); err != nil {
			t.Fatalf("SetCellFormats(D2): %v", err)
		}
		before, err := styled.file.GetCellStyle("Sheet1", "D2")
		if err != nil {
			t.Fatal(err)
		}
		styleWriter := any(styled).(spreadsheetTimeWriter)
		if changed, err := styleWriter.SetCellTimeChanged("Sheet1", "D2", value); err != nil || !changed {
			t.Fatalf("SetCellTimeChanged changed=%t err=%v; want true, nil", changed, err)
		}
		if got, err := styled.file.GetCellStyle("Sheet1", "D2"); err != nil || got != before {
			t.Fatalf("D2 style after time insert=%d err=%v; want original %d", got, err, before)
		}
		window, err := styled.Window("Sheet1", 2, 4, 1, 1)
		if err != nil || !window.Cells[0][0].Style.Bold || window.Cells[0][0].Style.NumberFormat != numberFormat {
			t.Fatalf("D2 style after time insert=%+v err=%v; want bold and %q", window.Cells[0][0].Style, err, numberFormat)
		}
	})

	t.Run("same serial in General format still gets a time display format", func(t *testing.T) {
		general := NewBlank()
		defer general.Close()
		if err := general.file.SetCellValue("Sheet1", "A1", wantSerial); err != nil {
			t.Fatalf("SetCellValue(A1): %v", err)
		}
		generalWriter := any(general).(spreadsheetTimeWriter)
		if changed, err := generalWriter.SetCellTimeChanged("Sheet1", "A1", value); err != nil || !changed {
			t.Fatalf("SetCellTimeChanged changed=%t err=%v; want time format applied", changed, err)
		}
		window, err := general.Window("Sheet1", 1, 1, 1, 1)
		if err != nil || window.Cells[0][0].Style.NumberFormat != "h:mm" {
			t.Fatalf("A1 format=%+v err=%v; want h:mm", window.Cells[0][0].Style, err)
		}
	})
}
