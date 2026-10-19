package desktopui

import (
	"bytes"
	"context"
	"errors"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"
	"glance/internal/format/markdowndoc"
	wordpdf "glance/internal/format/pdf"
	"glance/internal/wordlayout"
)

func TestMarkdownPDFExport(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{R: 200, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.png"), data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := markdowndoc.Parse("# 中文标题\n\n中文正文 ~~删除内容~~ $x^2$\n\n![图片](a.png)\n\n- 一级列表\n  - 二级列表\n    - 三级列表\n\n> 第一层引用\n>\n> > 第二层引用\n\n|名称|居中|值|\n|:---|:---:|---:|\n|中文|中间文字|42|\n|![表格图片](a.png)|居中|$x^2$|\n\n$$\\frac{a}{b}$$\n\n```text\n$x$ ~~literal~~\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	resources, diagnostics := markdowndoc.PreviewResources(snapshot, filepath.Join(dir, "source.md"))
	blocks, images, err := wordpdf.MarkdownWordBlocks(snapshot, resources, diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	shaper, err := newWordTextShaper()
	if err != nil {
		t.Fatal(err)
	}
	scene, err := wordlayout.NewEngine(shaper).Layout(context.Background(), wordlayout.Input{Blocks: blocks, DefaultPageSetup: defaultWordPageSetup})
	if err != nil {
		t.Fatal(err)
	}
	placements, cells, strikeFragments := 0, 0, 0
	for _, page := range scene.Pages {
		placements += len(page.Images)
		cells += len(page.TableCells)
		for _, fragment := range page.Text {
			if fragment.Style.Strike {
				strikeFragments++
			}
		}
	}
	if placements != 5 || len(images) != 3 || cells != 9 {
		t.Fatalf("images=%d resources=%d table cells=%d", placements, len(images), cells)
	}
	if strikeFragments == 0 {
		t.Fatal("strike did not propagate through shared pagination")
	}
	path := filepath.Join(dir, "out.pdf")
	if err := exportMarkdownPDF(context.Background(), snapshot, filepath.Join(dir, "source.md"), path, nil); err != nil {
		t.Fatal(err)
	}
	reader, err := gopdf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if reader.NumPages() < 1 || reader.Repaired() {
		t.Fatal("invalid PDF")
	}
	text, err := reader.PageText(0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(text), []byte("中文")) || !bytes.Contains([]byte(text), []byte("42")) {
		t.Fatalf("PDF text = %q", text)
	}
	if capture := os.Getenv("GLANCE_MARKDOWN_PDF_CAPTURE"); capture != "" {
		if err := os.MkdirAll(filepath.Dir(capture), 0700); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(capture, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMarkdownPDFExportFailurePreservesDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.pdf")
	old := []byte("original output")
	os.WriteFile(path, old, 0600)
	snapshot, _ := markdowndoc.Parse("中文正文")
	if err := exportMarkdownPDF(context.Background(), snapshot, "source.md", path, func() (func(), error) { return nil, errors.New("stale revision") }); err == nil {
		t.Fatal("expected guard failure")
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(old, got) {
		t.Fatal("guard failure replaced destination")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := exportMarkdownPDF(ctx, snapshot, "source.md", path, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	snapshot, _ = markdowndoc.Parse("$\\unsupported{x}$")
	if err := exportMarkdownPDF(context.Background(), snapshot, "source.md", path, nil); err == nil {
		t.Fatal("expected unsupported formula failure")
	}
	got, _ = os.ReadFile(path)
	if !bytes.Equal(old, got) {
		t.Fatal("failure replaced destination")
	}
	snapshot, _ = markdowndoc.Parse("|公式|\n|---|\n|$\\unsupported{x}$|\n")
	if err := exportMarkdownPDF(context.Background(), snapshot, "source.md", path, nil); err == nil {
		t.Fatal("unsupported table math silently exported")
	}
	got, _ = os.ReadFile(path)
	if !bytes.Equal(old, got) {
		t.Fatal("unsupported table math replaced destination")
	}
}

func TestMarkdownPDFExportPickerCancellationPreservesDraft(t *testing.T) {
	a := markdownTestApp(t)
	v := a.ensureMarkdownView()
	v.editor.SetText("中文未保存")
	a.setWordFileCommandRunner(func(kind fileCommandKind, format string) fileCommandResult {
		if kind != commandExportPDF || format != "pdf" {
			t.Errorf("picker %v %q", kind, format)
		}
		return fileCommandResult{}
	})
	a.requestMarkdownPDFExport()
	preflight := <-v.exports
	v.exports <- preflight
	a.pollMarkdownFileResults()
	a.modal.OnAction("confirm")
	result := <-v.exports
	v.exports <- result
	a.pollMarkdownFileResults()
	if v.exportPending || a.markdownDoc.Text() != "中文未保存" || !a.markdownDoc.Dirty() || a.path != "" {
		t.Fatal("cancel changed source or retained busy flag")
	}
}

func TestMarkdownPDFExportReportsActualPathAndPersistentFailure(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	v.editor.SetText("# PDF 验收\n\n正文")
	a.flushMarkdownDraft()
	path := filepath.Join(t.TempDir(), "导出文件")
	a.setWordFileCommandRunner(func(fileCommandKind, string) fileCommandResult { return fileCommandResult{path: path} })
	a.requestMarkdownPDFExport()
	preflight := <-v.exports
	v.exports <- preflight
	a.pollMarkdownFileResults()
	a.modal.OnAction("confirm")
	picker := <-v.exports
	v.exports <- picker
	a.pollMarkdownFileResults()
	result := <-v.exports
	v.exports <- result
	a.pollMarkdownFileResults()
	written := path + ".pdf"
	data, err := os.ReadFile(written)
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("PDF missing: %v", err)
	}
	if !strings.Contains(a.visibleStatus(), written) {
		t.Fatalf("success has no full path: %q", a.visibleStatus())
	}
	// The footer retains errors after transient toast expiry.
	result.err = errors.New("HTTP 400")
	v.exports <- result
	a.pollMarkdownFileResults()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1200, 34))}
	a.layoutMarkdownStatus(gtx)
	if v.exportMessage.ID != "markdown.pdf.export.error.failed" || !strings.Contains(a.feedback.detail, "HTTP 400") {
		t.Fatalf("export failure should be localized while preserving its diagnostic: message=%+v detail=%q", v.exportMessage, a.feedback.detail)
	}
}

func TestMarkdownExternalPDFExportAcceptance(t *testing.T) {
	path := os.Getenv("GLANCE_MARKDOWN_ACCEPTANCE_SOURCE")
	if path == "" {
		t.Skip("optional user acceptance document")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := markdowndoc.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "acceptance.pdf")
	err = exportMarkdownPDF(context.Background(), p, path, destination, nil)
	if err != nil {
		t.Logf("export failure: %v", err)
		if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
			t.Fatalf("failed export left destination: %v", statErr)
		}
		return
	}
	data, err := os.ReadFile(destination)
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("successful export did not create PDF: %v", err)
	}
	t.Logf("PDF created: %d bytes", len(data))
}

func TestMarkdownPDFUnavailableImagesStillCreateFileAndReportWarning(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(http.StatusBadRequest) }))
	defer server.Close()
	a := markdownTestApp(t)
	v := a.markdownView
	source := "前文 ![封面](" + server.URL + "/image.png) 后文\n\n| 图片 |\n| --- |\n| ![表格图](" + server.URL + "/image.png) |\n"
	v.editor.SetText(source)
	a.flushMarkdownDraft()
	path := filepath.Join(t.TempDir(), "export.pdf")
	var pickerCalls atomic.Int32
	a.setWordFileCommandRunner(func(fileCommandKind, string) fileCommandResult {
		pickerCalls.Add(1)
		return fileCommandResult{path: path}
	})
	a.requestMarkdownPDFExport()
	preflight := <-v.exports
	v.exports <- preflight
	a.pollMarkdownFileResults()
	if !a.modal.Visible || !strings.Contains(a.pdfExportPreview.markdownResult.warnings, "封面") || !strings.Contains(a.pdfExportPreview.markdownResult.warnings, "HTTP 400") {
		t.Fatalf("missing preflight warning: %q", a.messageText(a.modal.MessageRef, a.modal.Message))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("PDF written before confirmation")
	}
	if pickerCalls.Load() != 0 {
		t.Fatal("picker opened before preview confirmation")
	}
	a.modal.OnAction("confirm")
	picker := <-v.exports
	v.exports <- picker
	a.pollMarkdownFileResults()
	result := <-v.exports
	if result.err != nil {
		t.Fatal(result.err)
	}
	v.exports <- result
	a.pollMarkdownFileResults()
	if requests.Load() != 1 {
		t.Fatalf("resource fetched again: %d", requests.Load())
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("missing PDF: %v", err)
	}
	if result.missingImages != 2 || v.lastPDFPath != path || v.exportMessage.ID != "markdown.pdf.export.status.exported_with_warnings" || v.exportMessage.Args["Count"] != 2 {
		t.Fatalf("warning not reported: count=%d message=%q", result.missingImages, v.exportMessage)
	}
	if a.feedback.kind == feedbackError {
		t.Fatal("completed export was reported as failure")
	}
	if v.text() != source || !a.markdownDoc.Dirty() {
		t.Fatal("export changed draft or dirty state")
	}
	reader, err := gopdf.NewReader(data)
	if err != nil {
		t.Fatal(err)
	}
	text, err := reader.PageText(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"图片无法加载", "封面", "表格图", "HTTP 400", "后文"} {
		if !strings.Contains(text, want) {
			t.Fatalf("PDF lost %q: %q", want, text)
		}
	}
}

