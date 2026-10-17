package desktopui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"glance/internal/wordlayout"
	"math"
	"path/filepath"
	"strings"
	"time"

	"glance/internal/application"
	"glance/internal/document"
	csvdoc "glance/internal/format/csv"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/markdowndoc"
	"glance/internal/format/pdfdoc"
	"glance/internal/format/pptx"
	"glance/internal/format/xlsx"
)

type desktopSessionAdapter struct {
	runtime *desktopRuntime
	session *sharedDocumentSession
	format  application.Format
}

func (a *desktopSessionAdapter) DocumentID() string         { return a.session.documentID }
func (a *desktopSessionAdapter) Format() application.Format { return a.format }
func (a *desktopSessionAdapter) SourceFormat() application.Format {
	a.session.adapterMu.RLock()
	defer a.session.adapterMu.RUnlock()
	if a.session.sourceFormat != "" {
		return a.session.sourceFormat
	}
	return a.format
}
func (a *desktopSessionAdapter) Path() string {
	a.session.adapterMu.RLock()
	defer a.session.adapterMu.RUnlock()
	return a.session.path
}
func (a *desktopSessionAdapter) Dirty() bool {
	a.session.adapterMu.RLock()
	defer a.session.adapterMu.RUnlock()
	s := a.session
	return s.book != nil && s.book.Dirty() || s.word != nil && s.word.Dirty() || s.jsonDoc != nil && s.jsonDoc.Dirty() || s.markdownDoc != nil && s.markdownDoc.Dirty() || s.pdfDoc != nil && s.pdfDoc.Dirty() || s.pptxDoc != nil && s.pptxDoc.Dirty()
}
func (a *desktopSessionAdapter) ReadOnly() bool {
	a.session.adapterMu.RLock()
	defer a.session.adapterMu.RUnlock()
	return a.session.book != nil && !a.session.book.Editable()
}
func (a *desktopSessionAdapter) Inspect(ctx context.Context, request application.InspectRequest) (json.RawMessage, []application.Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return nil, nil, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	switch {
	case s.book != nil:
		return application.InspectSpreadsheet(ctx, s.book, request)
	case s.word != nil:
		return application.InspectWord(ctx, s.word, request)
	case s.jsonDoc != nil:
		return application.InspectJSON(ctx, s.jsonDoc, request)
	case s.markdownDoc != nil:
		return application.InspectMarkdown(ctx, s.markdownDoc, request)
	case s.pdfDoc != nil:
		return application.InspectPDF(ctx, s.pdfDoc, request)
	case s.pptxDoc != nil:
		return application.InspectPresentation(ctx, s.pptxDoc, request)
	default:
		return nil, nil, application.ErrQueryNotSupported
	}
}

func (a *desktopSessionAdapter) ReadJSONText(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return "", application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.jsonDoc == nil {
		return "", application.ErrCapabilityUnsupported
	}
	return s.jsonDoc.Text(), nil
}

func (a *desktopSessionAdapter) SetJSONText(ctx context.Context, value string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.jsonDoc == nil {
		return false, application.ErrCapabilityUnsupported
	}
	if s.jsonDoc.Text() == value {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := s.jsonDoc.SetText(value); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) ReadMarkdownText(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return "", application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.markdownDoc == nil {
		return "", application.ErrCapabilityUnsupported
	}
	return s.markdownDoc.Text(), nil
}

func (a *desktopSessionAdapter) SetMarkdownText(ctx context.Context, value string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.markdownDoc == nil {
		return false, application.ErrCapabilityUnsupported
	}
	if s.markdownDoc.Text() == value {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := s.markdownDoc.SetText(value); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) ReadWordBlockWindow(ctx context.Context, start, limit int) ([]document.WordBlock, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return nil, 0, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.word == nil {
		return nil, 0, application.ErrCapabilityUnsupported
	}
	return s.word.WordBlockWindow(start, limit)
}

func (a *desktopSessionAdapter) SetCellText(ctx context.Context, sheet, cell, value string) (bool, error) {
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
	if reader, ok := s.book.(interface {
		Value(string, string) (string, error)
	}); ok {
		current, err := reader.Value(sheet, cell)
		if err != nil {
			return false, err
		}
		if current == value {
			return false, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := s.book.SetCellText(sheet, cell, value); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) SetSpreadsheetCellValue(ctx context.Context, sheet, cell string, value document.CellValue) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetCellValueEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return editor.SetCellValueChanged(sheet, cell, value)
}

func (a *desktopSessionAdapter) SetSpreadsheetCellDate(ctx context.Context, sheet, cell string, date time.Time) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetCellDateEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return editor.SetCellDateChanged(sheet, cell, date)
}

func (a *desktopSessionAdapter) SetSpreadsheetCellTime(ctx context.Context, sheet, cell string, value time.Time) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetCellTimeEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return editor.SetCellTimeChanged(sheet, cell, value)
}

func (a *desktopSessionAdapter) SetSpreadsheetCellFormula(ctx context.Context, sheet, cell, formula string) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetFormulaEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	cleanFormula := strings.TrimPrefix(strings.TrimSpace(formula), "=")
	current, err := editor.GetCellFormula(sheet, cell)
	if err != nil {
		return false, err
	}
	if current == cleanFormula {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.SetCellFormula(sheet, cell, formula); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) SetSpreadsheetFormulas(ctx context.Context, sheet string, edits []document.CellFormulaEdit) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetFormulaBatchEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return editor.SetCellFormulasChanged(sheet, edits)
}

func (a *desktopSessionAdapter) SetSpreadsheetFormulaRange(ctx context.Context, sheet string, active document.CellCoordinate, bounds document.CellRange, formula string) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetFormulaRangeEntryEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return editor.SetCellFormulaRangeChanged(sheet, active, bounds, formula)
}

func (a *desktopSessionAdapter) RecalculateSpreadsheet(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil || !s.book.Editable() {
		return application.ErrReadOnlyDocument
	}
	recalculator, ok := s.book.(document.SpreadsheetRecalculateEditor)
	if !ok {
		return application.ErrCapabilityUnsupported
	}
	return recalculator.RecalculateFormulas()
}

func (a *desktopSessionAdapter) SetSpreadsheetCells(ctx context.Context, sheet string, edits []document.CellEdit) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetBatchChangeEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return editor.SetCellsChanged(sheet, edits)
}

func (a *desktopSessionAdapter) TransferSpreadsheetCellRange(ctx context.Context, sheet string, source document.CellRange, targetRow, targetColumn int, cells [][]document.Cell) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetCellTransferEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return editor.TransferCellRangeChanged(sheet, source, targetRow, targetColumn, cells)
}

func (a *desktopSessionAdapter) FillDownSpreadsheet(ctx context.Context, sheet string, bounds document.CellRange) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetFillDownEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return editor.FillDownChanged(sheet, bounds)
}

func (a *desktopSessionAdapter) FillRightSpreadsheet(ctx context.Context, sheet string, bounds document.CellRange) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetFillRightEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return editor.FillRightChanged(sheet, bounds)
}

func (a *desktopSessionAdapter) MergeCellsSpreadsheet(ctx context.Context, sheet string, bounds document.CellRange, confirmationToken string) (document.SpreadsheetMergeResult, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetMergeResult{}, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.SpreadsheetMergeResult{}, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil || !s.book.Editable() {
		return document.SpreadsheetMergeResult{}, application.ErrReadOnlyDocument
	}
	editor, ok := any(s.book).(document.SpreadsheetMergeCellsEditor)
	if !ok {
		return document.SpreadsheetMergeResult{}, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetMergeResult{}, err
	}
	return editor.MergeCellsChanged(sheet, bounds, confirmationToken)
}

func (a *desktopSessionAdapter) SetSpreadsheetCellFormats(ctx context.Context, sheet string, bounds document.CellRange, patch document.CellFormatPatch) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetFormatChangeEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	return editor.SetCellFormatsChanged(sheet, bounds, patch)
}

func (a *desktopSessionAdapter) PaintSpreadsheetCellFormat(ctx context.Context, sheet, sourceCell string, targetBounds document.CellRange) (bool, error) {
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
	painter, ok := any(s.book).(document.SpreadsheetCellFormatPainter)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	return painter.PaintCellFormat(sheet, sourceCell, targetBounds)
}

