package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"
)

func runtimePDFUtilityBytes(t *testing.T, widths ...float64) []byte {
	t.Helper()
	doc := gopdf.New()
	for index, width := range widths {
		page := doc.AddPageSize(gopdf.PageSize{W: width, H: 100})
		page.SetFont(gopdf.Helvetica, 12)
		page.Text(20, 30, fmt.Sprintf("runtime-page-%d", index+1))
	}
	var output bytes.Buffer
	if _, err := doc.WriteTo(&output); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func writeRuntimePDFUtilityFixture(t *testing.T, path string, widths ...float64) {
	t.Helper()
	if err := os.WriteFile(path, runtimePDFUtilityBytes(t, widths...), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimePDFUtilityInspectMergeSplit(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	dir := t.TempDir()
	first := filepath.Join(dir, "first.pdf")
	second := filepath.Join(dir, "second.pdf")
	writeRuntimePDFUtilityFixture(t, first, 100, 110)
	writeRuntimePDFUtilityFixture(t, second, 200)

	opened, err := runtime.OpenDocument(ctx, first)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	before, err := runtime.GetDocument(ctx, opened.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before utility operation: %v", err)
	}
	if info, err := runtime.InspectPDFUtilityFile(ctx, first); err != nil || info.PageCount != 2 || info.SizeBytes <= 0 {
		t.Fatalf("InspectPDFUtilityFile() = %+v, %v", info, err)
	}
	mergedPath := filepath.Join(dir, "merged.pdf")
	merged, err := runtime.MergePDFUtilityFiles(ctx, []string{first, second}, mergedPath)
	if err != nil {
		t.Fatalf("MergePDFUtilityFiles: %v", err)
	}
	if merged.PageCount != 3 || merged.Path != mergedPath || merged.SizeBytes <= 0 {
		t.Fatalf("merge output = %+v", merged)
	}
	reader, err := gopdf.Open(mergedPath)
	if err != nil || reader.NumPages() != 3 {
		t.Fatalf("reopen merged output: reader=%v err=%v", reader, err)
	}
	for index, width := range []float64{100, 110, 200} {
		size, err := reader.PageSize(index)
		if err != nil || size.W != width {
			t.Fatalf("merged page %d width = %.2f, %v; want %.2f", index+1, size.W, err, width)
		}
	}

	ranges, err := ParsePDFUtilityPageRanges("2, 1", 2)
	if err != nil || len(ranges) != 2 || ranges[0] != (PDFUtilityPageRange{Start: 2, End: 2}) {
		t.Fatalf("ParsePDFUtilityPageRanges() = %+v, %v", ranges, err)
	}
	if _, err := ParsePDFUtilityPageRanges("1, 1", 2); err == nil {
		t.Fatal("ParsePDFUtilityPageRanges accepted overlapping pages")
	}
	split, err := runtime.SplitPDFUtilityFile(ctx, first, dir, "range", PDFUtilityModeRanges, ranges)
	if err != nil || len(split) != 2 || split[0].PageCount != 1 || split[1].PageCount != 1 {
		t.Fatalf("range split = %+v, %v", split, err)
	}
	byPage, err := runtime.SplitPDFUtilityFile(ctx, second, dir, "single", PDFUtilityModeByPage, nil)
	if err != nil || len(byPage) != 1 || byPage[0].PageCount != 1 {
		t.Fatalf("page split = %+v, %v", byPage, err)
	}
	if _, err := runtime.SplitPDFUtilityFile(ctx, first, dir, "unknown", PDFUtilityMode("other"), nil); err == nil {
		t.Fatal("SplitPDFUtilityFile accepted an unknown mode")
	}

	partialDir := filepath.Join(dir, "partial")
	if err := os.Mkdir(partialDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(partialDir, "part-02.pdf"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	partial, err := runtime.SplitPDFUtilityFile(ctx, first, partialDir, "part", PDFUtilityModeByPage, nil)
	if err == nil || len(partial) != 1 || filepath.Base(partial[0].Path) != "part-01.pdf" {
		t.Fatalf("partial split = %+v, %v", partial, err)
	}

	after, err := runtime.GetDocument(ctx, opened.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument after utility operation: %v", err)
	}
	if after.DocumentID != before.DocumentID || after.SourcePath != before.SourcePath || after.Revision != before.Revision || after.Dirty != before.Dirty {
		t.Fatalf("utility operation changed open session: before=%+v after=%+v", before, after)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := runtime.InspectPDFUtilityFile(cancelled, first); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled inspect error = %v", err)
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := runtime.InspectPDFUtilityFile(ctx, first); !errors.Is(err, ErrRuntimeClosed) {
		t.Fatalf("inspect after close = %v, want ErrRuntimeClosed", err)
	}
}
