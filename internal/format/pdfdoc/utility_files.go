package pdfdoc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/SalvioniDigitalSolutions/gopdf"
	"glance/internal/fileutil"
)

// UtilityFileInfo contains metadata for a PDF used by the merge/split tool.
type UtilityFileInfo struct {
	Path      string
	PageCount int
	SizeBytes int64
}

// UtilityOutput describes a committed PDF produced by the merge/split tool.
type UtilityOutput struct {
	Path      string
	PageCount int
	SizeBytes int64
}

// InspectUtilityPDF reads metadata only for a PDF that the current adapter can
// safely parse. Encrypted and repaired files are rejected by Open.
func InspectUtilityPDF(ctx context.Context, path string) (UtilityFileInfo, error) {
	if err := checkUtilityContext(ctx); err != nil {
		return UtilityFileInfo{}, err
	}
	absolutePath, err := utilityAbsolutePath(path)
	if err != nil {
		return UtilityFileInfo{}, err
	}
	stat, err := os.Stat(absolutePath)
	if err != nil {
		return UtilityFileInfo{}, fmt.Errorf("inspect PDF %q: %w", absolutePath, err)
	}
	if !stat.Mode().IsRegular() {
		return UtilityFileInfo{}, fmt.Errorf("inspect PDF %q: input is not a regular file", absolutePath)
	}
	doc, err := Open(absolutePath)
	if err != nil {
		return UtilityFileInfo{}, fmt.Errorf("inspect PDF %q: %w", absolutePath, err)
	}
	defer doc.Close()
	if err := checkUtilityContext(ctx); err != nil {
		return UtilityFileInfo{}, err
	}
	info := doc.Info()
	if info.Pages < 1 {
		return UtilityFileInfo{}, fmt.Errorf("inspect PDF %q: document has no pages", absolutePath)
	}
	return UtilityFileInfo{Path: absolutePath, PageCount: info.Pages, SizeBytes: stat.Size()}, nil
}

// MergeUtilityPDFs imports all pages from at least two source PDFs in the
// provided order and writes one validated output file without replacing files.
func MergeUtilityPDFs(ctx context.Context, sources []string, targetPath string) (UtilityOutput, error) {
	if err := checkUtilityContext(ctx); err != nil {
		return UtilityOutput{}, err
	}
	if len(sources) < 2 {
		return UtilityOutput{}, errors.New("merge requires at least two PDF files")
	}
	target, err := utilityAbsolutePath(targetPath)
	if err != nil {
		return UtilityOutput{}, err
	}
	absSources := make([]string, len(sources))
	for index, source := range sources {
		absSources[index], err = utilityAbsolutePath(source)
		if err != nil {
			return UtilityOutput{}, fmt.Errorf("source %d: %w", index+1, err)
		}
		if absSources[index] == target {
			return UtilityOutput{}, fmt.Errorf("target %q is also a source PDF", target)
		}
	}
	if err := rejectExistingUtilityTarget(target); err != nil {
		return UtilityOutput{}, fmt.Errorf("prepare PDF output %q: %w", target, err)
	}

	merged := gopdf.New()
	pageCount := 0
	inputs := make([]*Document, 0, len(absSources))
	// Imported resource streams may alias reader bytes; keep mmap-backed inputs alive through serialization.
	defer func() {
		for _, input := range inputs {
			_ = input.Close()
		}
	}()
	for _, source := range absSources {
		if err := checkUtilityContext(ctx); err != nil {
			return UtilityOutput{}, err
		}
		input, err := Open(source)
		if err != nil {
			return UtilityOutput{}, fmt.Errorf("open source PDF %q: %w", source, err)
		}
		inputs = append(inputs, input)
		for pageIndex := 0; pageIndex < input.Info().Pages; pageIndex++ {
			if err := checkUtilityContext(ctx); err != nil {
				return UtilityOutput{}, err
			}
			if _, err := merged.ImportPage(input.reader, pageIndex); err != nil {
				return UtilityOutput{}, fmt.Errorf("import page %d from %q: %w", pageIndex+1, source, err)
			}
			pageCount++
		}
	}
	return writeUtilityPDF(ctx, merged, target, pageCount)
}

