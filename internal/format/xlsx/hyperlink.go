package xlsx

import (
	"fmt"
	"strings"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

var _ document.SpreadsheetHyperlinkEditor = (*Workbook)(nil)

// GetCellHyperlink gets the hyperlink attached to the specified cell, if any.
func (w *Workbook) GetCellHyperlink(sheet, cell string) (document.CellHyperlink, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return document.CellHyperlink{}, false, err
	}
	hasLink, target, err := w.file.GetCellHyperLink(sheet, cell)
	if err != nil {
		return document.CellHyperlink{}, false, fmt.Errorf("get hyperlink for %s!%s: %w", sheet, cell, err)
	}
	if !hasLink {
		return document.CellHyperlink{}, false, nil
	}
	return document.CellHyperlink{Target: target}, true, nil
}

// SetCellHyperlink attaches or updates an external or location hyperlink on the specified cell.
func (w *Workbook) SetCellHyperlink(sheet, cell, target, tooltip string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	cleanTarget := strings.TrimSpace(target)
	linkType := "External"
	if strings.Contains(cleanTarget, "!") && !strings.Contains(cleanTarget, "://") {
		linkType = "Location"
	}
	var opts []excelize.HyperlinkOpts
	cleanTooltip := strings.TrimSpace(tooltip)
	if cleanTooltip != "" {
		opts = append(opts, excelize.HyperlinkOpts{
			Tooltip: &cleanTooltip,
		})
	}
	if val, _ := w.file.GetCellValue(sheet, cell); val == "" {
		_ = w.file.SetCellValue(sheet, cell, cleanTarget)
	}
	if err := w.file.SetCellHyperLink(sheet, cell, cleanTarget, linkType, opts...); err != nil {
		return fmt.Errorf("set hyperlink for %s!%s: %w", sheet, cell, err)
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

// RemoveCellHyperlink deletes the hyperlink attached to the specified cell.
func (w *Workbook) RemoveCellHyperlink(sheet, cell string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	if err := w.file.SetCellHyperLink(sheet, cell, "", "None"); err != nil {
		return fmt.Errorf("remove hyperlink for %s!%s: %w", sheet, cell, err)
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}