func (a *desktopSessionAdapter) SortSpreadsheetRange(ctx context.Context, request document.SortRequest) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetSortChangeEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	return editor.SortRangeChanged(request)
}

func (a *desktopSessionAdapter) ApplySpreadsheetHistory(ctx context.Context, undo bool) (bool, error) {
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
	history, ok := any(s.book).(document.SpreadsheetHistory)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if undo {
		return history.Undo()
	}
	return history.Redo()
}

func (a *desktopSessionAdapter) FindSpreadsheetCells(ctx context.Context, sheet, query string, limit int) ([]document.CellMatch, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return nil, false, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return nil, false, application.ErrCapabilityUnsupported
	}
	searcher, ok := any(s.book).(document.SpreadsheetSearcher)
	if !ok {
		return nil, false, application.ErrCapabilityUnsupported
	}
	matches, err := searcher.SearchSheet(sheet, query)
	if err != nil {
		return nil, false, err
	}
	truncated := len(matches) > limit
	if truncated {
		matches = matches[:limit]
	}
	if matches == nil {
		matches = []document.CellMatch{}
	}
	return matches, truncated, nil
}

func (a *desktopSessionAdapter) ApplySpreadsheetStructure(ctx context.Context, edit document.StructureEdit) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetStructureEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if edit.Kind == document.StructureRenameSheet && edit.NewName == edit.Sheet {
		return false, nil
	}
	if err := editor.EditStructure(edit); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) ReadSpreadsheetRange(ctx context.Context, sheet string, bounds document.CellRange) ([][]document.Cell, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return nil, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return nil, application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.book).(document.SpreadsheetRangeReader)
	if !ok {
		return nil, application.ErrCapabilityUnsupported
	}
	if clipboardReader, ok := any(s.book).(document.SpreadsheetClipboardRangeReader); ok {
		return clipboardReader.ReadClipboardRange(sheet, bounds)
	}
	return reader.ReadRange(sheet, bounds)
}

func (a *desktopSessionAdapter) ReadSpreadsheetGridWindow(ctx context.Context, sheet string, bounds document.CellRange) (document.SheetWindow, error) {
	if err := ctx.Err(); err != nil {
		return document.SheetWindow{}, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.SheetWindow{}, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return document.SheetWindow{}, application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.book).(document.GridReader)
	if !ok {
		return document.SheetWindow{}, application.ErrCapabilityUnsupported
	}
	if bounds.StartRow < 1 || bounds.StartRow > 1_048_576 || bounds.EndRow < bounds.StartRow || bounds.EndRow > 1_048_576 || bounds.StartColumn < 1 || bounds.StartColumn > 16_384 || bounds.EndColumn < bounds.StartColumn || bounds.EndColumn > 16_384 {
		return document.SheetWindow{}, application.ErrInvalidCommandArguments
	}
	rows := bounds.EndRow - bounds.StartRow + 1
	columns := bounds.EndColumn - bounds.StartColumn + 1
	if rows < 1 || columns < 1 || rows > 10_000 || columns > 10_000 || rows*columns > 10_000 {
		return document.SheetWindow{}, application.ErrInvalidCommandArguments
	}
	return reader.Window(sheet, bounds.StartRow, bounds.StartColumn, rows, columns)
}

func (a *desktopSessionAdapter) ReadSpreadsheetPDFWindow(ctx context.Context, sheet string, bounds document.CellRange) (document.SheetWindow, error) {
	if err := ctx.Err(); err != nil {
		return document.SheetWindow{}, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.SheetWindow{}, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return document.SheetWindow{}, application.ErrCapabilityUnsupported
	}
	rows, columns := bounds.EndRow-bounds.StartRow+1, bounds.EndColumn-bounds.StartColumn+1
	if bounds.StartRow < 1 || bounds.StartColumn < 1 || rows < 1 || rows > 5000 || columns < 1 || columns > 100 || rows*columns > 500_000 || bounds.EndRow > 1_048_576 || bounds.EndColumn > 16_384 {
		return document.SheetWindow{}, application.ErrInvalidCommandArguments
	}
	reader, ok := any(s.book).(document.GridReader)
	if !ok {
		return document.SheetWindow{}, application.ErrCapabilityUnsupported
	}
	return reader.Window(sheet, bounds.StartRow, bounds.StartColumn, rows, columns)
}

func (a *desktopSessionAdapter) GetSpreadsheetPageLayout(ctx context.Context, sheet string) (document.SpreadsheetPageLayout, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetPageLayout{}, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.SpreadsheetPageLayout{}, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return document.SpreadsheetPageLayout{}, application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.book).(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return document.SpreadsheetPageLayout{}, application.ErrCapabilityUnsupported
	}
	return reader.GetSheetPageLayout(sheet)
}

func (a *desktopSessionAdapter) SetSpreadsheetPageLayout(ctx context.Context, sheet string, patch application.SpreadsheetPageLayoutPatch) (document.SpreadsheetPageLayout, bool, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetPageLayout{}, false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.SpreadsheetPageLayout{}, false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil || !s.book.Editable() {
		return document.SpreadsheetPageLayout{}, false, application.ErrReadOnlyDocument
	}
	editor, ok := any(s.book).(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return document.SpreadsheetPageLayout{}, false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetPageLayout{}, false, err
	}
	return application.ApplySpreadsheetPageLayoutPatch(editor, sheet, patch)
}

func (a *desktopSessionAdapter) GetSpreadsheetGridlines(ctx context.Context, sheet string) (bool, error) {
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
	reader, ok := any(s.book).(document.SpreadsheetGridlinesEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	return reader.GetSheetGridlines(sheet)
}

func (a *desktopSessionAdapter) SetSpreadsheetGridlines(ctx context.Context, sheet string, show bool) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetGridlinesEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	return application.ApplySpreadsheetGridlines(ctx, editor, sheet, show)
}

func (a *desktopSessionAdapter) SetSpreadsheetDataValidation(ctx context.Context, spec document.SpreadsheetDataValidationSpec, clear bool) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetDataValidationEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if clear {
		return editor.DeleteDataValidation(spec.Sheet, spec.Range)
	}
	return editor.AddDataValidation(spec)
}

func (a *desktopSessionAdapter) SetSpreadsheetConditionalFormat(ctx context.Context, spec document.SpreadsheetConditionalFormatSpec, clear bool) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetConditionalFormatEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if clear {
		return editor.ClearConditionalFormat(spec.Sheet, spec.Range)
	}
	return editor.SetConditionalFormat(spec)
}

