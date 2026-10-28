package pdfdoc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"
)

func utilityTestPDF(t testing.TB, widths ...float64) []byte {
	t.Helper()
	doc := gopdf.New()
	for index, width := range widths {
		page := doc.AddPageSize(gopdf.PageSize{W: width, H: 100})
		page.SetFont(gopdf.Helvetica, 12)
		page.Text(20, 30, fmt.Sprintf("page-%d", index+1))
	}
	var data bytes.Buffer
	if _, err := doc.WriteTo(&data); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func utilityTestPDFWithImage(t testing.TB) []byte {
	t.Helper()
	doc := gopdf.New()
	page := doc.AddPageSize(gopdf.PageSize{W: 100, H: 100})
	pixels := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			pixels.SetRGBA(x, y, color.RGBA{R: 220, G: 70, B: 35, A: 255})
		}
	}
	img, err := doc.AddImage(pixels)
	if err != nil {
		t.Fatal(err)
	}
	page.DrawImage(img, 10, 10, 40, 40)
	var data bytes.Buffer
	if _, err := doc.WriteTo(&data); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func writeUtilityTestPDF(t testing.TB, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertUtilityPageWidths(t testing.TB, path string, want []float64) {
	t.Helper()
	reader, err := gopdf.Open(path)
	if err != nil {
		t.Fatalf("open output %q: %v", path, err)
	}
	if got := reader.NumPages(); got != len(want) {
		t.Fatalf("output %q has %d pages, want %d", path, got, len(want))
	}
	for index, wantWidth := range want {
		size, err := reader.PageSize(index)
		if err != nil {
			t.Fatalf("page %d size: %v", index+1, err)
		}
		if math.Abs(size.W-wantWidth) > 0.01 {
			t.Errorf("page %d width = %.2f, want %.2f", index+1, size.W, wantWidth)
		}
	}
}

func TestPDFUtilityInspectReadsMetadata(t *testing.T) {
	path := writeUtilityTestPDF(t, t.TempDir(), "source.pdf", utilityTestPDF(t, 100, 200, 300))
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := InspectUtilityPDF(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != path || got.PageCount != 3 || got.SizeBytes != stat.Size() {
		t.Fatalf("InspectUtilityPDF() = %+v, want path %q, 3 pages, %d bytes", got, path, stat.Size())
	}
}

func TestPDFUtilityMergePreservesInputOrder(t *testing.T) {
	dir := t.TempDir()
	first := writeUtilityTestPDF(t, dir, "first.pdf", utilityTestPDF(t, 110, 120))
	second := writeUtilityTestPDF(t, dir, "second.pdf", utilityTestPDF(t, 210, 220))
	firstBefore, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBefore, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "merged.pdf")
	got, err := MergeUtilityPDFs(context.Background(), []string{first, second}, target)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != target || got.PageCount != 4 || got.SizeBytes <= 0 {
		t.Fatalf("MergeUtilityPDFs() = %+v", got)
	}
	assertUtilityPageWidths(t, target, []float64{110, 120, 210, 220})
	for index, source := range []string{first, second} {
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		want := [][]byte{firstBefore, secondBefore}[index]
		if !bytes.Equal(data, want) {
			t.Fatalf("source %q changed during merge", source)
		}
	}
}

func TestPDFUtilityMergeKeepsMappedImageResourcesAlive(t *testing.T) {
	const helperEnv = "GLANCE_PDF_UTILITY_IMAGE_STREAM_HELPER"
	if os.Getenv(helperEnv) == "1" {
		dir := t.TempDir()
		source := writeUtilityTestPDF(t, dir, "source.pdf", utilityTestPDFWithImage(t))
		target := filepath.Join(dir, "merged.pdf")
		if _, err := MergeUtilityPDFs(context.Background(), []string{source, source}, target); err != nil {
			t.Fatalf("MergeUtilityPDFs: %v", err)
		}
		reader, err := gopdf.Open(target)
		if err != nil {
			t.Fatalf("open merged PDF: %v", err)
		}
		images, err := reader.PageImages(0)
		if err != nil || len(images) != 1 {
			t.Fatalf("merged page images = %d, %v; want one image", len(images), err)
		}
		decoded, err := images[0].Decode()
		if err != nil || decoded.Bounds().Dx() != 8 || decoded.Bounds().Dy() != 8 {
			t.Fatalf("merged image decode = %v, %v; want 8x8 image", decoded, err)
		}
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestPDFUtilityMergeKeepsMappedImageResourcesAlive$")
	command.Env = append(os.Environ(), helperEnv+"=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("isolated image-stream merge failed: %v\n%s", err, output)
	}
}

func TestPDFUtilitySplitByRangesWritesOneFilePerRange(t *testing.T) {
	dir := t.TempDir()
	source := writeUtilityTestPDF(t, dir, "source.pdf", utilityTestPDF(t, 100, 110, 120, 130))
	got, err := SplitUtilityPDFByRanges(context.Background(), source, dir, "range", []PageRange{{Start: 1, End: 2}, {Start: 4, End: 4}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d outputs, want 2", len(got))
	}
	if filepath.Base(got[0].Path) != "range-01.pdf" || filepath.Base(got[1].Path) != "range-02.pdf" {
		t.Fatalf("output names = %q, %q", got[0].Path, got[1].Path)
	}
	if got[0].PageCount != 2 || got[1].PageCount != 1 {
		t.Fatalf("output page counts = %d, %d", got[0].PageCount, got[1].PageCount)
	}
	assertUtilityPageWidths(t, got[0].Path, []float64{100, 110})
	assertUtilityPageWidths(t, got[1].Path, []float64{130})
}

func TestPDFUtilitySplitByPageWritesSinglePageFiles(t *testing.T) {
	dir := t.TempDir()
	source := writeUtilityTestPDF(t, dir, "source.pdf", utilityTestPDF(t, 100, 110, 120))
	got, err := SplitUtilityPDFByPage(context.Background(), source, dir, "each")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d outputs, want 3", len(got))
	}
	for index, width := range []float64{100, 110, 120} {
		wantName := fmt.Sprintf("each-%02d.pdf", index+1)
		if filepath.Base(got[index].Path) != wantName || got[index].PageCount != 1 {
			t.Errorf("output %d = %+v, want %s with one page", index, got[index], wantName)
		}
		assertUtilityPageWidths(t, got[index].Path, []float64{width})
	}
}

func TestPDFUtilityRejectsUnsupportedInputAndCleansStaging(t *testing.T) {
	dir := t.TempDir()
	plain := utilityTestPDF(t, 100)
	encryptedDoc := gopdf.New()
	encryptedDoc.AddPage()
	encryptedDoc.Encrypt("user", "owner", gopdf.AllowPrint, gopdf.AES256)
	var encrypted bytes.Buffer
	if _, err := encryptedDoc.WriteTo(&encrypted); err != nil {
		t.Fatal(err)
	}
	brokenXref := bytes.Replace(plain, []byte("startxref\n"), []byte("startxref\n0\n"), 1)
	inputs := map[string][]byte{
		"encrypted.pdf": encrypted.Bytes(),
		"repaired.pdf":  brokenXref,
		"truncated.pdf": plain[:len(plain)/2],
	}
	for name, data := range inputs {
		t.Run(name, func(t *testing.T) {
			path := writeUtilityTestPDF(t, dir, name, data)
			if name == "repaired.pdf" {
				reader, err := gopdf.Open(path)
				if err != nil || !reader.Repaired() {
					t.Fatalf("repaired fixture was not recognized as repaired: reader=%v err=%v", reader, err)
				}
			}
			if _, err := InspectUtilityPDF(context.Background(), path); err == nil {
				t.Fatalf("InspectUtilityPDF(%q) succeeded for unsupported input", name)
			}
			if _, err := MergeUtilityPDFs(context.Background(), []string{path, path}, filepath.Join(dir, "should-not-exist-"+name)); err == nil {
				t.Fatalf("MergeUtilityPDFs(%q) succeeded for unsupported input", name)
			}
			assertNoPDFUtilityStagingFiles(t, dir)
		})
	}
}

func TestPDFUtilityRejectsSourceAndExistingTargets(t *testing.T) {
	dir := t.TempDir()
	source := writeUtilityTestPDF(t, dir, "source.pdf", utilityTestPDF(t, 100, 110))
	if _, err := MergeUtilityPDFs(context.Background(), nil, filepath.Join(dir, "none.pdf")); err == nil {
		t.Fatal("merge accepted zero input PDFs")
	}
	if _, err := MergeUtilityPDFs(context.Background(), []string{source}, filepath.Join(dir, "one.pdf")); err == nil {
		t.Fatal("merge accepted one input PDF")
	}
	if _, err := MergeUtilityPDFs(context.Background(), []string{source, source}, source); err == nil {
		t.Fatal("merge accepted its source path as the target")
	}
	if _, err := SplitUtilityPDFByRanges(context.Background(), source, dir, "bad", []PageRange{{Start: 1, End: 3}}); err == nil {
		t.Fatal("split accepted a range beyond the source page count")
	}
	if err := os.WriteFile(filepath.Join(dir, "present.pdf"), []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeUtilityPDFs(context.Background(), []string{source, source}, filepath.Join(dir, "present.pdf")); err == nil {
		t.Fatal("merge replaced an existing target")
	}
	if got, err := os.ReadFile(filepath.Join(dir, "present.pdf")); err != nil || string(got) != "keep me" {
		t.Fatalf("existing target changed: data=%q err=%v", got, err)
	}
}

func TestPDFUtilityReportsDestinationFailureAndCleansStaging(t *testing.T) {
	dir := t.TempDir()
	source := writeUtilityTestPDF(t, dir, "source.pdf", utilityTestPDF(t, 100, 110))
	fileAsDirectory := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(fileAsDirectory, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(fileAsDirectory, "out.pdf")
	if _, err := MergeUtilityPDFs(context.Background(), []string{source, source}, target); err == nil {
		t.Fatal("merge unexpectedly succeeded in a file path")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("destination failure left an output target")
	}
	assertNoPDFUtilityStagingFiles(t, dir)
}

func TestPDFUtilitySplitReportsPartialCommitAndCleansStaging(t *testing.T) {
	dir := t.TempDir()
	source := writeUtilityTestPDF(t, dir, "source.pdf", utilityTestPDF(t, 100, 110, 120))
	if err := os.WriteFile(filepath.Join(dir, "split-02.pdf"), []byte("keep existing"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := SplitUtilityPDFByPage(context.Background(), source, dir, "split")
	if err == nil {
		t.Fatal("split succeeded despite an existing second target")
	}
	if len(got) != 1 || filepath.Base(got[0].Path) != "split-01.pdf" {
		t.Fatalf("partial outputs = %+v, want only split-01.pdf", got)
	}
	if data, readErr := os.ReadFile(filepath.Join(dir, "split-02.pdf")); readErr != nil || string(data) != "keep existing" {
		t.Fatalf("existing second target changed: data=%q err=%v", data, readErr)
	}
	assertNoPDFUtilityStagingFiles(t, dir)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if outputs, cancelErr := SplitUtilityPDFByPage(cancelled, source, dir, "cancelled"); !errors.Is(cancelErr, context.Canceled) || len(outputs) != 0 {
		t.Fatalf("cancelled split = outputs %v, error %v", outputs, cancelErr)
	}
	assertNoPDFUtilityStagingFiles(t, dir)
}

func assertNoPDFUtilityStagingFiles(t testing.TB, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".glance-pdf-utility-") {
			t.Errorf("staging file leaked: %s", entry.Name())
		}
	}
}

func BenchmarkPDFUtilitySplitByPage(b *testing.B) {
	dir := b.TempDir()
	widths := make([]float64, 100)
	for index := range widths {
		widths[index] = 100 + float64(index)
	}
	source := writeUtilityTestPDF(b, dir, "benchmark.pdf", utilityTestPDF(b, widths...))
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		outputDir := filepath.Join(dir, fmt.Sprintf("run-%d", index))
		if err := os.Mkdir(outputDir, 0700); err != nil {
			b.Fatal(err)
		}
		if outputs, err := SplitUtilityPDFByPage(context.Background(), source, outputDir, "page"); err != nil || len(outputs) != 100 {
			b.Fatalf("split outputs=%d err=%v", len(outputs), err)
		}
	}
}
