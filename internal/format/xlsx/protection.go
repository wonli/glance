package xlsx

import (
	"fmt"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

var _ document.SpreadsheetProtectionEditor = (*Workbook)(nil)

// GetSheetProtection reports whether the specified sheet is protected.
func (w *Workbook) GetSheetProtection(sheet string) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	opts, err := w.file.GetSheetProtection(sheet)
	if err != nil {
		return false, fmt.Errorf("get sheet protection for %q: %w", sheet, err)
	}
	// When a sheet is protected, SelectLockedCells, SelectUnlockedCells, or AlgorithmName is set.
	// In unprotected sheets, all fields in SheetProtectionOptions are empty/false.
	isProtected := opts.SelectLockedCells || opts.SelectUnlockedCells || opts.AlgorithmName != ""
	return isProtected, nil
}

// ProtectSheet enables worksheet protection with an optional password.
func (w *Workbook) ProtectSheet(sheet string, password string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	opts := excelize.SheetProtectionOptions{
		SelectLockedCells:   true,
		SelectUnlockedCells: true,
	}
	if password != "" {
		opts.AlgorithmName = "SHA-512"
		opts.Password = password
	}
	if err := w.file.ProtectSheet(sheet, &opts); err != nil {
		return fmt.Errorf("protect sheet %q: %w", sheet, err)
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	w.dirty = true
	return nil
}

// UnprotectSheet removes worksheet protection with an optional password.
func (w *Workbook) UnprotectSheet(sheet string, password string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	var errUnprotect error
	if password != "" {
		errUnprotect = w.file.UnprotectSheet(sheet, password)
	} else {
		errUnprotect = w.file.UnprotectSheet(sheet)
	}
	if errUnprotect != nil {
		return fmt.Errorf("unprotect sheet %q: %w", sheet, errUnprotect)
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	w.dirty = true
	return nil
}