func (a *desktopSessionAdapter) CreateSpreadsheetTable(ctx context.Context, spec document.SpreadsheetTableSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil || !s.book.Editable() {
		return application.ErrReadOnlyDocument
	}
	editor, ok := any(s.book).(document.SpreadsheetTableEditor)
	if !ok {
		return application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return editor.AddTable(spec)
}

func (a *desktopSessionAdapter) CreateSpreadsheetPivotTable(ctx context.Context, spec document.SpreadsheetPivotTableSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil || !s.book.Editable() {
		return application.ErrReadOnlyDocument
	}
	editor, ok := any(s.book).(document.SpreadsheetPivotTableEditor)
	if !ok {
		return application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return editor.AddPivotTable(spec)
}

func (a *desktopSessionAdapter) CreateSpreadsheetChart(ctx context.Context, spec document.SpreadsheetChartSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil || !s.book.Editable() {
		return application.ErrReadOnlyDocument
	}
	editor, ok := any(s.book).(document.SpreadsheetChartEditor)
	if !ok {
		return application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return editor.AddChart(spec)
}

func (a *desktopSessionAdapter) CreateSpreadsheetShape(ctx context.Context, spec document.SpreadsheetShapeSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil || !s.book.Editable() {
		return application.ErrReadOnlyDocument
	}
	editor, ok := any(s.book).(document.SpreadsheetShapeEditor)
	if !ok {
		return application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return editor.AddShape(spec)
}

func (a *desktopSessionAdapter) GetSpreadsheetDefinedNames(ctx context.Context) ([]document.SpreadsheetDefinedName, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return nil, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return nil, application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.book).(document.SpreadsheetDefinedNameEditor)
	if !ok {
		return nil, application.ErrCapabilityUnsupported
	}
	return reader.GetDefinedNames()
}

func (a *desktopSessionAdapter) SetSpreadsheetDefinedName(ctx context.Context, name document.SpreadsheetDefinedName) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetDefinedNameEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	current, err := editor.GetDefinedNames()
	if err != nil {
		return false, err
	}
	for _, existing := range current {
		if strings.EqualFold(existing.Name, name.Name) && strings.EqualFold(existing.Scope, name.Scope) && existing.Ref == name.Ref && existing.Comment == name.Comment {
			return false, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.SetDefinedName(name); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) DeleteSpreadsheetDefinedName(ctx context.Context, name, scope string) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetDefinedNameEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	current, err := editor.GetDefinedNames()
	if err != nil {
		return false, err
	}
	found := false
	for _, existing := range current {
		if strings.EqualFold(existing.Name, name) && strings.EqualFold(existing.Scope, scope) {
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.DeleteDefinedName(name, scope); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) SplitSpreadsheetTextToColumns(ctx context.Context, sheet string, bounds document.CellRange, keyColumn int, delimiter string) (document.SpreadsheetTextToColumnsResult, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetTextToColumnsResult{}, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.SpreadsheetTextToColumnsResult{}, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil || !s.book.Editable() {
		return document.SpreadsheetTextToColumnsResult{}, application.ErrReadOnlyDocument
	}
	editor, ok := any(s.book).(document.SpreadsheetTextToColumnsEditor)
	if !ok {
		return document.SpreadsheetTextToColumnsResult{}, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetTextToColumnsResult{}, err
	}
	return editor.SplitTextToColumns(sheet, bounds, keyColumn, delimiter)
}

func (a *desktopSessionAdapter) RemoveSpreadsheetDuplicates(ctx context.Context, sheet string, bounds document.CellRange, keyColumn int) (document.SpreadsheetDuplicatesResult, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetDuplicatesResult{}, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.SpreadsheetDuplicatesResult{}, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil || !s.book.Editable() {
		return document.SpreadsheetDuplicatesResult{}, application.ErrReadOnlyDocument
	}
	editor, ok := any(s.book).(document.SpreadsheetDuplicatesEditor)
	if !ok {
		return document.SpreadsheetDuplicatesResult{}, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetDuplicatesResult{}, err
	}
	return editor.RemoveDuplicates(sheet, bounds, keyColumn)
}

func (a *desktopSessionAdapter) GetSpreadsheetPanes(ctx context.Context, sheet string) (document.SpreadsheetPaneFreeze, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetPaneFreeze{}, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.SpreadsheetPaneFreeze{}, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return document.SpreadsheetPaneFreeze{}, application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.book).(document.SpreadsheetPanesEditor)
	if !ok {
		return document.SpreadsheetPaneFreeze{}, application.ErrCapabilityUnsupported
	}
	return reader.GetSheetPanes(sheet)
}

func (a *desktopSessionAdapter) SetSpreadsheetPanes(ctx context.Context, sheet string, panes document.SpreadsheetPaneFreeze) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetPanesEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	current, err := editor.GetSheetPanes(sheet)
	if err != nil {
		return false, err
	}
	if current == panes {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.SetSheetPanes(sheet, panes); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) GetSpreadsheetView(ctx context.Context, sheet string) (document.SpreadsheetSheetView, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetSheetView{}, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.SpreadsheetSheetView{}, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return document.SpreadsheetSheetView{}, application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.book).(document.SpreadsheetViewsEditor)
	if !ok {
		return document.SpreadsheetSheetView{}, application.ErrCapabilityUnsupported
	}
	return reader.GetSheetView(sheet)
}

func (a *desktopSessionAdapter) SetSpreadsheetViewMode(ctx context.Context, sheet, mode string) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetViewsEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	current, err := editor.GetSheetView(sheet)
	if err != nil {
		return false, err
	}
	if current.ViewMode == mode || (current.ViewMode == "" && mode == "normal") {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.SetSheetViewMode(sheet, mode); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) SetSpreadsheetViewRuler(ctx context.Context, sheet string, show bool) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetViewsEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	current, err := editor.GetSheetView(sheet)
	if err != nil {
		return false, err
	}
	if current.ShowRuler == show {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.SetSheetRuler(sheet, show); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) GetSpreadsheetComments(ctx context.Context, sheet string) ([]document.SpreadsheetComment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return nil, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return nil, application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.book).(document.SpreadsheetCommentEditor)
	if !ok {
		return nil, application.ErrCapabilityUnsupported
	}
	return reader.GetComments(sheet)
}

func (a *desktopSessionAdapter) AddSpreadsheetComment(ctx context.Context, sheet string, comment document.SpreadsheetComment) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetCommentEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	comments, err := editor.GetComments(sheet)
	if err != nil {
		return false, err
	}
	for _, existing := range comments {
		if strings.EqualFold(existing.Cell, comment.Cell) && existing.Author == comment.Author && existing.Text == comment.Text {
			return false, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.AddComment(sheet, comment); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) DeleteSpreadsheetComment(ctx context.Context, sheet, cell string) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetCommentEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	comments, err := editor.GetComments(sheet)
	if err != nil {
		return false, err
	}
	found := false
	for _, existing := range comments {
		if strings.EqualFold(existing.Cell, cell) {
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.DeleteComment(sheet, cell); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) FlashFillSpreadsheet(ctx context.Context, sheet string, bounds document.CellRange) (document.SpreadsheetFlashFillResult, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetFlashFillResult{}, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.SpreadsheetFlashFillResult{}, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil || !s.book.Editable() {
		return document.SpreadsheetFlashFillResult{}, application.ErrReadOnlyDocument
	}
	editor, ok := any(s.book).(document.SpreadsheetFlashFillEditor)
	if !ok {
		return document.SpreadsheetFlashFillResult{}, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetFlashFillResult{}, err
	}
	return editor.FlashFill(sheet, bounds)
}

func (a *desktopSessionAdapter) GetSpreadsheetHyperlink(ctx context.Context, sheet, cell string) (document.CellHyperlink, bool, error) {
	if err := ctx.Err(); err != nil {
		return document.CellHyperlink{}, false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.CellHyperlink{}, false, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return document.CellHyperlink{}, false, application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.book).(document.SpreadsheetHyperlinkEditor)
	if !ok {
		return document.CellHyperlink{}, false, application.ErrCapabilityUnsupported
	}
	return reader.GetCellHyperlink(sheet, cell)
}

func (a *desktopSessionAdapter) SetSpreadsheetHyperlink(ctx context.Context, sheet, cell, target, tooltip string, remove bool) (bool, error) {
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
	editor, ok := any(s.book).(document.SpreadsheetHyperlinkEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	current, exists, err := editor.GetCellHyperlink(sheet, cell)
	if err != nil {
		return false, err
	}
	if remove {
		if !exists {
			return false, nil
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if err := editor.RemoveCellHyperlink(sheet, cell); err != nil {
			return false, err
		}
		return true, nil
	}
	if exists && current.Target == target && current.Tooltip == tooltip {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.SetCellHyperlink(sheet, cell, target, tooltip); err != nil {
		return false, err
	}
	return true, nil
}

func (a *desktopSessionAdapter) ReplaceWordTextRun(ctx context.Context, runID, value string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.word == nil {
		return false, application.ErrCapabilityUnsupported
	}
	committed, drafts := s.wordTextStateSnapshot()
	for draftRunID, draft := range drafts {
		if draft != committed[draftRunID] && (draftRunID != runID || draft != value) {
			return false, application.ErrPendingEditorDraft
		}
	}
	blocks := s.word.WordBlocks()
	if current, ok := selectedWordRun(blocks, runID); ok && current.Editable && current.Text == value {
		s.wordBlocksMu.Lock()
		s.wordBlocks = cloneWordBlockSnapshot(blocks)
		s.wordBlocksMu.Unlock()
		s.syncWordTextState(blocks, true)
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	editor, ok := any(s.word).(document.WordTextEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	if err := editor.ReplaceTextRun(runID, value); err != nil {
		return false, err
	}
	blocks = cloneWordBlockSnapshot(s.word.WordBlocks())
	s.wordBlocksMu.Lock()
	s.wordBlocks = blocks
	s.wordBlocksMu.Unlock()
	s.syncWordTextState(blocks, true)
	s.cancelWordLayout()
	return true, nil
}

func (a *desktopSessionAdapter) AppendWordTextRun(ctx context.Context, runID, value string) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		run, ok := selectedWordRun(content.WordBlocks(), runID)
		if !ok || !run.Editable {
			return false, application.ErrWordTextRunUnavailable
		}
		editor, ok := content.(document.WordTextEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		appended := value
		if run.Text != "" {
			appended = run.Text + " " + value
		}
		if err := editor.ReplaceTextRun(runID, appended); err != nil {
			return false, err
		}
		return appended != run.Text, nil
	})
}

func (a *desktopSessionAdapter) ReplaceWordTextRange(ctx context.Context, runID string, start, end int, replacement string) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		run, ok := selectedWordRun(content.WordBlocks(), runID)
		if !ok || !run.Editable {
			return false, application.ErrWordTextRunUnavailable
		}
		current := []rune(run.Text)
		if start < 0 || end < start || end > len(current) {
			return false, application.ErrInvalidCommandArguments
		}
		updated := make([]rune, 0, len(current)-(end-start)+len([]rune(replacement)))
		updated = append(updated, current[:start]...)
		updated = append(updated, []rune(replacement)...)
		updated = append(updated, current[end:]...)
		if string(updated) == run.Text {
			return false, nil
		}
		editor, ok := content.(document.WordTextRangeEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.ReplaceTextRange(runID, start, end, replacement); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) ApplyWordFormatting(ctx context.Context, sourceRunID, targetRunID string) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		blocks := content.WordBlocks()
		if sourceRun, ok := selectedWordRun(blocks, sourceRunID); !ok || !sourceRun.Editable {
			return false, application.ErrWordTextRunUnavailable
		}
		if targetRun, ok := selectedWordRun(blocks, targetRunID); !ok || !targetRun.Editable {
			return false, application.ErrWordTextRunUnavailable
		}
		painter, ok := content.(document.WordFormatPainter)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		return painter.ApplyWordFormatting(sourceRunID, targetRunID)
	})
}

func (a *desktopSessionAdapter) ReadWordTextRange(ctx context.Context, runID string, start, end int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return "", application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.word == nil {
		return "", application.ErrCapabilityUnsupported
	}
	run, ok := selectedWordRun(s.word.WordBlocks(), runID)
	if !ok || !run.Editable {
		return "", application.ErrWordTextRunUnavailable
	}
	current := []rune(run.Text)
	if start < 0 || end < start || end > len(current) {
		return "", application.ErrInvalidCommandArguments
	}
	return string(current[start:end]), nil
}

func (a *desktopSessionAdapter) CutWordTextRange(ctx context.Context, runID string, start, end int) (string, bool, error) {
	var selected string
	changed, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		run, ok := selectedWordRun(content.WordBlocks(), runID)
		if !ok || !run.Editable {
			return false, application.ErrWordTextRunUnavailable
		}
		current := []rune(run.Text)
		if start < 0 || end < start || end > len(current) {
			return false, application.ErrInvalidCommandArguments
		}
		selected = string(current[start:end])
		if len(selected) > application.MaxQueryResultBytes {
			return false, application.ErrQueryResultTooLarge
		}
		if start == end {
			return false, nil
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		editor, ok := content.(document.WordTextRangeEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.ReplaceTextRange(runID, start, end, ""); err != nil {
			return false, err
		}
		return true, nil
	})
	return selected, changed, err
}

func (a *desktopSessionAdapter) SetWordTextRunStyle(ctx context.Context, runID string, patch document.WordTextFormatPatch) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		run, ok := selectedWordRun(content.WordBlocks(), runID)
		if !ok || !run.Editable {
			return false, application.ErrWordTextRunUnavailable
		}
		style := run.Style
		if reader, ok := content.(document.WordLayoutReader); ok {
			if resolved, found := wordlayout.ResolveDocumentRunStyle(reader.WordLayoutData(), content.WordBlocks(), runID); found {
				style = resolved
			}
		}
		if desktopWordTextStylePatchUnchanged(style, patch) {
			return false, nil
		}
		editor, ok := content.(document.WordTextEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.SetTextRunStyle(runID, patch); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) SetWordParagraphAlignment(ctx context.Context, paragraphID string, alignment document.WordAlignment) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		paragraph, ok := selectedWordParagraph(content.WordBlocks(), paragraphID)
		if !ok {
			return false, application.ErrWordParagraphUnavailable
		}
		if paragraph.Alignment == alignment {
			return false, nil
		}
		editor, ok := content.(document.WordTextEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.SetParagraphAlignment(paragraphID, alignment); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) SetWordParagraphIndents(ctx context.Context, paragraphID string, leftEMU, rightEMU int64) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		paragraph, ok := selectedWordParagraph(content.WordBlocks(), paragraphID)
		if !ok {
			return false, application.ErrWordParagraphUnavailable
		}
		if paragraph.LeftIndentEMU == leftEMU && paragraph.RightIndentEMU == rightEMU {
			return false, nil
		}
		editor, ok := content.(document.WordTextEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.SetParagraphIndents(paragraphID, leftEMU, rightEMU); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) SetWordTextRunFontFamily(ctx context.Context, runID, family string) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		run, ok := selectedWordRun(content.WordBlocks(), runID)
		if !ok || !run.Editable {
			return false, application.ErrWordTextRunUnavailable
		}
		if run.Style.FontFamily == family {
			return false, nil
		}
		editor, ok := content.(document.WordTextFontFamilyEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.SetTextRunFontFamily(runID, family); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) SetWordTextRunFontSize(ctx context.Context, runID string, points float64) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		run, ok := selectedWordRun(content.WordBlocks(), runID)
		if !ok || !run.Editable {
			return false, application.ErrWordTextRunUnavailable
		}
		if run.Style.FontSize == points {
			return false, nil
		}
		editor, ok := content.(document.WordTextFontSizeEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.SetTextRunFontSize(runID, points); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) SetWordParagraphList(ctx context.Context, paragraphID string, kind document.WordListKind) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		paragraph, ok := selectedWordParagraph(content.WordBlocks(), paragraphID)
		if !ok {
			return false, application.ErrWordParagraphUnavailable
		}
		if paragraph.ListKind == kind {
			return false, nil
		}
		editor, ok := content.(document.WordParagraphListEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.SetParagraphList(paragraphID, kind); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) SetWordParagraphSpacing(ctx context.Context, paragraphID string, beforeTwips, afterTwips *int64) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		paragraph, ok := selectedWordParagraph(content.WordBlocks(), paragraphID)
		if !ok {
			return false, application.ErrWordParagraphUnavailable
		}
		if desktopWordParagraphSpacingUnchanged(paragraph, beforeTwips, afterTwips) {
			return false, nil
		}
		editor, ok := content.(document.WordParagraphSpacingEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.SetParagraphSpacing(paragraphID, beforeTwips, afterTwips); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) InsertWordTable(ctx context.Context, afterParagraphID string, rows, columns int) (string, error) {
	var tableID string
	_, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		if afterParagraphID != "" {
			if _, ok := selectedWordBodyParagraph(content.WordBlocks(), afterParagraphID); !ok {
				return false, application.ErrWordParagraphUnavailable
			}
		}
		editor, ok := content.(document.WordTableInsertEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		var err error
		tableID, err = editor.InsertTable(afterParagraphID, rows, columns)
		return err == nil, err
	})
	return tableID, err
}

func (a *desktopSessionAdapter) InsertWordTextBox(ctx context.Context, afterParagraphID, text string) error {
	_, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		if afterParagraphID != "" {
			if _, ok := selectedWordBodyParagraph(content.WordBlocks(), afterParagraphID); !ok {
				return false, application.ErrWordParagraphUnavailable
			}
		}
		editor, ok := content.(document.WordTextBoxEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.InsertTextBox(afterParagraphID, text); err != nil {
			return false, err
		}
		return true, nil
	})
	return err
}

func (a *desktopSessionAdapter) SetWordHeader(ctx context.Context, text string) (bool, error) {
	return a.mutateWordPackage(ctx, func(content document.WordContentReader) error {
		editor, ok := content.(document.WordHeaderFooterEditor)
		if !ok {
			return application.ErrCapabilityUnsupported
		}
		return editor.SetHeader(text)
	})
}

func (a *desktopSessionAdapter) SetWordFooter(ctx context.Context, text string) (bool, error) {
	return a.mutateWordPackage(ctx, func(content document.WordContentReader) error {
		editor, ok := content.(document.WordHeaderFooterEditor)
		if !ok {
			return application.ErrCapabilityUnsupported
		}
		return editor.SetFooter(text)
	})
}

func (a *desktopSessionAdapter) SetWordPageNumber(ctx context.Context, alignment string) (bool, error) {
	return a.mutateWordPackage(ctx, func(content document.WordContentReader) error {
		editor, ok := content.(document.WordHeaderFooterEditor)
		if !ok {
			return application.ErrCapabilityUnsupported
		}
		return editor.SetPageNumber(alignment)
	})
}

func (a *desktopSessionAdapter) SetWordPageOrientation(ctx context.Context, orientation string) (bool, error) {
	return a.mutateWordPageSetup(ctx, func(editor document.WordPageSetupEditor) error {
		return editor.SetPageOrientation(orientation)
	})
}

func (a *desktopSessionAdapter) SetWordPageMargins(ctx context.Context, top, right, bottom, left float64) (bool, error) {
	return a.mutateWordPageSetup(ctx, func(editor document.WordPageSetupEditor) error {
		return editor.SetPageMargins(top, right, bottom, left)
	})
}

func (a *desktopSessionAdapter) SetWordPageSize(ctx context.Context, width, height float64) (bool, error) {
	return a.mutateWordPageSetup(ctx, func(editor document.WordPageSetupEditor) error {
		return editor.SetPageSize(width, height)
	})
}

func (a *desktopSessionAdapter) SetWordPageColumns(ctx context.Context, columns int) (bool, error) {
	return a.mutateWordPageSetup(ctx, func(editor document.WordPageSetupEditor) error {
		return editor.SetPageColumns(columns)
	})
}

func (a *desktopSessionAdapter) SetWordPageLineNumbers(ctx context.Context, mode string) (bool, error) {
	return a.mutateWordPageSetup(ctx, func(editor document.WordPageSetupEditor) error {
		return editor.SetPageLineNumbers(mode)
	})
}

func (a *desktopSessionAdapter) SetWordPageBorders(ctx context.Context, borderType string) (bool, error) {
	return a.mutateWordPageSetup(ctx, func(editor document.WordPageSetupEditor) error {
		return editor.SetPageBorders(borderType)
	})
}

func (a *desktopSessionAdapter) GetWordDocumentProperties(ctx context.Context) (document.WordDocumentProperties, error) {
	if err := ctx.Err(); err != nil {
		return document.WordDocumentProperties{}, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.WordDocumentProperties{}, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.word == nil {
		return document.WordDocumentProperties{}, application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.word).(document.WordDocumentPropertiesEditor)
	if !ok {
		return document.WordDocumentProperties{}, application.ErrCapabilityUnsupported
	}
	return reader.GetWordDocumentProperties()
}

func (a *desktopSessionAdapter) PatchWordDocumentProperties(ctx context.Context, patch application.WordDocumentPropertiesPatch) (document.WordDocumentProperties, bool, error) {
	var result document.WordDocumentProperties
	changed, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordDocumentPropertiesEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		current, err := editor.GetWordDocumentProperties()
		if err != nil {
			return false, err
		}
		result = application.ApplyWordDocumentPropertiesPatch(current, patch)
		if result == current {
			return false, nil
		}
		if err := editor.SetWordDocumentProperties(result); err != nil {
			return false, err
		}
		return true, nil
	})
	return result, changed, err
}

func (a *desktopSessionAdapter) GetWordWatermark(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return "", application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.word == nil {
		return "", application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.word).(document.WordWatermarkEditor)
	if !ok {
		return "", application.ErrCapabilityUnsupported
	}
	return reader.GetWatermark()
}

func (a *desktopSessionAdapter) SetWordWatermark(ctx context.Context, text string) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordWatermarkEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		current, err := editor.GetWatermark()
		if err != nil {
			return false, err
		}
		if current == text {
			return false, nil
		}
		if text == "" {
			err = editor.ClearWatermark()
		} else {
			err = editor.SetWatermark(text)
		}
		return err == nil, err
	})
}

