// Package wordpdf renders a neutral Word document snapshot as a PDF file.
package wordpdf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"glance/internal/document"
	"glance/internal/format/docx"
	pdfformat "glance/internal/format/pdf"
	"glance/internal/wordfont"
	"glance/internal/wordlayout"
)

const maxReturnedDiagnostics = 1000

type Diagnostic struct {
	Severity string `json:"severity"`
	SourceID string `json:"source_id"`
	Code     string `json:"code"`
	Detail   string `json:"detail"`
}

type Result struct {
	Path                 string       `json:"path"`
	BytesWritten         int64        `json:"bytes_written"`
	PageCount            int          `json:"page_count"`
	Diagnostics          []Diagnostic `json:"diagnostics"`
	DiagnosticsTruncated bool         `json:"diagnostics_truncated"`
}

// CommitGuard acquires a short-lived lock after rendering and before the new
// output path is linked. Its release function runs after the link attempt.
type CommitGuard func() (release func(), err error)

type imageSource struct{ document *docx.Document }

func (s imageSource) ImageBytes(imageID string) ([]byte, error) {
	return s.document.WordImageBytes(imageID)
}

// ExportSnapshot renders a complete DOCX snapshot, validates the generated
// PDF, and creates path without replacing an existing file.
func ExportSnapshot(ctx context.Context, path string, snapshot []byte, documentID string, revision uint64, orientation string, guard CommitGuard) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(path) == "" {
		return Result{}, errors.New("Word PDF destination path is empty")
	}
	if len(snapshot) == 0 {
		return Result{}, errors.New("Word PDF snapshot is empty")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	word, err := docx.OpenSnapshot(snapshot)
	if err != nil {
		return Result{}, fmt.Errorf("open Word snapshot for PDF export: %w", err)
	}
	defer word.Close()

	systemShaper := struct {
		shaper *wordfont.Shaper
		err    error
	}{}
	systemShaper.shaper, systemShaper.err = wordfont.NewSystem()
	if systemShaper.err != nil {
		return Result{}, systemShaper.err
	}
	blocks := word.WordBlocks()
	layoutData := word.WordLayoutData()
	defaultPageSetup := wordlayout.DefaultPageSetup()
	if orientation != "" && orientation != "document" {
		var ok bool
		layoutData, defaultPageSetup, ok = withOrientation(layoutData, defaultPageSetup, orientation)
		if !ok {
			return Result{}, fmt.Errorf("unsupported Word PDF orientation %q", orientation)
		}
	}
	layout, err := wordlayout.NewEngine(systemShaper.shaper).Layout(ctx, wordlayout.Input{
		DocumentID: documentID, Revision: revision, Blocks: blocks,
		Layout: layoutData, DefaultPageSetup: defaultPageSetup,
	})
	if err != nil {
		return Result{}, fmt.Errorf("lay out Word snapshot for PDF export: %w", err)
	}

	return ExportScene(ctx, path, layout, systemShaper.shaper, imageSource{document: word}, guard)
}

// ExportScene validates a prepared page scene and atomically creates a new
// destination path without replacing an existing file.
func ExportScene(ctx context.Context, path string, layout wordlayout.Result, fonts pdfformat.FontSource, images pdfformat.ImageSource, guard CommitGuard) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(path) == "" {
		return Result{}, errors.New("Word PDF destination path is empty")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	directory := filepath.Dir(path)
	stage, err := os.CreateTemp(directory, ".glance-word-pdf-stage-*.pdf")
	if err != nil {
		return Result{}, fmt.Errorf("create Word PDF staging file: %w", err)
	}
	stagePath := stage.Name()
	if err := stage.Close(); err != nil {
		_ = os.Remove(stagePath)
		return Result{}, fmt.Errorf("close Word PDF staging file: %w", err)
	}
	defer os.Remove(stagePath)

	if err := pdfformat.ExportWordPDFFile(ctx, stagePath, layout, fonts, images); err != nil {
		return Result{}, err
	}
	info, err := os.Stat(stagePath)
	if err != nil {
		return Result{}, fmt.Errorf("inspect generated Word PDF: %w", err)
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

	diagnostics := make([]Diagnostic, 0, min(len(layout.Diagnostics), maxReturnedDiagnostics))
	for _, diagnostic := range layout.Diagnostics[:min(len(layout.Diagnostics), maxReturnedDiagnostics)] {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: string(diagnostic.Severity), SourceID: diagnostic.SourceID,
			Code: diagnostic.Code, Detail: diagnostic.Detail,
		})
	}
	return Result{
		Path: path, BytesWritten: info.Size(), PageCount: len(layout.Pages),
		Diagnostics: diagnostics, DiagnosticsTruncated: len(layout.Diagnostics) > len(diagnostics),
	}, nil
}

func withOrientation(layout document.WordLayoutData, defaultSetup document.WordPageSetup, orientation string) (document.WordLayoutData, document.WordPageSetup, bool) {
	orientation = strings.ToLower(strings.TrimSpace(orientation))
	if orientation != "portrait" && orientation != "landscape" {
		return layout, defaultSetup, false
	}
	width, height := 595.28, 841.89
	if orientation == "landscape" {
		width, height = height, width
	}
	defaultSetup.WidthPT, defaultSetup.HeightPT, defaultSetup.Orientation = width, height, orientation
	defaultSetup.Set |= document.WordPageWidthSet | document.WordPageHeightSet | document.WordPageOrientationSet
	if len(layout.Sections) > 0 {
		layout.Sections = append([]document.WordSection(nil), layout.Sections...)
		section := &layout.Sections[0]
		section.PageSetup.Orientation = orientation
		section.PageSetup.Set |= document.WordPageOrientationSet
		if orientation == "landscape" && section.PageSetup.WidthPT < section.PageSetup.HeightPT {
			section.PageSetup.WidthPT, section.PageSetup.HeightPT = section.PageSetup.HeightPT, section.PageSetup.WidthPT
		} else if orientation == "portrait" && section.PageSetup.WidthPT > section.PageSetup.HeightPT {
			section.PageSetup.WidthPT, section.PageSetup.HeightPT = section.PageSetup.HeightPT, section.PageSetup.WidthPT
		}
	}
	return layout, defaultSetup, true
}
