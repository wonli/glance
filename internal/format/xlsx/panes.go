package xlsx

import (
	"fmt"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

var _ document.SpreadsheetPanesEditor = (*Workbook)(nil)

// GetSheetPanes queries the frozen pane configuration for the worksheet.
func (w *Workbook) GetSheetPanes(sheet string) (document.SpreadsheetPaneFreeze, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return document.SpreadsheetPaneFreeze{}, err
	}
	panes, err := w.file.GetPanes(sheet)
	if err != nil {
		return document.SpreadsheetPaneFreeze{}, fmt.Errorf("get panes for sheet %q: %w", sheet, err)
	}
	return document.SpreadsheetPaneFreeze{
		Frozen: panes.Freeze,
		XSplit: panes.XSplit,
		YSplit: panes.YSplit,
	}, nil
}

// SetSheetPanes configures or removes frozen panes on the worksheet.
func (w *Workbook) SetSheetPanes(sheet string, freeze document.SpreadsheetPaneFreeze) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}

	if !freeze.Frozen || (freeze.XSplit <= 0 && freeze.YSplit <= 0) {
		if err := w.file.SetPanes(sheet, &excelize.Panes{Freeze: false, Split: false}); err != nil {
			return fmt.Errorf("unfreeze panes for sheet %q: %w", sheet, err)
		}
	} else {
		topLeftCell := CellName(freeze.XSplit+1, freeze.YSplit+1)
		activePane := "bottomRight"
		if freeze.XSplit > 0 && freeze.YSplit == 0 {
			activePane = "topRight"
		} else if freeze.XSplit == 0 && freeze.YSplit > 0 {
			activePane = "bottomLeft"
		}
		panes := &excelize.Panes{
			Freeze:      true,
			Split:       false,
			XSplit:      freeze.XSplit,
			YSplit:      freeze.YSplit,
			TopLeftCell: topLeftCell,
			ActivePane:  activePane,
		}
		if err := w.file.SetPanes(sheet, panes); err != nil {
			return fmt.Errorf("set frozen panes for sheet %q: %w", sheet, err)
		}
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}