func (a *desktopSessionAdapter) GetWordTrackRevisions(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.word == nil {
		return false, application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.word).(document.WordTrackRevisionsEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	return reader.GetTrackRevisions()
}

func (a *desktopSessionAdapter) SetWordTrackRevisions(ctx context.Context, enabled bool) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordTrackRevisionsEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		current, err := editor.GetTrackRevisions()
		if err != nil {
			return false, err
		}
		if current == enabled {
			return false, nil
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if err := editor.SetTrackRevisions(enabled); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) ApplyWordRevisionChanges(ctx context.Context, action string) (int, error) {
	if action != "accept_all" && action != "reject_all" {
		return 0, application.ErrInvalidCommandArguments
	}
	var count int
	_, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordTrackRevisionsEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		var err error
		if action == "accept_all" {
			count, err = editor.AcceptAllRevisions()
		} else {
			count, err = editor.RejectAllRevisions()
		}
		return count > 0, err
	})
	return count, err
}

func (a *desktopSessionAdapter) GetWordDocumentProtection(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.word == nil {
		return false, application.ErrCapabilityUnsupported
	}
	editor, ok := any(s.word).(document.WordDocumentProtectionEditor)
	if !ok {
		return false, application.ErrCapabilityUnsupported
	}
	return editor.GetWordDocumentProtection()
}

