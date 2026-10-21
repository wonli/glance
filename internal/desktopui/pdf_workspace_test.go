package desktopui

import (
	"bytes"
	gioapp "gioui.org/app"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"github.com/SalvioniDigitalSolutions/gopdf"
	"glance/internal/document"
	"glance/internal/format/pdfdoc"
	"glance/internal/workspace"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func pdfTestBytes(t *testing.T) []byte {
	t.Helper()
	d := gopdf.New()
	for i := 0; i < 4; i++ {
		p := d.AddPage()
		p.SetFont(gopdf.HelveticaBold, 26)
		p.Text(50, 70, "GLANCE PRODUCT REPORT")
		p.SetFont(gopdf.Helvetica, 12)
		p.Text(50, 120, "Growth, experience and opportunities")
		p.SetFillColor(gopdf.RGB(240, 244, 249))
		p.Rect(50, 160, 490, 100, gopdf.Fill)
		p.SetFillColor(gopdf.Black)
		p.Text(65, 195, "Active users +38%       Success rate 92.4%       Satisfaction 4.8/5")
		p.Text(50, 320, "01   PRODUCT PERFORMANCE")
		p.Text(50, 355, "A shared document model powers reading, editing and saving.")
		p.AddNote(450, 340, "Confirm final numbers before publication.", gopdf.NoteOptions{Author: "Reviewer"})
	}
	var b bytes.Buffer
	if _, e := d.WriteTo(&b); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestPDFWorkspaceRecoveryAndSharedModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.PDF")
	if e := os.WriteFile(path, pdfTestBytes(t), 0600); e != nil {
		t.Fatal(e)
	}
	loaded, e := loadDocument(path)
	if e != nil {
		t.Fatal(e)
	}
	s := sharedSessionFromLoadedDocument(loaded)
	if loadedDocumentFromSharedSession(s).pdfDoc != s.pdfDoc {
		t.Fatal("PDF model forked")
	}
	p, _ := s.pdfDoc.Page(0)
	if e = s.pdfDoc.Apply(document.PDFCommand{Kind: "replace", Page: 0, Revision: p.Revision, Run: 0, Text: "UPDATED PRODUCT REPORT"}); e != nil {
		t.Fatal(e)
	}
	state := newDocumentSessionView(s, 1)
	b, e := snapshotState(state)
	if e != nil {
		t.Fatal(e)
	}
	a := New(nil, "")
	defer close(a.shutdown)
	restored, e := a.restoreSessionState(workspace.Session{Tab: workspace.Tab{ID: "pdf", SourcePath: path}, Format: "pdf", Payload: b, Dirty: true})
	if e != nil {
		t.Fatal(e)
	}
	p, e = restored.pdfDoc.Page(0)
	if e != nil || p.Text[0].Text != "UPDATED PRODUCT REPORT" || !sessionDirty(restored) {
		t.Fatalf("restoration lost edits: %+v %v", p, e)
	}
}
func TestPDFStaleSceneCannotReplaceEditedRevision(t *testing.T) {
	v := &pdfViewState{info: document.PDFInfo{Revision: 3}, cache: map[int]pdfPageResult{}, thumbCache: map[int]paint.ImageOp{}}
	v.accept(pdfPageResult{info: document.PDFInfo{Revision: 2}, page: document.PDFPage{Revision: 2}, pixels: image.NewRGBA(image.Rect(0, 0, 10, 10))})
	if v.info.Revision != 3 || len(v.cache) != 0 {
		t.Fatal("old scene replaced current model")
	}
}
func TestPDFPenWorkspaceVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := New(nil, "")
	defer close(a.shutdown)
	data := pdfTestBytes(t)
	if source := os.Getenv("GLANCE_PDF_VISUAL_SOURCE"); source != "" {
		var e error
		data, e = os.ReadFile(source)
		if e != nil {
			t.Fatal(e)
		}
	}
	d, e := pdfdoc.OpenSnapshot(data)
	if e != nil {
		t.Fatal(e)
	}
	a.pdfDoc = d
	v := a.ensurePDFView()
	for i := 0; i < min(4, v.info.Pages); i++ {
		img, p, w, e := d.Render(i, 120)
		if e != nil {
			t.Fatal(e)
		}
		v.accept(pdfPageResult{page: p, pixels: img, warning: w, info: d.Info()})
	}
	for _, width := range []int{1440, 800} {
		size := image.Pt(width, 900)
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
		dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(a.layoutWorkspaceTabs), layout.Flexed(1, a.layoutPDFDocument))
		ops.Reset()
		dims = layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(a.layoutWorkspaceTabs), layout.Flexed(1, a.layoutPDFDocument))
		if dims.Size != size {
			t.Fatalf("layout %v want %v", dims.Size, size)
		}
		win, e := headless.NewWindow(size.X, size.Y)
		if e != nil {
			t.Skip(e)
		}
		if e = win.Frame(&ops); e != nil {
			win.Release()
			t.Fatal(e)
		}
		shot := image.NewRGBA(image.Rectangle{Max: size})
		e = win.Screenshot(shot)
		win.Release()
		if e != nil {
			t.Fatal(e)
		}
		if width == 1440 && os.Getenv("GLANCE_PDF_VISUAL_SOURCE") == "" {
			c := shot.RGBAAt(230, 120)
			if c.R != 231 || c.G != 236 || c.B != 240 {
				t.Fatalf("canvas obscured: %v", c)
			}
			c = shot.RGBAAt(700, 150)
			if c.R < 250 || c.G < 250 || c.B < 250 {
				t.Fatalf("page missing: %v", c)
			}
		}
		if dir := os.Getenv("GLANCE_PDF_CAPTURE"); dir != "" {
			name := "pdf-workspace.png"
			if width == 800 {
				name = "pdf-workspace-small.png"
			}
			f, e := os.Create(filepath.Join(dir, name))
			if e != nil {
				t.Fatal(e)
			}
			e = png.Encode(f, shot)
			f.Close()
			if e != nil {
				t.Fatal(e)
			}
		}
	}
}

