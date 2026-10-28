package application

import (
	"context"
	"errors"
	"fmt"

	"glance/internal/format/pdfdoc"
)

// PDFUtilityPageRange is a 1-based inclusive page range for the PDF workbench.
type PDFUtilityPageRange struct {
	Start int
	End   int
}

// PDFUtilityFileInfo is the format-neutral metadata shown for a local PDF.
type PDFUtilityFileInfo struct {
	Path      string
	PageCount int
	SizeBytes int64
}

// PDFUtilityOutput describes a committed output PDF.
type PDFUtilityOutput struct {
	Path      string
	PageCount int
	SizeBytes int64
}

// PDFUtilityMode selects range-based or one-file-per-page splitting.
type PDFUtilityMode string

const (
	PDFUtilityModeRanges PDFUtilityMode = "ranges"
	PDFUtilityModeByPage PDFUtilityMode = "by_page"
)

// ParsePDFUtilityPageRanges exposes the adapter's page range grammar without
// leaking format-library types into the UI layer.
func ParsePDFUtilityPageRanges(input string, pageCount int) ([]PDFUtilityPageRange, error) {
	parsed, err := pdfdoc.ParsePageRanges(input, pageCount)
	if err != nil {
		return nil, err
	}
	out := make([]PDFUtilityPageRange, len(parsed))
	for index, pageRange := range parsed {
		out[index] = PDFUtilityPageRange{Start: pageRange.Start, End: pageRange.End}
	}
	return out, nil
}

// InspectPDFUtilityFile returns metadata for a locally readable PDF.
func (r *Runtime) InspectPDFUtilityFile(ctx context.Context, path string) (PDFUtilityFileInfo, error) {
	if err := r.checkPDFUtilityRuntime(ctx); err != nil {
		return PDFUtilityFileInfo{}, err
	}
	info, err := pdfdoc.InspectUtilityPDF(ctx, path)
	if err != nil {
		return PDFUtilityFileInfo{}, err
	}
	return PDFUtilityFileInfo{Path: info.Path, PageCount: info.PageCount, SizeBytes: info.SizeBytes}, nil
}

// MergePDFUtilityFiles merges local PDFs in the supplied order into a new file.
func (r *Runtime) MergePDFUtilityFiles(ctx context.Context, sources []string, targetPath string) (PDFUtilityOutput, error) {
	if err := r.checkPDFUtilityRuntime(ctx); err != nil {
		return PDFUtilityOutput{}, err
	}
	output, err := pdfdoc.MergeUtilityPDFs(ctx, append([]string(nil), sources...), targetPath)
	if err != nil {
		return PDFUtilityOutput{}, err
	}
	return PDFUtilityOutput{Path: output.Path, PageCount: output.PageCount, SizeBytes: output.SizeBytes}, nil
}

// SplitPDFUtilityFile writes range outputs or one output per source page.
// Partial output metadata is returned if a later output fails to commit.
func (r *Runtime) SplitPDFUtilityFile(ctx context.Context, sourcePath, outputDir, prefix string, mode PDFUtilityMode, ranges []PDFUtilityPageRange) ([]PDFUtilityOutput, error) {
	if err := r.checkPDFUtilityRuntime(ctx); err != nil {
		return nil, err
	}
	var (
		outputs []pdfdoc.UtilityOutput
		err     error
	)
	switch mode {
	case PDFUtilityModeRanges:
		adapterRanges := make([]pdfdoc.PageRange, len(ranges))
		for index, pageRange := range ranges {
			adapterRanges[index] = pdfdoc.PageRange{Start: pageRange.Start, End: pageRange.End}
		}
		outputs, err = pdfdoc.SplitUtilityPDFByRanges(ctx, sourcePath, outputDir, prefix, adapterRanges)
	case PDFUtilityModeByPage:
		outputs, err = pdfdoc.SplitUtilityPDFByPage(ctx, sourcePath, outputDir, prefix)
	default:
		return nil, fmt.Errorf("unknown PDF split mode %q", mode)
	}
	neutral := make([]PDFUtilityOutput, len(outputs))
	for index, output := range outputs {
		neutral[index] = PDFUtilityOutput{Path: output.Path, PageCount: output.PageCount, SizeBytes: output.SizeBytes}
	}
	return neutral, err
}

func (r *Runtime) checkPDFUtilityRuntime(ctx context.Context) error {
	if r == nil {
		return ErrRuntimeClosed
	}
	if ctx == nil {
		return errors.New("PDF utility context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return ErrRuntimeClosed
	}
	return nil
}