// SplitUtilityPDFByRanges writes one PDF for each disjoint, 1-based inclusive
// page range. Outputs are committed in the order of ranges.
func SplitUtilityPDFByRanges(ctx context.Context, sourcePath, outputDir, prefix string, ranges []PageRange) ([]UtilityOutput, error) {
	if err := checkUtilityContext(ctx); err != nil {
		return nil, err
	}
	if len(ranges) == 0 {
		return nil, errors.New("at least one page range is required")
	}
	doc, err := openUtilitySplitSource(ctx, sourcePath)
	if err != nil {
		return nil, err
	}
	defer doc.Close()
	if err := validateUtilityRanges(ranges, doc.Info().Pages); err != nil {
		return nil, err
	}
	return splitUtilityRanges(ctx, doc, outputDir, prefix, ranges)
}

// SplitUtilityPDFByPage writes one validated single-page PDF per source page.
func SplitUtilityPDFByPage(ctx context.Context, sourcePath, outputDir, prefix string) ([]UtilityOutput, error) {
	if err := checkUtilityContext(ctx); err != nil {
		return nil, err
	}
	doc, err := openUtilitySplitSource(ctx, sourcePath)
	if err != nil {
		return nil, err
	}
	defer doc.Close()
	pageCount := doc.Info().Pages
	outputs := make([]UtilityOutput, 0, pageCount)
	for page := 1; page <= pageCount; page++ {
		result, err := splitUtilityRanges(ctx, doc, outputDir, prefix, []PageRange{{Start: page, End: page}}, len(outputs))
		if err != nil {
			return outputs, err
		}
		outputs = append(outputs, result...)
	}
	return outputs, nil
}

func openUtilitySplitSource(ctx context.Context, sourcePath string) (*Document, error) {
	if err := checkUtilityContext(ctx); err != nil {
		return nil, err
	}
	source, err := utilityAbsolutePath(sourcePath)
	if err != nil {
		return nil, err
	}
	doc, err := Open(source)
	if err != nil {
		return nil, fmt.Errorf("open source PDF %q: %w", source, err)
	}
	return doc, nil
}

func splitUtilityRanges(ctx context.Context, source *Document, outputDir, prefix string, ranges []PageRange, offset ...int) ([]UtilityOutput, error) {
	cleanPrefix, err := safeUtilityPrefix(prefix)
	if err != nil {
		return nil, err
	}
	absDir, err := utilityAbsolutePath(outputDir)
	if err != nil {
		return nil, err
	}
	startIndex := 0
	if len(offset) > 0 {
		startIndex = offset[0]
	}
	outputs := make([]UtilityOutput, 0, len(ranges))
	for index, pageRange := range ranges {
		if err := checkUtilityContext(ctx); err != nil {
			return outputs, err
		}
		sequence := startIndex + index + 1
		target := filepath.Join(absDir, fmt.Sprintf("%s-%02d.pdf", cleanPrefix, sequence))
		doc := gopdf.New()
		for page := pageRange.Start; page <= pageRange.End; page++ {
			if err := checkUtilityContext(ctx); err != nil {
				return outputs, err
			}
			if _, err := doc.ImportPage(source.reader, page-1); err != nil {
				return outputs, fmt.Errorf("import page %d for output %q: %w", page, target, err)
			}
		}
		output, err := writeUtilityPDF(ctx, doc, target, pageRange.End-pageRange.Start+1)
		if err != nil {
			return outputs, err
		}
		outputs = append(outputs, output)
	}
	return outputs, nil
}

