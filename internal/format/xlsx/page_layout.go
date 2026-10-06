package xlsx

import (
	"fmt"
	"strings"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

// GetSheetPageLayout reads the current page setup for a worksheet.
func (w *Workbook) GetSheetPageLayout(sheet string) (document.SpreadsheetPageLayout, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return document.SpreadsheetPageLayout{}, err
	}
	opts, err := w.file.GetPageLayout(sheet)
	if err != nil {
		return document.SpreadsheetPageLayout{}, fmt.Errorf("get page layout for sheet %q: %w", sheet, err)
	}
	margins, err := w.file.GetPageMargins(sheet)
	if err != nil {
		return document.SpreadsheetPageLayout{}, fmt.Errorf("get page margins for sheet %q: %w", sheet, err)
	}

	result := document.SpreadsheetPageLayout{
		Orientation:  "portrait",
		PaperSize:    9, // A4 default
		MarginTop:    0.75,
		MarginRight:  0.7,
		MarginBottom: 0.75,
		MarginLeft:   0.7,
	}
	if opts.Orientation != nil && *opts.Orientation != "" {
		result.Orientation = strings.ToLower(*opts.Orientation)
	}
	if opts.Size != nil && *opts.Size > 0 {
		result.PaperSize = *opts.Size
	}
	if margins.Top != nil {
		result.MarginTop = *margins.Top
	}
	if margins.Right != nil {
		result.MarginRight = *margins.Right
	}
	if margins.Bottom != nil {
		result.MarginBottom = *margins.Bottom
	}
	if margins.Left != nil {
		result.MarginLeft = *margins.Left
	}
	return result, nil
}

// SetSheetPageOrientation updates the orientation (portrait or landscape) for a worksheet.
func (w *Workbook) SetSheetPageOrientation(sheet string, orientation string) error {
	orient := strings.ToLower(strings.TrimSpace(orientation))
	if orient != "portrait" && orient != "landscape" {
		return fmt.Errorf("unsupported orientation %q: must be portrait or landscape", orientation)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	if err := w.file.SetPageLayout(sheet, &excelize.PageLayoutOptions{Orientation: &orient}); err != nil {
		return fmt.Errorf("set page orientation for sheet %q: %w", sheet, err)
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

// SetSheetPageMargins updates the print margins (in inches) for a worksheet.
func (w *Workbook) SetSheetPageMargins(sheet string, top, right, bottom, left float64) error {
	if top < 0 || right < 0 || bottom < 0 || left < 0 {
		return fmt.Errorf("margins must be non-negative inches")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	opts := &excelize.PageLayoutMarginsOptions{
		Top:    &top,
		Right:  &right,
		Bottom: &bottom,
		Left:   &left,
	}
	if err := w.file.SetPageMargins(sheet, opts); err != nil {
		return fmt.Errorf("set page margins for sheet %q: %w", sheet, err)
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

// SetSheetPageSize updates the paper size index for a worksheet (e.g. 1=Letter, 9=A4, 8=A3).
func (w *Workbook) SetSheetPageSize(sheet string, paperSize int) error {
	if paperSize <= 0 {
		return fmt.Errorf("paper size must be positive")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	if err := w.file.SetPageLayout(sheet, &excelize.PageLayoutOptions{Size: &paperSize}); err != nil {
		return fmt.Errorf("set page size for sheet %q: %w", sheet, err)
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

var _ document.SpreadsheetPageLayoutEditor = (*Workbook)(nil)
