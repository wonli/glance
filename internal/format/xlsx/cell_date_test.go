package xlsx

import (
	"strconv"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

type spreadsheetDateWriter interface {
	SetCellDateChanged(sheet, cell string, date time.Time) (bool, error)
}

func TestSetCellDateChangedWritesStaticDatePreservesFormatAndUndoesOnce(t *testing.T) {
	date := time.Date(2026, time.October, 17, 0, 0, 0, 0, time.UTC)
	t.Run("general cell gets default date style and snapshot round trips", func(t *testing.T) {
		book := NewBlank()
		defer book.Close()
		writer, ok := any(book).(spreadsheetDateWriter)
		if !ok {
			t.Fatal("XLSX workbook does not implement static date insertion")
		}
		changed, err := writer.SetCellDateChanged("Sheet1", "B2", date)
		if err != nil || !changed {
			t.Fatalf("SetCellDateChanged changed=%t err=%v; want true, nil", changed, err)
		}
		window, err := book.Window("Sheet1", 2, 2, 1, 1)
		if err != nil {
			t.Fatalf("Window(B2): %v", err)
		}
		if got := window.Cells[0][0].Style.NumberFormat; got != "m/d/yy" {
			t.Fatalf("B2 date format=%q; want built-in date format m/d/yy", got)
		}
		raw, err := book.file.GetCellValue("Sheet1", "B2", excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatalf("raw date serial: %v", err)
		}
		serial, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			t.Fatalf("parse date serial %q: %v", raw, err)
		}
		reopenedDate, err := excelize.ExcelDateToTime(serial, false)
		if err != nil || reopenedDate.Format("2006-01-02") != "2026-10-17" {
			t.Fatalf("stored date serial %v reopens as %v, %v; want 2026-10-17", serial, reopenedDate, err)
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
		if got, err := reopened.file.GetCellValue("Sheet1", "B2"); err != nil || got == "" {
			t.Fatalf("saved static date value=%q err=%v", got, err)
		}
		if got, err := reopened.file.GetCellStyle("Sheet1", "B2"); err != nil || got != window.Cells[0][0].StyleID {
			t.Fatalf("saved date style=%d err=%v; want %d", got, err, window.Cells[0][0].StyleID)
		}
		if changed, err := writer.SetCellDateChanged("Sheet1", "B2", date); err != nil || changed {
			t.Fatalf("repeated date insert changed=%t err=%v; want no-op", changed, err)
		}
		if changed, err := book.Undo(); err != nil || !changed {
			t.Fatalf("Undo date insert changed=%t err=%v; want one history entry", changed, err)
		}
		if value, err := book.file.GetCellValue("Sheet1", "B2"); err != nil || value != "" {
			t.Fatalf("B2 after undo=%q err=%v; want empty", value, err)
		}
		if style, err := book.file.GetCellStyle("Sheet1", "B2"); err != nil || style != 0 {
			t.Fatalf("B2 style after undo=%d err=%v; want default style", style, err)
		}
	})

	t.Run("preformatted cell keeps its existing style", func(t *testing.T) {
		book := NewBlank()
		defer book.Close()
		dateFormat := "yyyy-mm-dd"
		bold := true
		bounds := document.CellRange{StartRow: 3, StartColumn: 4, EndRow: 3, EndColumn: 4}
		if err := book.SetCellFormats("Sheet1", bounds, document.CellFormatPatch{NumberFormat: &dateFormat, Bold: &bold}); err != nil {
			t.Fatalf("SetCellFormats(D3): %v", err)
		}
		before, err := book.file.GetCellStyle("Sheet1", "D3")
		if err != nil {
			t.Fatalf("GetCellStyle(D3): %v", err)
		}
		writer, ok := any(book).(spreadsheetDateWriter)
		if !ok {
			t.Fatal("XLSX workbook does not implement static date insertion")
		}
		if changed, err := writer.SetCellDateChanged("Sheet1", "D3", date); err != nil || !changed {
			t.Fatalf("SetCellDateChanged changed=%t err=%v; want true, nil", changed, err)
		}
		if got, err := book.file.GetCellStyle("Sheet1", "D3"); err != nil || got != before {
			t.Fatalf("D3 style after date insert=%d err=%v; want original style %d", got, err, before)
		}
		window, err := book.Window("Sheet1", 3, 4, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !window.Cells[0][0].Style.Bold || window.Cells[0][0].Style.NumberFormat != dateFormat {
			t.Fatalf("D3 style after insert=%+v; want bold and %q", window.Cells[0][0].Style, dateFormat)
		}
	})

	t.Run("same serial in General format still gets a date display format", func(t *testing.T) {
		book := NewBlank()
		defer book.Close()
		if err := book.file.SetCellValue("Sheet1", "A1", date); err != nil {
			t.Fatalf("SetCellValue(A1): %v", err)
		}
		general := "General"
		bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}
		if err := book.SetCellFormats("Sheet1", bounds, document.CellFormatPatch{NumberFormat: &general}); err != nil {
			t.Fatalf("SetCellFormats(A1, General): %v", err)
		}
		writer, ok := any(book).(spreadsheetDateWriter)
		if !ok {
			t.Fatal("XLSX workbook does not implement static date insertion")
		}
		if changed, err := writer.SetCellDateChanged("Sheet1", "A1", date); err != nil || !changed {
			t.Fatalf("SetCellDateChanged changed=%t err=%v; want date formatting applied", changed, err)
		}
		window, err := book.Window("Sheet1", 1, 1, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := window.Cells[0][0].Style.NumberFormat; got != "m/d/yy" {
			t.Fatalf("A1 date format=%q; want default date format m/d/yy", got)
		}
	})

	t.Run("1904 date system", func(t *testing.T) {
		book := NewBlank()
		defer book.Close()
		date1904 := true
		if err := book.file.SetWorkbookProps(&excelize.WorkbookPropsOptions{Date1904: &date1904}); err != nil {
			t.Fatalf("SetWorkbookProps: %v", err)
		}
		writer, ok := any(book).(spreadsheetDateWriter)
		if !ok {
			t.Fatal("XLSX workbook does not implement static date insertion")
		}
		if changed, err := writer.SetCellDateChanged("Sheet1", "A1", date); err != nil || !changed {
			t.Fatalf("SetCellDateChanged changed=%t err=%v; want true, nil", changed, err)
		}
		raw, err := book.file.GetCellValue("Sheet1", "A1", excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatalf("read raw 1904 date: %v", err)
		}
		serial, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			t.Fatalf("parse raw 1904 date %q: %v", raw, err)
		}
		got, err := excelize.ExcelDateToTime(serial, true)
		if err != nil || got.Format("2006-01-02") != "2026-10-17" {
			t.Fatalf("1904 date serial %v reopens as %v, %v; want 2026-10-17", serial, got, err)
		}
	})
}