func writeUtilityPDF(ctx context.Context, doc *gopdf.Document, target string, expectedPages int) (UtilityOutput, error) {
	if err := checkUtilityContext(ctx); err != nil {
		return UtilityOutput{}, err
	}
	if expectedPages < 1 {
		return UtilityOutput{}, fmt.Errorf("write PDF %q: output must contain at least one page", target)
	}
	if err := rejectExistingUtilityTarget(target); err != nil {
		return UtilityOutput{}, fmt.Errorf("prepare PDF output %q: %w", target, err)
	}
	dir := filepath.Dir(target)
	stage, err := os.CreateTemp(dir, ".glance-pdf-utility-*.pdf")
	if err != nil {
		return UtilityOutput{}, fmt.Errorf("create staged PDF beside %q: %w", target, err)
	}
	stagePath := stage.Name()
	defer os.Remove(stagePath)
	if _, err := doc.WriteTo(contextUtilityWriter{ctx: ctx, writer: stage}); err != nil {
		_ = stage.Close()
		return UtilityOutput{}, fmt.Errorf("write staged PDF for %q: %w", target, err)
	}
	if err := checkUtilityContext(ctx); err != nil {
		_ = stage.Close()
		return UtilityOutput{}, err
	}
	if err := stage.Sync(); err != nil {
		_ = stage.Close()
		return UtilityOutput{}, fmt.Errorf("sync staged PDF for %q: %w", target, err)
	}
	if err := stage.Close(); err != nil {
		return UtilityOutput{}, fmt.Errorf("close staged PDF for %q: %w", target, err)
	}
	if err := os.Chmod(stagePath, 0644); err != nil {
		return UtilityOutput{}, fmt.Errorf("set permissions for staged PDF %q: %w", target, err)
	}
	verified, err := Open(stagePath)
	if err != nil {
		return UtilityOutput{}, fmt.Errorf("verify staged PDF for %q: %w", target, err)
	}
	actualPages := verified.Info().Pages
	closeErr := verified.Close()
	if closeErr != nil {
		return UtilityOutput{}, fmt.Errorf("close staged PDF verification for %q: %w", target, closeErr)
	}
	if actualPages != expectedPages {
		return UtilityOutput{}, fmt.Errorf("verify staged PDF for %q: found %d pages, want %d", target, actualPages, expectedPages)
	}
	stat, err := os.Stat(stagePath)
	if err != nil {
		return UtilityOutput{}, fmt.Errorf("stat staged PDF for %q: %w", target, err)
	}
	if err := checkUtilityContext(ctx); err != nil {
		return UtilityOutput{}, err
	}
	if err := fileutil.MoveExclusive(stagePath, target); err != nil {
		return UtilityOutput{}, fmt.Errorf("commit PDF output %q: %w", target, err)
	}
	return UtilityOutput{Path: target, PageCount: actualPages, SizeBytes: stat.Size()}, nil
}

type contextUtilityWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w contextUtilityWriter) Write(data []byte) (int, error) {
	if err := checkUtilityContext(w.ctx); err != nil {
		return 0, err
	}
	return w.writer.Write(data)
}

func validateUtilityRanges(ranges []PageRange, pageCount int) error {
	ordered := append([]PageRange(nil), ranges...)
	for index, pageRange := range ordered {
		if pageRange.Start < 1 || pageRange.End < pageRange.Start || pageRange.End > pageCount {
			return fmt.Errorf("page range %d (%d-%d) is invalid for a %d-page PDF", index+1, pageRange.Start, pageRange.End, pageCount)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Start == ordered[j].Start {
			return ordered[i].End < ordered[j].End
		}
		return ordered[i].Start < ordered[j].Start
	})
	for index := 1; index < len(ordered); index++ {
		if ordered[index].Start <= ordered[index-1].End {
			return fmt.Errorf("page ranges overlap at page %d", ordered[index].Start)
		}
	}
	return nil
}

func safeUtilityPrefix(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "." || prefix == ".." || filepath.Base(prefix) != prefix || strings.ContainsAny(prefix, `/\\`) {
		return "", errors.New("split file prefix must be a non-empty filename")
	}
	for _, r := range prefix {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"|?*`, r) {
			return "", errors.New("split file prefix contains an invalid filename character")
		}
	}
	if strings.HasSuffix(prefix, ".") || strings.HasSuffix(prefix, " ") {
		return "", errors.New("split file prefix cannot end with a dot or space")
	}
	return prefix, nil
}

func rejectExistingUtilityTarget(target string) error {
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("output target %q already exists", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check output target %q: %w", target, err)
	}
	return nil
}

func utilityAbsolutePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("PDF path is empty")
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve PDF path %q: %w", path, err)
	}
	return abs, nil
}

func checkUtilityContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("PDF utility context is nil")
	}
	return ctx.Err()
}