func (a *desktopSessionAdapter) SetWordDocumentProtection(ctx context.Context, enabled bool) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordDocumentProtectionEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		current, err := editor.GetWordDocumentProtection()
		if err != nil {
			return false, err
		}
		if current == enabled {
			return false, nil
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if err := editor.SetWordDocumentProtection(enabled); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) GetWordPageBackground(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return "", application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.word == nil {
		return "", application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.word).(document.WordPageBackgroundEditor)
	if !ok {
		return "", application.ErrCapabilityUnsupported
	}
	color, err := reader.GetPageBackgroundColor()
	if err != nil {
		return "", err
	}
	return canonicalWordPageBackground(color), nil
}

func (a *desktopSessionAdapter) SetWordPageBackground(ctx context.Context, color string) (bool, error) {
	color = canonicalWordPageBackground(color)
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordPageBackgroundEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		current, err := editor.GetPageBackgroundColor()
		if err != nil {
			return false, err
		}
		if canonicalWordPageBackground(current) == color {
			return false, nil
		}
		if err := editor.SetPageBackgroundColor(color); err != nil {
			return false, err
		}
		return true, nil
	})
}

func canonicalWordPageBackground(color string) string {
	return strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(color), "#"))
}

func (a *desktopSessionAdapter) SetWordLanguage(ctx context.Context, runID, languageCode string) (bool, error) {
	return a.mutateWordPackage(ctx, func(content document.WordContentReader) error {
		blocks := content.WordBlocks()
		if runID != "" {
			if run, ok := selectedWordRun(blocks, runID); !ok || !run.Editable {
				return application.ErrWordTextRunUnavailable
			}
		} else {
			hasEditableText := false
			for _, paragraph := range allWordParagraphs(blocks) {
				for _, inline := range paragraph.Inlines {
					if inline.Text != nil && inline.Text.Editable {
						hasEditableText = true
						break
					}
				}
				if hasEditableText {
					break
				}
			}
			if !hasEditableText {
				return nil
			}
		}
		editor, ok := content.(document.WordLanguageEditor)
		if !ok {
			return application.ErrCapabilityUnsupported
		}
		return editor.SetLanguage(runID, languageCode)
	})
}

