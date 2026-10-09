package pdf

import (
	"context"
	"errors"
	"fmt"
	"glance/internal/document"
	"glance/internal/fileutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/SalvioniDigitalSolutions/gopdf"
)

// ExportSpreadsheetPDFFile writes a complete spreadsheet PDF beside path,
// validates it, and atomically replaces path.
func ExportSpreadsheetPDFFile(ctx context.Context, path string, window document.SheetWindow, options SpreadsheetPDFOptions, fonts FontSource, images SpreadsheetImageSource) error {
	return ExportSpreadsheetPDFFileWithCommitGuard(ctx, path, window, options, fonts, images, nil)
}

// ExportSpreadsheetPDFFileWithCommitGuard creates a temporary PDF, validates it,
// executes guard(), and replaces the destination path atomically.
func ExportSpreadsheetPDFFileWithCommitGuard(ctx context.Context, path string, window document.SheetWindow, options SpreadsheetPDFOptions, fonts FontSource, images SpreadsheetImageSource, guard func() (func(), error)) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("PDF destination path is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".spreadsheet-pdf-*.pdf")
	if err != nil {
		return fmt.Errorf("create temporary PDF beside destination: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := WriteSpreadsheetPDF(ctx, temporary, window, options, fonts, images); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary PDF: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary PDF: %w", err)
	}

	reader, err := gopdf.Open(temporaryPath)
	if err != nil {
		return fmt.Errorf("validate generated PDF: %w", err)
	}
	if reader.Repaired() {
		return errors.New("validate generated PDF: reader had to repair the file")
	}
	if reader.NumPages() < 1 {
		return errors.New("validate generated PDF: output has no pages")
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	var release func()
	if guard != nil {
		var err error
		release, err = guard()
		if err != nil {
			return err
		}
		if release != nil {
			defer release()
		}
	}
	if err := fileutil.ReplaceFile(temporaryPath, path); err != nil {
		return fmt.Errorf("replace PDF destination: %w", err)
	}
	return nil
}
