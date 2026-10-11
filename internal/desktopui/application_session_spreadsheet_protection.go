package desktopui

import (
	"context"

	"glance/internal/application"
	"glance/internal/document"
)

func (a *desktopSessionAdapter) GetSpreadsheetProtection(ctx context.Context, scope, sheet string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return false, application.ErrCapabilityUnsupported
	}
	switch scope {
	case "sheet":
		reader, ok := any(s.book).(document.SpreadsheetProtectionEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		return reader.GetSheetProtection(sheet)
	case "workbook":
		reader, ok := any(s.book).(document.SpreadsheetWorkbookProtectionEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		return reader.GetWorkbookProtection()
	default:
		return false, application.ErrInvalidCommandArguments
	}
}

func (a *desktopSessionAdapter) SetSpreadsheetProtection(ctx context.Context, scope, sheet string, enabled bool, password string) (bool, error) {
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
	if err := ctx.Err(); err != nil {
		return false, err
	}
	switch scope {
	case "sheet":
		editor, ok := any(s.book).(document.SpreadsheetProtectionEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		current, err := editor.GetSheetProtection(sheet)
		if err != nil || current == enabled {
			return false, err
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if enabled {
			err = editor.ProtectSheet(sheet, password)
		} else {
			err = editor.UnprotectSheet(sheet, password)
		}
		return err == nil, err
	case "workbook":
		editor, ok := any(s.book).(document.SpreadsheetWorkbookProtectionEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		current, err := editor.GetWorkbookProtection()
		if err != nil || current == enabled {
			return false, err
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if enabled {
			err = editor.ProtectWorkbook(password)
		} else {
			err = editor.UnprotectWorkbook(password)
		}
		return err == nil, err
	default:
		return false, application.ErrInvalidCommandArguments
	}
}