func (a *desktopSessionAdapter) mutateWordPageSetup(ctx context.Context, update func(document.WordPageSetupEditor) error) (bool, error) {
	return a.mutateWordPackage(ctx, func(content document.WordContentReader) error {
		editor, ok := content.(document.WordPageSetupEditor)
		if !ok {
			return application.ErrCapabilityUnsupported
		}
		return update(editor)
	})
}

func (a *desktopSessionAdapter) mutateWordPackage(ctx context.Context, update func(document.WordContentReader) error) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		snapshotter, ok := content.(document.WordPackageSnapshotter)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		before, err := snapshotter.Snapshot()
		if err != nil {
			return false, err
		}
		if err := update(content); err != nil {
			return false, err
		}
		after, err := snapshotter.Snapshot()
		if err != nil {
			return false, err
		}
		return !bytes.Equal(before, after), nil
	})
}

func (a *desktopSessionAdapter) InsertWordCoverPage(ctx context.Context, title, subtitle, author string) error {
	_, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordCoverPageEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.InsertCoverPage(title, subtitle, author); err != nil {
			return false, err
		}
		return true, nil
	})
	return err
}

func (a *desktopSessionAdapter) InsertWordPageBreak(ctx context.Context, runID string, start, end int) (string, string, error) {
	return a.InsertWordBreak(ctx, runID, start, end, document.WordBreakPage)
}

func (a *desktopSessionAdapter) InsertWordBreak(ctx context.Context, runID string, start, end int, breakType document.WordBreakType) (string, string, error) {
	var paragraphID, caretRunID string
	_, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		run, ok := selectedWordRun(content.WordBlocks(), runID)
		if !ok || !run.Editable {
			return false, application.ErrWordTextRunUnavailable
		}
		editor, ok := content.(document.WordPageBreakEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		var err error
		paragraphID, caretRunID, err = editor.InsertBreakAtRange(runID, run.Text, start, end, breakType)
		return err == nil, err
	})
	return paragraphID, caretRunID, err
}

func (a *desktopSessionAdapter) InsertWordBlankPage(ctx context.Context, runID string, start, end int) (string, string, error) {
	var paragraphID, caretRunID string
	_, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		run, ok := selectedWordRun(content.WordBlocks(), runID)
		if !ok || !run.Editable {
			return false, application.ErrWordTextRunUnavailable
		}
		editor, ok := content.(document.WordPageBreakEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		var err error
		paragraphID, caretRunID, err = editor.InsertBlankPageAtRange(runID, run.Text, start, end)
		return err == nil, err
	})
	return paragraphID, caretRunID, err
}

func (a *desktopSessionAdapter) InsertWordBookmark(ctx context.Context, runID, name string) error {
	_, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		run, ok := selectedWordRun(content.WordBlocks(), runID)
		if !ok || !run.Editable {
			return false, application.ErrWordTextRunUnavailable
		}
		editor, ok := content.(document.WordBookmarkEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.InsertBookmark(runID, name); err != nil {
			return false, err
		}
		return true, nil
	})
	return err
}

func (a *desktopSessionAdapter) InsertWordHyperlink(ctx context.Context, runID, text, targetURL string) error {
	_, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		run, ok := selectedWordRun(content.WordBlocks(), runID)
		if !ok || !run.Editable {
			return false, application.ErrWordTextRunUnavailable
		}
		editor, ok := content.(document.WordHyperlinkEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.InsertHyperlink(runID, text, targetURL); err != nil {
			return false, err
		}
		return true, nil
	})
	return err
}

func selectedWordBodyParagraph(blocks []document.WordBlock, paragraphID string) (document.WordParagraph, bool) {
	for _, block := range blocks {
		if block.Paragraph != nil && block.Paragraph.ID == paragraphID {
			return *block.Paragraph, true
		}
	}
	return document.WordParagraph{}, false
}

func desktopWordParagraphSpacingUnchanged(paragraph document.WordParagraph, beforeTwips, afterTwips *int64) bool {
	if beforeTwips != nil && (paragraph.Layout.Set&document.WordParagraphBeforeSet == 0 || int64(math.Round(paragraph.Layout.BeforePT*20)) != *beforeTwips) {
		return false
	}
	if afterTwips != nil && (paragraph.Layout.Set&document.WordParagraphAfterSet == 0 || int64(math.Round(paragraph.Layout.AfterPT*20)) != *afterTwips) {
		return false
	}
	return true
}

func (a *desktopSessionAdapter) mutateWordContent(ctx context.Context, mutate func(document.WordContentReader) (bool, error)) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.word == nil {
		return false, application.ErrCapabilityUnsupported
	}
	committed, drafts := s.wordTextStateSnapshot()
	for runID, draft := range drafts {
		if draft != committed[runID] {
			return false, application.ErrPendingEditorDraft
		}
	}
	changed, err := mutate(s.word)
	if err != nil || !changed {
		return changed, err
	}
	blocks := cloneWordBlockSnapshot(s.word.WordBlocks())
	s.wordBlocksMu.Lock()
	s.wordBlocks = blocks
	s.wordBlocksMu.Unlock()
	s.syncWordTextState(blocks, true)
	s.cancelWordLayout()
	return true, nil
}

func desktopWordTextStylePatchUnchanged(style document.WordTextStyle, patch document.WordTextFormatPatch) bool {
	if patch.Bold != nil && style.Bold != *patch.Bold || patch.Italic != nil && style.Italic != *patch.Italic || patch.Underline != nil && style.Underline != *patch.Underline {
		return false
	}
	if patch.FontColor != nil {
		want := strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(*patch.FontColor), "#"))
		got := strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(style.FontColor), "#"))
		if want != got {
			return false
		}
	}
	if patch.Highlight != nil {
		want, got := strings.ToLower(strings.TrimSpace(*patch.Highlight)), strings.ToLower(strings.TrimSpace(style.Highlight))
		if want == "none" {
			want = ""
		}
		if want != got {
			return false
		}
	}
	if patch.FontSize != nil && style.FontSize != *patch.FontSize {
		return false
	}
	return true
}

func (a *desktopSessionAdapter) AppendBodyTextParagraph(ctx context.Context, text string) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return "", "", application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.word == nil {
		return "", "", application.ErrCapabilityUnsupported
	}
	committed, drafts := s.wordTextStateSnapshot()
	for runID, draft := range drafts {
		if draft != committed[runID] {
			return "", "", application.ErrPendingEditorDraft
		}
	}
	editor, ok := any(s.word).(document.WordBodyTextEditor)
	if !ok {
		return "", "", application.ErrCapabilityUnsupported
	}
	paragraphID, runID, err := editor.AppendBodyTextParagraph(text)
	if err != nil {
		return "", "", err
	}
	blocks := cloneWordBlockSnapshot(s.word.WordBlocks())
	s.wordBlocksMu.Lock()
	s.wordBlocks = blocks
	s.wordBlocksMu.Unlock()
	s.syncWordTextState(blocks, true)
	s.cancelWordLayout()
	return paragraphID, runID, nil
}