func TestMarkdownPDFExportRejectsChangedPickerDraft(t *testing.T) {
	a := markdownTestApp(t)
	v := a.ensureMarkdownView()
	v.editor.SetText("原文")
	path := filepath.Join(t.TempDir(), "out.pdf")
	a.setWordFileCommandRunner(func(fileCommandKind, string) fileCommandResult { return fileCommandResult{path: path} })
	a.requestMarkdownPDFExport()
	result := <-v.exports
	v.editor.SetText("修改后")
	v.draftEpoch.Add(1)
	v.exports <- result
	a.pollMarkdownFileResults()
	if v.exportPending {
		t.Fatal("stale picker started export")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale export destination: %v", err)
	}
	if !a.markdownDoc.Dirty() || a.path != "" {
		t.Fatal("stale picker modified source identity")
	}
}

func TestMarkdownPDFExportRejectsSourceAlias(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.md")
	alias := filepath.Join(dir, "source.pdf")
	if err := os.WriteFile(source, []byte("中文"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, alias); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := markdowndoc.Parse("中文")
	if err := exportMarkdownPDF(context.Background(), snapshot, source, alias, nil); err == nil {
		t.Fatal("source hardlink overwritten")
	}
	got, _ := os.ReadFile(source)
	if string(got) != "中文" {
		t.Fatal("source changed")
	}
}

func TestMarkdownPDFExportRejectsConcurrentDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.pdf")
	abs, _ := filepath.Abs(path)
	key := strings.ToLower(filepath.Clean(abs))
	markdownPDFDestinations.Lock()
	markdownPDFDestinations.active[key] = true
	markdownPDFDestinations.Unlock()
	defer func() {
		markdownPDFDestinations.Lock()
		delete(markdownPDFDestinations.active, key)
		markdownPDFDestinations.Unlock()
	}()
	snapshot, _ := markdowndoc.Parse("中文")
	if err := exportMarkdownPDF(context.Background(), snapshot, "", path, nil); err == nil || !strings.Contains(err.Error(), "正在导出") {
		t.Fatalf("concurrent export = %v", err)
	}
}

