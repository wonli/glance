// Package presentationpdf renders the neutral presentation projection to PDF.
package presentationpdf

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"strings"

	"github.com/SalvioniDigitalSolutions/gopdf"

	"glance/internal/document"
	"glance/internal/fileutil"
	"glance/internal/fontmanager/fontcatalog"
	pdfformat "glance/internal/format/pdf"
	"glance/internal/wordfont"
)

const maxReturnedWarnings = 100

type CommitGuard func() (release func(), err error)

type Result struct {
	Path              string   `json:"path"`
	BytesWritten      int64    `json:"bytes_written"`
	PageCount         int      `json:"page_count"`
	Warnings          []string `json:"warnings"`
	WarningsTruncated bool     `json:"warnings_truncated"`
}

func textSource() (*wordfont.Shaper, error) {
	return wordfont.NewSystem()
}

func textSourceWithFontSnapshot(fonts fontcatalog.Snapshot) (*wordfont.Shaper, error) {
	return wordfont.NewWithSnapshot(fonts)
}

func DefaultOrientation(snapshot document.PresentationSnapshot) string {
	if len(snapshot.Slides) > 0 {
		width, height := snapshot.Slides[0].WidthPT, snapshot.Slides[0].HeightPT
		if width <= 0 {
			width = snapshot.WidthPT
		}
		if height <= 0 {
			height = snapshot.HeightPT
		}
		if width > height {
			return "landscape"
		}
	}
	return "portrait"
}

// ExportSnapshot renders and validates a presentation PDF, then exclusively
// creates path. Existing destinations survive every failure and are never
// replaced by this operation.
func ExportSnapshot(ctx context.Context, path string, snapshot document.PresentationSnapshot, orientation string, guard CommitGuard) (Result, error) {
	shaper, err := textSource()
	if err != nil {
		return Result{}, err
	}
	return exportSnapshotWithShaper(ctx, path, snapshot, orientation, guard, shaper)
}

// ExportSnapshotWithFontSnapshot uses font resources captured from the owning
// runtime's immutable catalog snapshot.
func ExportSnapshotWithFontSnapshot(ctx context.Context, path string, snapshot document.PresentationSnapshot, orientation string, fonts fontcatalog.Snapshot, guard CommitGuard) (Result, error) {
	shaper, err := textSourceWithFontSnapshot(fonts)
	if err != nil {
		return Result{}, err
	}
	return exportSnapshotWithShaper(ctx, path, snapshot, orientation, guard, shaper)
}

func exportSnapshotWithShaper(ctx context.Context, path string, snapshot document.PresentationSnapshot, orientation string, guard CommitGuard, shaper *wordfont.Shaper) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(path) == "" {
		return Result{}, errors.New("presentation PDF destination path is empty")
	}
	if len(snapshot.Slides) == 0 || len(snapshot.Slides) != snapshot.SlideCount {
		return Result{}, fmt.Errorf("presentation snapshot has %d projected slides for %d total slides", len(snapshot.Slides), snapshot.SlideCount)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	stage, err := fileutil.CreateTemp(".glance-presentation-pdf-stage-*.pdf")
	if err != nil {
		return Result{}, fmt.Errorf("create presentation PDF staging file: %w", err)
	}
	stagePath := stage.Name()
	defer os.Remove(stagePath)
	if err := pdfformat.WritePresentationPDF(ctx, stage, snapshot, orientation, shaper); err != nil {
		_ = stage.Close()
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		_ = stage.Close()
		return Result{}, err
	}
	if err := stage.Sync(); err != nil {
		_ = stage.Close()
		return Result{}, fmt.Errorf("sync presentation PDF staging file: %w", err)
	}
	if err := stage.Close(); err != nil {
		return Result{}, fmt.Errorf("close presentation PDF staging file: %w", err)
	}
	info, err := os.Stat(stagePath)
	if err != nil {
		return Result{}, fmt.Errorf("inspect generated presentation PDF: %w", err)
	}
	reader, err := gopdf.Open(stagePath)
	if err != nil {
		return Result{}, fmt.Errorf("validate generated presentation PDF: %w", err)
	}
	if reader.Repaired() || reader.NumPages() != len(snapshot.Slides) {
		return Result{}, fmt.Errorf("validate generated presentation PDF: repaired=%t pages=%d want=%d", reader.Repaired(), reader.NumPages(), len(snapshot.Slides))
	}
	wantWidth, wantHeight := 595.28, 841.89
	if strings.EqualFold(strings.TrimSpace(orientation), "landscape") {
		wantWidth, wantHeight = wantHeight, wantWidth
	}
	for index := range snapshot.Slides {
		size, err := reader.PageSize(index)
		if err != nil {
			return Result{}, fmt.Errorf("validate presentation PDF page %d: %w", index+1, err)
		}
		if math.Abs(size.W-wantWidth) > 0.02 || math.Abs(size.H-wantHeight) > 0.02 {
			return Result{}, fmt.Errorf("validate presentation PDF page %d size %.2fx%.2f points; want %.2fx%.2f", index+1, size.W, size.H, wantWidth, wantHeight)
		}
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
	if err := fileutil.MoveExclusive(stagePath, path); err != nil {
		return Result{}, err
	}
	warnings, truncated := presentationWarnings(snapshot)
	return Result{Path: path, BytesWritten: info.Size(), PageCount: len(snapshot.Slides), Warnings: warnings, WarningsTruncated: truncated}, nil
}

// RenderSlidePreview uses the same PDF writer as export, then rasterizes only
// the requested page for the interactive preview.
func RenderSlidePreview(ctx context.Context, snapshot document.PresentationSnapshot, slideIndex int, orientation string) (image.Image, error) {
	shaper, err := textSource()
	if err != nil {
		return nil, err
	}
	return pdfformat.RenderPresentationSlidePreview(ctx, snapshot, slideIndex, orientation, shaper)
}

func RenderSlidePreviewWithFontSnapshot(ctx context.Context, snapshot document.PresentationSnapshot, slideIndex int, orientation string, fonts fontcatalog.Snapshot) (image.Image, error) {
	shaper, err := textSourceWithFontSnapshot(fonts)
	if err != nil {
		return nil, err
	}
	return pdfformat.RenderPresentationSlidePreview(ctx, snapshot, slideIndex, orientation, shaper)
}

func presentationWarnings(snapshot document.PresentationSnapshot) ([]string, bool) {
	warnings := make([]string, 0, 8)
	truncated := false
	visit := func(slide document.PresentationSlide, element document.PresentationElement) {
		if element.Kind != document.ElementKindAudio && element.Kind != document.ElementKindVideo {
			return
		}
		if len(warnings) >= maxReturnedWarnings {
			truncated = true
			return
		}
		label := "音频"
		if element.Kind == document.ElementKindVideo {
			label = "视频"
		}
		warnings = append(warnings, fmt.Sprintf("第 %d 页的%s对象以文字占位符输出，不包含媒体播放内容。", slide.Index+1, label))
	}
	for _, slide := range snapshot.Slides {
		for _, element := range slide.InheritedElements {
			visit(slide, element)
		}
		for _, element := range slide.Elements {
			visit(slide, element)
		}
	}
	return warnings, truncated
}