func (a *desktopSessionAdapter) CommandRevisionChanged(revision uint64) {
	if a != nil && a.runtime != nil && a.session != nil {
		a.runtime.publishCommandRevision(a.session.documentID, revision)
	}
}
func (a *desktopSessionAdapter) Save(ctx context.Context) error {
	_, err := a.SaveWithSnapshot(ctx)
	return err
}
func (a *desktopSessionAdapter) SaveWithSnapshot(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := a.session
	if s.finalized.Load() {
		return nil, errors.New("document session is closed")
	}
	s.adapterMu.RLock()
	saveAsRequired, sourceFormat := s.saveAsRequired, s.sourceFormat
	s.adapterMu.RUnlock()
	if saveAsRequired {
		if sourceFormat == application.FormatDOC {
			return nil, errors.New("旧版 .doc 导入的文档不能覆盖源文件，请另存为 .docx")
		}
		return nil, errors.New("旧版 .xls 导入的工作簿不能覆盖源文件，请另存为 .xlsx")
	}
	if s.pptxDoc != nil && s.hasPendingPresentationDraft() {
		return nil, application.ErrPendingEditorDraft
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	switch {
	case s.book != nil:
		zoomRevision := s.spreadsheetZoomRevision.Load()
		if err := applySpreadsheetSheetZooms(&documentSessionState{sharedDocumentSession: s}); err != nil {
			return nil, err
		}
		err := s.book.Save()
		if err == nil {
			(&documentSessionState{sharedDocumentSession: s}).markSpreadsheetZoomSaved(zoomRevision)
		}
		return nil, err
	case s.word != nil:
		if saver, ok := any(s.word).(interface{ SaveWithSnapshot() ([]byte, error) }); ok {
			return saver.SaveWithSnapshot()
		}
		return nil, s.word.Save()
	case s.pdfDoc != nil:
		return nil, s.pdfDoc.Save()
	case s.markdownDoc != nil:
		return nil, s.markdownDoc.Save()
	case s.jsonDoc != nil:
		return nil, s.jsonDoc.Save()
	case s.pptxDoc != nil:
		return nil, s.pptxDoc.Save()
	default:
		return nil, errors.New("document session has no format adapter")
	}
}

func (a *desktopSessionAdapter) Snapshot(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return nil, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.spreadsheetDraftViews.Load() > 0 {
		return nil, application.ErrPendingEditorDraft
	}
	if s.markdownDraftViews.Load() > 0 || s.markdownCompositionViews.Load() > 0 {
		return nil, application.ErrPendingEditorDraft
	}
	if s.word != nil {
		committed, drafts := s.wordTextStateSnapshot()
		for runID, draft := range drafts {
			if draft != committed[runID] {
				return nil, application.ErrPendingEditorDraft
			}
		}
	}
	switch {
	case s.word != nil:
		return s.word.Snapshot()
	case s.book != nil:
		if err := applySpreadsheetSheetZooms(&documentSessionState{sharedDocumentSession: s}); err != nil {
			return nil, err
		}
		snapshooter, ok := s.book.(interface{ Snapshot() ([]byte, error) })
		if !ok {
			return nil, application.ErrCapabilityUnsupported
		}
		return snapshooter.Snapshot()
	case s.markdownDoc != nil:
		return s.markdownDoc.Snapshot()
	case s.pdfDoc != nil:
		return s.pdfDoc.Snapshot()
	default:
		return nil, application.ErrCapabilityUnsupported
	}
}

func (a *desktopSessionAdapter) HasPendingDrafts() bool {
	s := a.session
	if s == nil || s.finalized.Load() {
		return false
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.spreadsheetDraftViews.Load() > 0 {
		return true
	}
	if s.markdownDraftViews.Load() > 0 || s.markdownCompositionViews.Load() > 0 {
		return true
	}
	if s.word != nil {
		committed, drafts := s.wordTextStateSnapshot()
		for runID, draft := range drafts {
			if draft != committed[runID] {
				return true
			}
		}
	}
	return false
}

func (a *desktopSessionAdapter) SaveAs(ctx context.Context, path string) error {
	_, err := a.SaveAsWithSnapshot(ctx, path)
	return err
}
func (a *desktopSessionAdapter) SaveAsWithSnapshot(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := a.session
	if s.finalized.Load() {
		return nil, errors.New("document session is closed")
	}
	s.adapterMu.RLock()
	saveAsRequired, sourceFormat := s.saveAsRequired, s.sourceFormat
	s.adapterMu.RUnlock()
	if saveAsRequired && sourceFormat == application.FormatDOC && !strings.EqualFold(filepath.Ext(path), ".docx") {
		return nil, errors.New("旧版 .doc 导入的文档只能另存为 .docx")
	}
	if strings.EqualFold(filepath.Ext(path), ".xls") {
		return nil, errors.New("旧版 .xls 导入的工作簿只能另存为 .xlsx")
	}
	if s.pptxDoc != nil && s.hasPendingPresentationDraft() {
		return nil, application.ErrPendingEditorDraft
	}
	s.adapterMu.Lock()
	var snapshot []byte
	var err error
	switch {
	case s.book != nil:
		zoomRevision := s.spreadsheetZoomRevision.Load()
		if err = applySpreadsheetSheetZooms(&documentSessionState{sharedDocumentSession: s}); err != nil {
			break
		}
		if saver, ok := s.book.(interface{ SaveAsDocument(string) error }); ok {
			if snapshoter, ok := s.book.(interface{ Snapshot() ([]byte, error) }); ok {
				snapshot, err = snapshoter.Snapshot()
			}
			if err == nil {
				err = saver.SaveAsDocument(path)
			}
			if err == nil {
				(&documentSessionState{sharedDocumentSession: s}).markSpreadsheetZoomSaved(zoomRevision)
			}
		} else {
			err = application.ErrReadOnlyDocument
		}
	case s.word != nil:
		if saver, ok := any(s.word).(interface {
			SaveAsDocumentWithSnapshot(string) ([]byte, error)
		}); ok {
			snapshot, err = saver.SaveAsDocumentWithSnapshot(path)
		} else {
			err = s.word.SaveAs(path)
		}
	case s.pdfDoc != nil:
		err = s.pdfDoc.SaveAs(path)
	case s.markdownDoc != nil:
		err = s.markdownDoc.SaveAs(path)
	case s.jsonDoc != nil:
		err = s.jsonDoc.SaveAs(path)
	case s.pptxDoc != nil:
		err = s.pptxDoc.SaveAs(path)
	default:
		err = errors.New("document session has no format adapter")
	}
	s.adapterMu.Unlock()
	if err != nil {
		return nil, err
	}
	if a.runtime != nil {
		a.runtime.updateSessionPathFromApplication(s, path)
	}
	return snapshot, nil
}
func (a *desktopSessionAdapter) ApplyPDFCommand(ctx context.Context, command document.PDFCommand) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pdfDoc == nil {
		return false, application.ErrCapabilityUnsupported
	}
	before := s.pdfDoc.Info().Revision
	var err error
	switch command.Kind {
	case "undo":
		s.pdfDoc.Undo()
	case "redo":
		s.pdfDoc.Redo()
	default:
		err = s.pdfDoc.Apply(command)
	}
	if err != nil {
		return false, err
	}
	return s.pdfDoc.Info().Revision != before, nil
}

func (a *desktopSessionAdapter) AddPresentationSlide(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return 0, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return 0, application.ErrCapabilityUnsupported
	}
	if err := s.pptxDoc.AddSlide(); err != nil {
		return 0, err
	}
	return s.pptxDoc.Snapshot().SlideCount, nil
}

func (a *desktopSessionAdapter) DeletePresentationSlide(ctx context.Context, slideID uint32) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return 0, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return 0, application.ErrCapabilityUnsupported
	}
	if err := s.pptxDoc.DeleteSlideByID(slideID); err != nil {
		return 0, err
	}
	return s.pptxDoc.Snapshot().SlideCount, nil
}

func (a *desktopSessionAdapter) DuplicatePresentationSlide(ctx context.Context, slideID uint32) (uint32, int, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return 0, 0, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return 0, 0, application.ErrCapabilityUnsupported
	}
	duplicateID, slideNumber, err := s.pptxDoc.DuplicateSlideByID(slideID)
	if err != nil {
		return 0, 0, err
	}
	return duplicateID, slideNumber, nil
}

func (a *desktopSessionAdapter) AddPresentationTextBox(ctx context.Context, slideID uint32, bounds document.PresentationRect, text string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return 0, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return 0, application.ErrCapabilityUnsupported
	}
	return s.pptxDoc.AddTextBoxByID(slideID, bounds, text)
}

func (a *desktopSessionAdapter) AddPresentationShape(ctx context.Context, slideID uint32, bounds document.PresentationRect, geometry, text string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return 0, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return 0, application.ErrCapabilityUnsupported
	}
	return s.pptxDoc.AddAutoShapeByID(slideID, bounds, geometry, text)
}

func (a *desktopSessionAdapter) AddPresentationPicture(ctx context.Context, slideID uint32, bounds document.PresentationRect, mediaType string, data []byte, fit, altText string, decorative bool) (int64, document.PresentationRect, error) {
	if err := ctx.Err(); err != nil {
		return 0, document.PresentationRect{}, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return 0, document.PresentationRect{}, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return 0, document.PresentationRect{}, application.ErrCapabilityUnsupported
	}
	return s.pptxDoc.AddSlidePictureByID(slideID, bounds, mediaType, data, fit, altText, decorative)
}