func TestMarkdownPDFPreviewOrientationAndCancellation(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	v.editor.SetText("# 方向验收\n\n" + strings.Repeat("分页正文。", 300))
	a.flushMarkdownDraft()
	var pickerCalls atomic.Int32
	a.setWordFileCommandRunner(func(fileCommandKind, string) fileCommandResult { pickerCalls.Add(1); return fileCommandResult{} })
	a.requestMarkdownPDFExport()
	preflight := <-v.exports
	v.exports <- preflight
	a.pollMarkdownFileResults()
	if !a.modal.Visible || a.messageText(a.modal.TitleRef, a.modal.Title) != "导出 PDF 预览" || a.pdfExportPreview.Format != "markdown" {
		t.Fatal("missing shared PDF preview")
	}
	a.updateMarkdownPDFOrientation("landscape")
	if !a.pdfExportPreview.wordPreviewPending {
		t.Fatal("orientation did not start background layout")
	}
	result := <-v.exports
	v.exports <- result
	a.pollMarkdownFileResults()
	prev := &a.pdfExportPreview
	page := prev.wordResult.Pages[0]
	if prev.Orientation != "landscape" || page.Clip.WidthPT <= page.Clip.HeightPT || prev.wordPreviewPending {
		t.Fatal("landscape preview not applied")
	}
	if prev.markdownResult.prepared.scene.Pages[0].Clip != page.Clip {
		t.Fatal("export scene differs from preview")
	}
	a.modal.OnAction("cancel")
	if pickerCalls.Load() != 0 || v.exportPending || prev.Active || a.modal.Visible {
		t.Fatal("cancel opened picker or left preview busy")
	}
	if !a.markdownDoc.Dirty() {
		t.Fatal("preview changed dirty state")
	}
}

func TestMarkdownPDFPreflightRejectsFormulaBeforePicker(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	v.editor.SetText("$\\unsupported{x}$")
	var calls atomic.Int32
	a.setWordFileCommandRunner(func(fileCommandKind, string) fileCommandResult { calls.Add(1); return fileCommandResult{} })
	a.requestMarkdownPDFExport()
	result := <-v.exports
	v.exports <- result
	a.pollMarkdownFileResults()
	if result.err == nil || calls.Load() != 0 || v.exportPending {
		t.Fatal("fatal preflight problem reached picker")
	}
}
