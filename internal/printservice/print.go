// Package printservice exposes the native print boundary for in-memory PDF data.
package printservice

import (
	"context"
	"errors"
	"fmt"

	"github.com/SalvioniDigitalSolutions/gopdf"
)

var (
	ErrUnavailable = errors.New("native printing is unavailable on this platform")
	ErrCanceled    = errors.New("native print operation was canceled")
)

// Options describes the requested physical print job. Page indexes are
// one-based; a zero/zero range means all pages.
type Options struct {
	Copies    int
	FirstPage int
	LastPage  int
}

// Printer consumes the exact PDF generated from the shared Word page scene.
type Printer interface {
	Available() bool
	Print(context.Context, []byte, Options) error
}

func normalizeOptions(options Options, pageCount int) (Options, error) {
	if pageCount <= 0 {
		return Options{}, errors.New("PDF has no pages to print")
	}
	if options.Copies == 0 {
		options.Copies = 1
	}
	if options.Copies < 1 {
		return Options{}, errors.New("print copies must be at least one")
	}
	if options.FirstPage == 0 && options.LastPage == 0 {
		return options, nil
	}
	if options.FirstPage < 1 || options.LastPage < 1 || options.FirstPage > options.LastPage || options.LastPage > pageCount {
		return Options{}, fmt.Errorf("invalid one-based page range %d-%d for %d-page PDF", options.FirstPage, options.LastPage, pageCount)
	}
	return options, nil
}

func validatePDF(data []byte) (int, error) {
	reader, err := gopdf.NewReader(data)
	if err != nil {
		return 0, fmt.Errorf("read PDF for printing: %w", err)
	}
	if reader.IsEncrypted() {
		return 0, errors.New("encrypted PDF cannot be printed")
	}
	if reader.NumPages() < 1 {
		return 0, errors.New("PDF has no pages to print")
	}
	return reader.NumPages(), nil
}
