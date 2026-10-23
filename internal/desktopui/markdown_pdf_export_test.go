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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SalvioniDigitalSolutions/gopdf"
	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	wordpdf "glance/internal/format/pdf"
	"glance/internal/nativewebview"
	"glance/internal/wordlayout"
)

func TestMarkdownVegaPDFUsesSettledInteractiveSnapshot(t *testing.T) {
	source := "```vega\n{\"mark\":\"point\"}\n```\n"
	snapshot, err := markdowndoc.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	span := markdowndoc.DiagramSpan(snapshot.Blocks[0])
	resourceKey := markdowndoc.ResourceKey(span)
	initial := image.NewUniform(color.NRGBA{R: 20, A: 255})
	settled := image.NewRGBA(image.Rect(0, 0, 400, 200))
	settled.Set(30, 40, color.NRGBA{G: 220, A: 255})
	order := make(chan string, 4)
	session := &markdownChartTestSession{events: make(chan nativewebview.PointerEvent, 4), closed: make(chan struct{}), image: settled, order: order}
	view := newMarkdownView(source)
	view.snapshot = snapshot
	view.parseGeneration = 1
	state := &markdownChartState{key: markdownChartIdentity(span), resourceKey: resourceKey, sourceStart: span.SourceStart, source: source, generation: 1, image: initial, logicalSize: image.Pt(200, 100), session: session, ctx: context.Background(), events: []nativewebview.PointerEvent{{Kind: nativewebview.PointerDown}, {Kind: nativewebview.PointerUp}}}
	view.charts[state.key] = state
	resources := map[string]image.Image{resourceKey: initial}
	frozen, err := freezeMarkdownChartResources(context.Background(), view, source, 1, resources)
	if err != nil {
		t.Fatal(err)
	}
	if resources[resourceKey] != initial {
		t.Fatal("freezing mutated the preview resource map")
	}
	if frozen[resourceKey] == initial || document.ResourceLogicalSize(frozen[resourceKey]) != image.Pt(200, 100) {
		t.Fatal("PDF resources did not receive the settled chart snapshot and logical dimensions")
	}
	if got := []string{<-order, <-order, <-order, <-order}; strings.Join(got, ",") != "dispatch,dispatch,dispatch,snapshot" {
		t.Fatalf("chart export sequence = %v; want queued input, pointer leave, then snapshot", got)
	}
	for _, want := range []nativewebview.PointerEventKind{nativewebview.PointerDown, nativewebview.PointerUp, nativewebview.PointerLeave} {
		if got := <-session.events; got.Kind != want {
			t.Fatalf("chart export event = %+v; want %q", got, want)
		}
	}
	prepared, err := prepareMarkdownPDF(context.Background(), snapshot, frozen, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := wordpdf.WriteWordPDF(context.Background(), io.Discard, prepared.scene, prepared.shaper, prepared.images); err != nil {
		t.Fatalf("prepared PDF still depended on the closed chart session: %v", err)
	}
}

func TestMarkdownPDFKeepsDuplicateVegaSnapshotsSeparate(t *testing.T) {
	spec := `{"mark":"point"}`
	source := "```vega\n" + spec + "\n```\n\n```vega\n" + spec + "\n```\n"
	snapshot, err := markdowndoc.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	firstSpan := markdowndoc.DiagramSpan(snapshot.Blocks[0])
	secondSpan := markdowndoc.DiagramSpan(snapshot.Blocks[1])
	firstKey := markdowndoc.ResourceKey(firstSpan)
	secondKey := markdowndoc.ResourceKey(secondSpan)
	if firstKey == secondKey {
		t.Fatalf("identical chart occurrences share PDF resource key %q", firstKey)
	}
	firstSettled := image.NewUniform(color.NRGBA{R: 220, A: 255})
	secondSettled := image.NewUniform(color.NRGBA{B: 220, A: 255})
	view := newMarkdownView(source)
	view.snapshot = snapshot
	view.parseGeneration = 1
	view.charts[markdownChartIdentity(firstSpan)] = &markdownChartState{
		key: markdownChartIdentity(firstSpan), resourceKey: firstKey, sourceStart: firstSpan.SourceStart,
		source: source, generation: 1, session: &markdownChartTestSession{image: firstSettled}, ctx: context.Background(),
	}
	view.charts[markdownChartIdentity(secondSpan)] = &markdownChartState{
		key: markdownChartIdentity(secondSpan), resourceKey: secondKey, sourceStart: secondSpan.SourceStart,
		source: source, generation: 1, session: &markdownChartTestSession{image: secondSettled}, ctx: context.Background(),
	}
	frozen, err := freezeMarkdownChartResources(context.Background(), view, source, 1, map[string]image.Image{
		firstKey: image.NewUniform(color.Black), secondKey: image.NewUniform(color.Black),
	})
	if err != nil {
		t.Fatal(err)
	}
	if frozen[firstKey] == frozen[secondKey] {
		t.Fatal("duplicate chart occurrences were collapsed to one PDF snapshot")
	}
	if got := color.NRGBAModel.Convert(frozen[firstKey].At(0, 0)).(color.NRGBA); got.R < 200 || got.B != 0 {
		t.Fatalf("first chart snapshot = %v, want red", got)
	}
	if got := color.NRGBAModel.Convert(frozen[secondKey].At(0, 0)).(color.NRGBA); got.B < 200 || got.R != 0 {
		t.Fatalf("second chart snapshot = %v, want blue", got)
	}
}

func TestMarkdownVegaPDFKeepsStaticSnapshotForUnopenedChart(t *testing.T) {
	source := "```vega-lite\n{\"mark\":\"bar\"}\n```\n"
	snapshot, err := markdowndoc.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	span := markdowndoc.DiagramSpan(snapshot.Blocks[0])
	resourceKey := markdowndoc.ResourceKey(span)
	initial := image.NewUniform(color.NRGBA{B: 180, A: 255})
	resources := map[string]image.Image{resourceKey: initial}
	view := newMarkdownView(source)
	view.charts["stale"] = &markdownChartState{key: "stale", resourceKey: resourceKey, source: source, generation: 1, image: image.NewUniform(color.NRGBA{R: 255, A: 255})}
	frozen, err := freezeMarkdownChartResources(context.Background(), view, source, 0, resources)
	if err != nil {
		t.Fatal(err)
	}
	if frozen[resourceKey] != initial {
		t.Fatal("unopened chart did not retain its offline static resource image")
	}
}

func TestMarkdownPDFPreflightFreezesChartSession(t *testing.T) {
	a := markdownTestApp(t)
	view := a.markdownView
	source := "```vega\n{\"mark\":\"point\"}\n```\n"
	parsed, err := markdowndoc.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	span := markdowndoc.DiagramSpan(parsed.Blocks[0])
	resourceKey := markdowndoc.ResourceKey(span)
	initial := image.NewUniform(color.NRGBA{R: 30, A: 255})
	settled := image.NewRGBA(image.Rect(0, 0, 400, 200))
	settled.Set(30, 40, color.NRGBA{G: 220, A: 255})
	session := &markdownChartTestSession{events: make(chan nativewebview.PointerEvent, 2), closed: make(chan struct{}), image: settled}
	state := &markdownChartState{key: markdownChartIdentity(span), resourceKey: resourceKey, sourceStart: span.SourceStart, source: source, generation: 1, image: initial, logicalSize: image.Pt(200, 100), session: session, ctx: context.Background()}
	view.editor.SetText(source)
	a.flushMarkdownDraft()
	view.snapshot = parsed
	view.parseGeneration = 1
	view.parsedPath = a.path
	view.images = map[string]image.Image{resourceKey: initial}
	view.errors = map[string]string{}
	view.charts[state.key] = state
	a.requestMarkdownPDFExport()
	var result markdownFileResult
	select {
	case result = <-view.exports:
	case <-time.After(5 * time.Second):
		t.Fatal("Markdown PDF preflight timed out")
	}
	if result.err != nil || result.prepared == nil {
		t.Fatalf("Markdown PDF preflight result=%v", result.err)
	}
	data, ok := result.prepared.images[resourceKey]
	if !ok {
		t.Fatal("preflight did not carry the chart snapshot into frozen PDF images")
	}
	frozen, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(frozen.At(30, 40)).(color.NRGBA); got.G < 200 {
		t.Fatalf("preflight encoded the static chart image instead of the settled frame: %v", got)
	}
	if got := <-session.events; got.Kind != nativewebview.PointerLeave {
		t.Fatalf("PDF preflight did not clear chart hover: %+v", got)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := wordpdf.WriteWordPDF(context.Background(), io.Discard, result.prepared.scene, result.prepared.shaper, result.prepared.images); err != nil {
		t.Fatalf("frozen preflight depended on the closed chart session: %v", err)
	}
	a.stopAndWaitManagedBackground()
}

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

func TestMarkdownPDFExportUsesBundledBitmapFallbacks(t *testing.T) {
	snapshot, err := markdowndoc.Parse("Check ✓, ✅, 🙂 and 📊. Sequences 👩🏽‍💻 👨‍👩‍👧‍👦 🇨🇳 1️⃣ ⚠️\n\n| Symbol | Emoji |\n| --- | --- |\n| ☑ | 🧠 |\n")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareMarkdownPDF(context.Background(), snapshot, nil, nil)
	if err != nil {
		t.Fatalf("prepare Markdown PDF with unsupported system glyphs: %v", err)
	}
	if len(prepared.images) < 6 {
		t.Fatalf("embedded bitmap resources=%d; want at least 6", len(prepared.images))
	}
	placements := 0
	for _, page := range prepared.scene.Pages {
		placements += len(page.Images)
	}
	if placements < 6 {
		t.Fatalf("PDF image placements=%d; want at least 6", placements)
	}
	var output bytes.Buffer
	if err := wordpdf.WriteWordPDF(context.Background(), &output, prepared.scene, prepared.shaper, prepared.images); err != nil {
		t.Fatalf("serialize Markdown PDF with bundled emoji: %v", err)
	}
	reader, err := gopdf.NewReader(output.Bytes())
	if err != nil || reader.NumPages() == 0 || reader.Repaired() {
		t.Fatalf("invalid emoji PDF: reader=%v err=%v", reader, err)
	}
	rendered, _, err := reader.RenderPageDetail(0, gopdf.RenderOpts{DPI: 96, IncludeRasterImages: true, IncludeVector: true})
	if err != nil {
		t.Fatal(err)
	}
	colored := 0
	for y := rendered.Bounds().Min.Y; y < rendered.Bounds().Max.Y; y++ {
		for x := rendered.Bounds().Min.X; x < rendered.Bounds().Max.X; x++ {
			pixel := color.NRGBAModel.Convert(rendered.At(x, y)).(color.NRGBA)
			if max(pixel.R, pixel.G, pixel.B)-min(pixel.R, pixel.G, pixel.B) > 50 {
				colored++
			}
		}
	}
	if colored < 100 {
		t.Fatalf("rendered color-glyph pixels=%d; bundled emoji images are not visible", colored)
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