func TestPDFSharedRevisionAndSaveRefresh(t *testing.T) {
	d, err := pdfdoc.OpenSnapshot(pdfTestBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	v := &pdfViewState{info: d.Info(), cache: map[int]pdfPageResult{0: {}}, thumbCache: map[int]paint.ImageOp{}, selected: 0}
	if err := d.Apply(document.PDFCommand{Kind: "note", Page: 0, Revision: d.Info().Revision, Text: "shared", X: 40, Y: 40}); err != nil {
		t.Fatal(err)
	}
	v.syncInfo(d.Info())
	if !v.info.Dirty || len(v.cache) != 0 || v.selected != -1 {
		t.Fatal("shared view retained old scene")
	}
	if err := d.SaveAs(filepath.Join(t.TempDir(), "saved.pdf")); err != nil {
		t.Fatal(err)
	}
	v.syncInfo(d.Info())
	if v.info.Dirty {
		t.Fatal("save status stale")
	}
	v.accept(pdfPageResult{info: d.Info(), page: document.PDFPage{Revision: 1}, pixels: image.NewRGBA(image.Rect(0, 0, 1, 1))})
	if len(v.cache) != 0 {
		t.Fatal("mixed-revision render accepted")
	}
}

func TestPDFBusyConfirmRetainsDraft(t *testing.T) {
	a := New(nil, "")
	defer close(a.shutdown)
	d, err := pdfdoc.OpenSnapshot(pdfTestBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	a.pdfDoc = d
	v := a.ensurePDFView()
	v.pending = true
	a.showPDFDraft(document.PDFCommand{Kind: "replace", Revision: d.Info().Revision, Text: "original"})
	a.modal.Input.SetText("中文输入草稿")
	a.modal.OnAction("confirm")
	if !a.modal.Visible || a.modal.Input.Text() != "中文输入草稿" || d.Dirty() {
		t.Fatal("busy confirmation lost draft or changed model")
	}
	a.modal.OnAction("cancel")
	if a.modal.Visible || d.Dirty() {
		t.Fatal("cancel changed document")
	}
}

func TestPDFRibbonIconsResolutionAndSolidGlyphs(t *testing.T) {
	wantIcons := []string{
		"open", "save", "save-as", "thumbnails", "search",
		"previous", "next", "zoom-out", "zoom-in", "fit-width", "fit-page",
		"rotate", "highlight", "note", "ink", "pencil", "undo", "redo",
		"printer", "download",
	}
	for _, name := range wantIcons {
		img, ok := pdfToolbarIcon(name, toolbarGlyphText)
		if !ok || img == nil {
			t.Fatalf("PDF toolbar icon %q failed to load", name)
		}
		if b := img.Bounds(); b.Dx() <= 0 || b.Dy() <= 0 {
			t.Fatalf("PDF toolbar icon %q has zero bounds: %v", name, b)
		}
	}

	for _, action := range pdfToolbarActions {
		if action.id == "close" {
			t.Fatalf("PDF ribbon should not contain close action")
		}
		if action.id == "ink" && action.icon != "pencil" {
			t.Fatalf("PDF ink action should use pencil icon from Pen node zBDLW, got %q", action.icon)
		}
		if action.icon == "" {
			continue
		}
		img, ok := pdfToolbarIcon(action.icon, toolbarGlyphText)
		if !ok || img == nil {
			t.Fatalf("action %q icon %q failed to load", action.id, action.icon)
		}
	}
}

func TestPDFZoomSyncAndInteractions(t *testing.T) {
	a := New(nil, "")
	a.window = &gioapp.Window{}
	defer close(a.shutdown)
	d, err := pdfdoc.OpenSnapshot(pdfTestBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	a.pdfDoc = d
	v := a.ensurePDFView()
	v.cache[0] = pdfPageResult{page: document.PDFPage{Width: 595, Height: 842}}

	// Layout with 1000x800 window
	size := image.Pt(1000, 800)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	gtx := layout.Context{Ops: &ops, Metric: metric, Constraints: layout.Exact(size)}

	// Initial layout: fit is "page"
	a.layoutPDFDocument(gtx)
	initialScale := v.scale
	if initialScale <= 0 {
		t.Fatalf("expected positive scale, got %.3f", initialScale)
	}
	baseScale := float64(metric.PxPerDp) * (4.0 / 3.0)
	expectedFitZoom := initialScale / baseScale
	if math.Abs(v.currentZoom(metric)-expectedFitZoom) > 0.001 {
		t.Fatalf("effective zoom %.3f != expected %.3f", v.currentZoom(metric), expectedFitZoom)
	}
	if math.Abs(v.zoom-expectedFitZoom) > 0.001 {
		t.Fatalf("v.zoom %.3f not synced to fit zoom %.3f", v.zoom, expectedFitZoom)
	}

	// Trigger zoom-in: should step up from effective fit zoom, not from 1.0
	a.handlePDFAction(gtx, "zoom-in")
	if v.fit != "" {
		t.Fatalf("expected fit to be cleared after zoom-in, got %q", v.fit)
	}
	expectedZoomIn := math.Round(min(4, expectedFitZoom+0.1)*100) / 100
	if math.Abs(v.zoom-expectedZoomIn) > 0.01 {
		t.Fatalf("zoom after zoom-in = %.3f; want %.3f (stepped from %.3f)", v.zoom, expectedZoomIn, expectedFitZoom)
	}

	// Re-layout and verify scale matches new manual zoom
	ops.Reset()
	a.layoutPDFDocument(gtx)
	if math.Abs(v.scale-(baseScale*v.zoom)) > 0.001 {
		t.Fatalf("scale = %.3f; want %.3f", v.scale, baseScale*v.zoom)
	}

	// Trigger zoom-out
	expectedZoomOut := math.Round(max(0.25, v.zoom-0.1)*100) / 100
	a.handlePDFAction(gtx, "zoom-out")
	ops.Reset()
	a.layoutPDFDocument(gtx)
	if math.Abs(v.zoom-expectedZoomOut) > 0.01 {
		t.Fatalf("zoom after zoom-out = %.3f; want %.3f", v.zoom, expectedZoomOut)
	}

	// Trigger zoom-indicator: reset to 100% (actual size)
	a.handlePDFAction(gtx, "zoom-indicator")
	if v.zoom != 1.0 || v.fit != "" {
		t.Fatalf("zoom-indicator did not reset to 1.0 (got zoom=%.3f fit=%q)", v.zoom, v.fit)
	}
	ops.Reset()
	a.layoutPDFDocument(gtx)
	if math.Abs(v.scale-baseScale) > 0.001 {
		t.Fatalf("100%% actual scale = %.3f; want %.3f", v.scale, baseScale)
	}
	if v.currentZoom(metric) != 1.0 {
		t.Fatalf("currentZoom at 100%% = %.3f; want 1.0", v.currentZoom(metric))
	}

	// Trigger fit-width
	a.handlePDFAction(gtx, "fit-width")
	if v.fit != "width" {
		t.Fatalf("fit-width not set, got %q", v.fit)
	}
	ops.Reset()
	a.layoutPDFDocument(gtx)
	if v.scale <= 0 || v.scale == baseScale {
		t.Fatalf("fit-width did not recompute scale: %.3f", v.scale)
	}

	// Trigger fit-page
	a.handlePDFAction(gtx, "fit-page")
	if v.fit != "page" {
		t.Fatalf("fit-page not set, got %q", v.fit)
	}
	ops.Reset()
	a.layoutPDFDocument(gtx)
	if math.Abs(v.scale-initialScale) > 0.001 {
		t.Fatalf("fit-page scale = %.3f; want initial scale %.3f", v.scale, initialScale)
	}
}
