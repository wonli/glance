package xlsx

import (
	"fmt"
	"strings"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

var _ document.SpreadsheetViewsEditor = (*Workbook)(nil)

// GetSheetView retrieves view settings for the specified worksheet.
func (w *Workbook) GetSheetView(sheet string) (document.SpreadsheetSheetView, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return document.SpreadsheetSheetView{}, err
	}
	opts, err := w.file.GetSheetView(sheet, 0)
	if err != nil {
		return document.SpreadsheetSheetView{ViewMode: "normal"}, nil
	}
	mode := "normal"
	if opts.View != nil && *opts.View != "" {
		mode = *opts.View
	}
	showRuler := false
	if opts.ShowRuler != nil {
		showRuler = *opts.ShowRuler
	}
	return document.SpreadsheetSheetView{
		ViewMode:  mode,
		ShowRuler: showRuler,
	}, nil
}

// SetSheetViewMode sets the display mode ("normal", "pageBreakPreview", "pageLayout") for the sheet.
func (w *Workbook) SetSheetViewMode(sheet string, mode string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	viewMode := strings.TrimSpace(mode)
	if viewMode == "" {
		viewMode = "normal"
	}
	if err := w.file.SetSheetView(sheet, 0, &excelize.ViewOptions{View: &viewMode}); err != nil {
		return fmt.Errorf("set sheet view mode for sheet %q: %w", sheet, err)
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	w.dirty = true
	return nil
}

// SetSheetRuler toggles ruler visibility for the sheet.
func (w *Workbook) SetSheetRuler(sheet string, show bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	if err := w.file.SetSheetView(sheet, 0, &excelize.ViewOptions{ShowRuler: &show}); err != nil {
		return fmt.Errorf("set sheet ruler for sheet %q: %w", sheet, err)
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	w.dirty = true
	return nil
}
