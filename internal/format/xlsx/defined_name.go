package xlsx

import (
	"errors"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"glance/internal/document"
)

var _ document.SpreadsheetDefinedNameEditor = (*Workbook)(nil)

// GetDefinedNames returns all defined names in the workbook.
func (w *Workbook) GetDefinedNames() ([]document.SpreadsheetDefinedName, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	raw := w.file.GetDefinedName()
	results := make([]document.SpreadsheetDefinedName, 0, len(raw))
	for _, item := range raw {
		results = append(results, document.SpreadsheetDefinedName{
			Name:    item.Name,
			Ref:     item.RefersTo,
			Scope:   item.Scope,
			Comment: item.Comment,
		})
	}
	return results, nil
}

// SetDefinedName creates or updates a named range.
func (w *Workbook) SetDefinedName(name document.SpreadsheetDefinedName) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	trimmedName := strings.TrimSpace(name.Name)
	if trimmedName == "" {
		return errors.New("defined name cannot be empty")
	}
	trimmedRef := strings.TrimSpace(name.Ref)
	if trimmedRef == "" {
		return errors.New("defined name target reference cannot be empty")
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return fmt.Errorf("capture defined name history: %w", err)
	}
	// Delete any existing name with same name & scope
	_ = w.file.DeleteDefinedName(&excelize.DefinedName{
		Name:  trimmedName,
		Scope: strings.TrimSpace(name.Scope),
	})
	if err := w.file.SetDefinedName(&excelize.DefinedName{
		Name:     trimmedName,
		RefersTo: trimmedRef,
		Scope:    strings.TrimSpace(name.Scope),
		Comment:  name.Comment,
	}); err != nil {
		return err
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	w.dirty = true
	return nil
}

// DeleteDefinedName deletes a defined name by name and scope.
func (w *Workbook) DeleteDefinedName(name string, scope string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return errors.New("defined name cannot be empty")
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return fmt.Errorf("capture defined name history: %w", err)
	}
	if err := w.file.DeleteDefinedName(&excelize.DefinedName{
		Name:  trimmedName,
		Scope: strings.TrimSpace(scope),
	}); err != nil {
		return err
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	w.dirty = true
	return nil
}
