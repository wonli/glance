package xlsx

import (
	"fmt"
	"reflect"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

var (
	_ document.SpreadsheetProtectionEditor         = (*Workbook)(nil)
	_ document.SpreadsheetWorkbookProtectionEditor = (*Workbook)(nil)
)

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

// GetWorkbookProtection reports whether the workbook structure is protected.
func (w *Workbook) GetWorkbookProtection() (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	val := reflect.ValueOf(w.file)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	wbField := val.FieldByName("WorkBook")
	if !wbField.IsValid() || wbField.IsNil() {
		return false, nil
	}
	protField := wbField.Elem().FieldByName("WorkbookProtection")
	if !protField.IsValid() || protField.IsNil() {
		return false, nil
	}
	return true, nil
}

// ProtectWorkbook enables workbook structure protection with an optional password.
func (w *Workbook) ProtectWorkbook(password string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	opts := excelize.WorkbookProtectionOptions{
		LockStructure: true,
	}
	if password != "" {
		opts.AlgorithmName = "SHA-512"
		opts.Password = password
	}
	if err := w.file.ProtectWorkbook(&opts); err != nil {
		return fmt.Errorf("protect workbook: %w", err)
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	w.dirty = true
	return nil
}

// UnprotectWorkbook removes workbook structure protection with an optional password.
func (w *Workbook) UnprotectWorkbook(password string) error {
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
		errUnprotect = w.file.UnprotectWorkbook(password)
	} else {
		errUnprotect = w.file.UnprotectWorkbook()
	}
	if errUnprotect != nil {
		return fmt.Errorf("unprotect workbook: %w", errUnprotect)
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	w.dirty = true
	return nil
}