func (a *desktopSessionAdapter) AddPresentationAudio(ctx context.Context, slideID uint32, bounds document.PresentationRect, audio document.PresentationAudioData) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return 0, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return 0, application.ErrCapabilityUnsupported
	}
	return s.pptxDoc.AddAudioByID(ctx, slideID, bounds, audio)
}

func (a *desktopSessionAdapter) AddPresentationVideo(ctx context.Context, slideID uint32, bounds document.PresentationRect, video document.PresentationVideoData) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return 0, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return 0, application.ErrCapabilityUnsupported
	}
	return s.pptxDoc.AddVideoByID(ctx, slideID, bounds, video)
}

func (a *desktopSessionAdapter) ReplacePresentationPicture(ctx context.Context, slideID uint32, elementID int64, mediaType string, data []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return false, application.ErrCapabilityUnsupported
	}
	return s.pptxDoc.ReplaceSlidePictureByID(slideID, elementID, mediaType, data)
}

func (a *desktopSessionAdapter) SetPresentationPictureCrop(ctx context.Context, slideID uint32, elementID int64, crop document.PresentationCrop) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return false, application.ErrCapabilityUnsupported
	}
	return s.pptxDoc.UpdateSlidePictureCropByID(slideID, elementID, crop)
}

func (a *desktopSessionAdapter) SetPresentationElementText(ctx context.Context, slideID uint32, elementID int64, text string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	before := s.pptxDoc.Snapshot().Revision
	if err := s.pptxDoc.UpdateSlideTextByID(slideID, elementID, text); err != nil {
		return false, err
	}
	return s.pptxDoc.Snapshot().Revision != before, nil
}

func (a *desktopSessionAdapter) SetPresentationElementStyle(ctx context.Context, slideID uint32, elementID int64, patch document.PresentationElementStylePatch) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	before := s.pptxDoc.Snapshot().Revision
	if err := s.pptxDoc.UpdateSlideElementStyleByID(slideID, elementID, patch); err != nil {
		return false, err
	}
	return s.pptxDoc.Snapshot().Revision != before, nil
}

func (a *desktopSessionAdapter) SetPresentationElementTransform(ctx context.Context, slideID uint32, elementID int64, bounds document.PresentationRect, rotationDeg float64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	before := s.pptxDoc.Snapshot().Revision
	if err := s.pptxDoc.UpdateSlideElementTransformByID(slideID, elementID, bounds, rotationDeg); err != nil {
		return false, err
	}
	return s.pptxDoc.Snapshot().Revision != before, nil
}

func (a *desktopSessionAdapter) SetPresentationSlideNotes(ctx context.Context, slideID uint32, expectedNotes, notes string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	changed, err := s.pptxDoc.UpdateSlideNotesByIDIfCurrent(slideID, expectedNotes, notes)
	if errors.Is(err, pptx.ErrSpeakerNotesConflict) {
		return false, application.ErrPresentationNotesConflict
	}
	return changed, err
}

func (a *desktopSessionAdapter) HasPendingPresentationDraft() bool {
	return a != nil && a.session != nil && a.session.hasPendingPresentationDraft()
}

func (a *desktopSessionAdapter) HasOpenPresentationTextEditor() bool {
	return a != nil && a.session != nil && a.session.hasOpenPresentationTextEditor()
}

func (a *desktopSessionAdapter) Close(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.runtime != nil {
		a.runtime.removeSession(a.session.documentID)
		return nil
	}
	closeSharedDocumentSession(a.session)
	return nil
}

type desktopFormatFactory struct {
	runtime *desktopRuntime
	format  application.Format
}

func (f desktopFormatFactory) Format() application.Format { return f.format }
func (f desktopFormatFactory) Open(ctx context.Context, path string) (application.DocumentAdapter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session, _, err := f.runtime.openDocumentSessionWithStatus(path)
	if err != nil {
		return nil, err
	}
	return newDesktopSessionAdapter(f.runtime, session)
}
func (f desktopFormatFactory) Create(ctx context.Context) (application.DocumentAdapter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	loaded, err := newBlankLoadedDocument(f.format)
	if err != nil {
		return nil, err
	}
	session := f.runtime.registerSessionWithoutApplication(sharedSessionFromLoadedDocument(loaded))
	return newDesktopSessionAdapter(f.runtime, session)
}

func newDesktopSessionAdapter(runtime *desktopRuntime, session *sharedDocumentSession) (*desktopSessionAdapter, error) {
	if session == nil {
		return nil, errors.New("document session is nil")
	}
	format := application.Format("")
	switch {
	case session.sourceFormat == application.FormatCSV:
		format = application.FormatCSV
	case session.book != nil:
		format = application.FormatXLSX
		if !session.book.Editable() {
			format = application.FormatXLS
		}
	case session.word != nil:
		format = application.FormatDocx
	case session.pdfDoc != nil:
		format = application.FormatPDF
	case session.markdownDoc != nil:
		format = application.FormatMarkdown
	case session.jsonDoc != nil:
		format = application.FormatJSON
	case session.pptxDoc != nil:
		format = application.FormatPPTX
	}
	if format == "" {
		return nil, fmt.Errorf("document session %q has no format adapter", session.documentID)
	}
	return &desktopSessionAdapter{runtime: runtime, session: session, format: format}, nil
}

func desktopApplicationFactories(runtime *desktopRuntime) []application.FormatFactory {
	formats := []application.Format{
		application.FormatDocx,
		application.FormatDOC,
		application.FormatXLSX,
		application.FormatXLS,
		application.FormatCSV,
		application.FormatPPTX,
		application.FormatPDF,
		application.FormatJSON,
		application.FormatMarkdown,
	}
	factories := make([]application.FormatFactory, 0, len(formats))
	for _, format := range formats {
		factories = append(factories, desktopFormatFactory{runtime: runtime, format: format})
	}
	return factories
}

func newBlankLoadedDocument(format application.Format) (loadedDocument, error) {
	switch format {
	case application.FormatDocx:
		d, err := docx.NewBlank()
		return loadedDocument{word: d}, err
	case application.FormatXLSX:
		d := xlsx.NewBlank()
		return loadedDocument{book: d, sheets: d.SheetNames()}, nil
	case application.FormatCSV:
		d := csvdoc.NewBlank()
		return loadedDocument{book: d, sheets: d.SheetNames(), sourceFormat: application.FormatCSV}, nil
	case application.FormatPPTX:
		d, err := pptx.NewBlank()
		return loadedDocument{pptxDoc: d}, err
	case application.FormatPDF:
		d, err := pdfdoc.NewBlank()
		return loadedDocument{pdfDoc: d}, err
	case application.FormatJSON:
		return loadedDocument{jsonDoc: jsondoc.NewBlank()}, nil
	case application.FormatMarkdown:
		return loadedDocument{markdownDoc: markdowndoc.NewBlank()}, nil
	case application.FormatXLS:
		return loadedDocument{}, errors.New("旧版 XLS 文件只支持读取")
	default:
		return loadedDocument{}, fmt.Errorf("unsupported document format %q", format)
	}
}

func (r *desktopRuntime) openLiveDocument(path string) (*sharedDocumentSession, bool, error) {
	if r.applicationRuntime == nil {
		return nil, false, errors.New("application runtime is unavailable")
	}
	r.mu.Lock()
	previous := r.findSessionByPathLocked(path)
	r.mu.Unlock()
	info, err := r.applicationRuntime.OpenDocument(context.Background(), path)
	if err != nil {
		return nil, false, err
	}
	session, ok := r.session(info.DocumentID)
	if !ok {
		return nil, false, fmt.Errorf("application document %q has no desktop session", info.DocumentID)
	}
	return session, previous == nil, nil
}

func (a *desktopSessionAdapter) HasPendingPresentationCropDraft() bool {
	return a != nil && a.session != nil && a.session.hasPendingPresentationCropDraft()
}
