package desktopui

import (
	"context"
	"glance/internal/application"
	"glance/internal/document"
)

func (a *desktopSessionAdapter) ClearSpreadsheetCellRange(ctx context.Context, sheet string, bounds document.CellRange) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil || !s.book.Editable() {
		return false, application.ErrReadOnlyDocument
	}
	editor, ok := any(s.book).(document.SpreadsheetRangeContentEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return editor.ClearCellRange(sheet, bounds)
}
