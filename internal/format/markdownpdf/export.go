// Package markdownpdf exports neutral Markdown snapshots to validated PDF files.
package markdownpdf

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"

	"glance/internal/document"
	"glance/internal/fontmanager/fontcatalog"
	"glance/internal/format/markdowndoc"
	pdfformat "glance/internal/format/pdf"
	"glance/internal/format/wordpdf"
	"glance/internal/wordfont"
	"glance/internal/wordlayout"
)

const (
	maxSnapshotBytes          = 8 << 20
	maxReturnedResourceErrors = 100
)

type Result struct {
	Path                      string               `json:"path"`
	BytesWritten              int64                `json:"bytes_written"`
	PageCount                 int                  `json:"page_count"`
	MissingImageCount         int                  `json:"missing_image_count"`
	ResourceWarnings          []string             `json:"resource_warnings"`
	ResourceWarningsTruncated bool                 `json:"resource_warnings_truncated"`
	Diagnostics               []wordpdf.Diagnostic `json:"diagnostics"`
	DiagnosticsTruncated      bool                 `json:"diagnostics_truncated"`
}

type CommitGuard = wordpdf.CommitGuard

// ExportSnapshot parses, renders, validates, and exclusively creates a PDF
// from the supplied exact Markdown source. Unavailable images become explicit
// placeholders and are reported in the result.
func ExportSnapshot(ctx context.Context, path string, source []byte, sourcePath, documentID string, revision uint64, orientation string, guard CommitGuard) (Result, error) {
	shaper, err := wordfont.NewSystem()
	if err != nil {
		return Result{}, err
	}
	return exportSnapshotWithShaper(ctx, path, source, sourcePath, documentID, revision, orientation, guard, shaper)
}

// ExportSnapshotWithFontSnapshot uses the runtime's captured font catalog for
// Markdown parsing, layout, and PDF text resources.
func ExportSnapshotWithFontSnapshot(ctx context.Context, path string, source []byte, sourcePath, documentID string, revision uint64, orientation string, fonts fontcatalog.Snapshot, guard CommitGuard) (Result, error) {
	shaper, err := wordfont.NewWithSnapshot(fonts)
	if err != nil {
		return Result{}, err
	}
	return exportSnapshotWithShaper(ctx, path, source, sourcePath, documentID, revision, orientation, guard, shaper)
}

func exportSnapshotWithShaper(ctx context.Context, path string, source []byte, sourcePath, documentID string, revision uint64, orientation string, guard CommitGuard, shaper *wordfont.Shaper) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(path) == "" {
		return Result{}, errors.New("Markdown PDF destination path is empty")
	}
	if len(source) == 0 {
		return Result{}, errors.New("Markdown PDF source is empty")
	}
	if len(source) > maxSnapshotBytes {
		return Result{}, fmt.Errorf("Markdown PDF source exceeds %d bytes", maxSnapshotBytes)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	snapshot, err := markdowndoc.Parse(string(source))
	if err != nil {
		return Result{}, fmt.Errorf("parse Markdown for PDF export: %w", err)
	}
	resources, resourceErrors := markdowndoc.PreviewResourcesContext(ctx, snapshot, sourcePath)
	blocks, images, err := pdfformat.MarkdownWordBlocks(snapshot, resources, resourceErrors)
	if err != nil {
		return Result{}, err
	}
	missingImages, warnings := missingImageWarnings(snapshot, resources)

	pageSetup := wordlayout.DefaultPageSetup()
	switch strings.ToLower(strings.TrimSpace(orientation)) {
	case "", "portrait":
	case "landscape":
		pageSetup.WidthPT, pageSetup.HeightPT = pageSetup.HeightPT, pageSetup.WidthPT
		pageSetup.Orientation = "landscape"
		pageSetup.Set |= document.WordPageWidthSet | document.WordPageHeightSet | document.WordPageOrientationSet
	default:
		return Result{}, fmt.Errorf("unsupported Markdown PDF orientation %q", orientation)
	}
	layout, err := wordlayout.NewEngine(shaper).Layout(ctx, wordlayout.Input{
		DocumentID: documentID, Revision: revision, Blocks: blocks, DefaultPageSetup: pageSetup,
	})
	if err != nil {
		return Result{}, fmt.Errorf("lay out Markdown snapshot for PDF export: %w", err)
	}
	for _, diagnostic := range layout.Diagnostics {
		if diagnostic.Severity == wordlayout.DiagnosticError {
			return Result{}, fmt.Errorf("Markdown PDF: %s", diagnostic.Detail)
		}
	}
	pdfResult, err := wordpdf.ExportScene(ctx, path, layout, shaper, images, guard)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Path: pdfResult.Path, BytesWritten: pdfResult.BytesWritten, PageCount: pdfResult.PageCount,
		MissingImageCount: missingImages, ResourceWarnings: warnings,
		ResourceWarningsTruncated: missingImages > len(warnings),
		Diagnostics:               pdfResult.Diagnostics, DiagnosticsTruncated: pdfResult.DiagnosticsTruncated,
	}, nil
}

func missingImageWarnings(snapshot document.MarkdownSnapshot, resources map[string]image.Image) (int, []string) {
	count := 0
	warnings := make([]string, 0, min(8, maxReturnedResourceErrors))
	visit := func(spans []document.MarkdownSpan) {
		for _, span := range spans {
			if (span.Kind != "image" && span.Kind != "diagram") || resources[markdowndoc.ResourceKey(span)] != nil {
				continue
			}
			count++
			if len(warnings) >= maxReturnedResourceErrors {
				continue
			}
			label := strings.TrimSpace(span.Text)
			if label == "" {
				label = "未命名图片"
			}
			warnings = append(warnings, fmt.Sprintf("图片“%s”不可用；PDF 中保留了占位符。", boundedRunes(label, 160)))
		}
	}
	for _, block := range snapshot.Blocks {
		if block.Kind == "diagram" {
			visit([]document.MarkdownSpan{markdowndoc.DiagramSpan(block)})
		}
		visit(block.Spans)
		for _, row := range block.TableSpans {
			for _, cell := range row {
				visit(cell)
			}
		}
	}
	return count, warnings
}

func boundedRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}
