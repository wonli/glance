// Package sheetpdf exports a bounded neutral spreadsheet window as a validated PDF.
package sheetpdf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/SalvioniDigitalSolutions/gopdf"

	"glance/internal/document"
	pdfformat "glance/internal/format/pdf"
	"glance/internal/wordfont"
)

const (
	maxRows      = 5000
	maxColumns   = 100
	maxCellCount = 500_000
)

type Result struct {
	Path         string `json:"path"`
	BytesWritten int64  `json:"bytes_written"`
	PageCount    int    `json:"page_count"`
}

type CommitGuard = func() (release func(), err error)

type ImageSource interface {
	SpreadsheetImages(sheet string) ([]document.SpreadsheetImage, error)
	SpreadsheetImageBytes(imageID string) ([]byte, error)
}

var systemFont struct {
	once   sync.Once
	shaper *wordfont.Shaper
	err    error
}

// ExportWindow renders an explicit bounded sheet window and atomically creates
// a new PDF path. It never replaces an existing destination.
func ExportWindow(ctx context.Context, path string, sheet string, window document.SheetWindow, orientation string, images ImageSource, guard CommitGuard) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return Result{}, errors.New("spreadsheet PDF destination path is empty")
	}
	if !strings.EqualFold(filepath.Ext(path), ".pdf") {
		return Result{}, errors.New("spreadsheet PDF destination must use .pdf extension")
	}
	sheet = strings.TrimSpace(sheet)
	if sheet == "" {
		return Result{}, errors.New("spreadsheet PDF sheet name is empty")
	}
	if err := validateWindow(window); err != nil {
		return Result{}, err
	}
	orientation = strings.ToLower(strings.TrimSpace(orientation))
	if orientation == "" {
		orientation = "portrait"
	}
	if orientation != "portrait" && orientation != "landscape" {
		return Result{}, fmt.Errorf("unsupported spreadsheet PDF orientation %q", orientation)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	systemFont.once.Do(func() {
		systemFont.shaper, systemFont.err = wordfont.NewSystem()
	})
	if systemFont.err != nil {
		return Result{}, fmt.Errorf("load spreadsheet PDF fonts: %w", systemFont.err)
	}

	directory := filepath.Dir(path)
	stage, err := os.CreateTemp(directory, ".glance-sheet-pdf-stage-*.pdf")
	if err != nil {
		return Result{}, fmt.Errorf("create spreadsheet PDF staging file: %w", err)
	}
	stagePath := stage.Name()
	if err := stage.Close(); err != nil {
		_ = os.Remove(stagePath)
		return Result{}, fmt.Errorf("close spreadsheet PDF staging file: %w", err)
	}
	defer os.Remove(stagePath)

	options := pdfformat.SpreadsheetPDFOptions{
		SheetName: sheet, Orientation: orientation, FitToPageWidth: true,
	}
	if err := pdfformat.ExportSpreadsheetPDFFile(ctx, stagePath, window, options, systemFont.shaper, images); err != nil {
		return Result{}, err
	}
	reader, err := gopdf.Open(stagePath)
	if err != nil {
		return Result{}, fmt.Errorf("validate spreadsheet PDF: %w", err)
	}
	if reader.Repaired() || reader.NumPages() < 1 {
		return Result{}, errors.New("validate spreadsheet PDF: generated file is repaired or has no pages")
	}
	info, err := os.Stat(stagePath)
	if err != nil {
		return Result{}, fmt.Errorf("inspect generated spreadsheet PDF: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	var release func()
	if guard != nil {
		release, err = guard()
		if err != nil {
			return Result{}, err
		}
		if release != nil {
			defer release()
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := os.Link(stagePath, path); err != nil {
		return Result{}, err
	}
	return Result{Path: path, BytesWritten: info.Size(), PageCount: reader.NumPages()}, nil
}

func validateWindow(window document.SheetWindow) error {
	if window.StartRow < 1 || window.StartColumn < 1 {
		return errors.New("spreadsheet PDF window has invalid origin")
	}
	rows := len(window.Cells)
	if rows < 1 || rows > maxRows {
		return fmt.Errorf("spreadsheet PDF row count must be from 1 to %d", maxRows)
	}
	columns := len(window.Cells[0])
	if columns < 1 || columns > maxColumns || rows*columns > maxCellCount {
		return fmt.Errorf("spreadsheet PDF window exceeds %d rows, %d columns, or %d cells", maxRows, maxColumns, maxCellCount)
	}
	for row, cells := range window.Cells {
		if len(cells) != columns {
			return fmt.Errorf("spreadsheet PDF row %d has inconsistent column count", row+1)
		}
	}
	if len(window.RowHeightsPoints) != 0 && len(window.RowHeightsPoints) != rows {
		return errors.New("spreadsheet PDF row geometry does not match the cell window")
	}
	if len(window.ColumnWidthsChars) != 0 && len(window.ColumnWidthsChars) != columns {
		return errors.New("spreadsheet PDF column geometry does not match the cell window")
	}
	return nil
}
