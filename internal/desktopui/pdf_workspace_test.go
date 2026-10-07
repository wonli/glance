package desktopui

import (
	"bytes"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/SalvioniDigitalSolutions/gopdf"
	"glance/internal/document"
	"glance/internal/format/pdfdoc"
	"glance/internal/workspace"
	"image"
	"image/png"
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
	v := &pdfViewState{info: document.PDFInfo{Revision: 3}, cache: map[int]pdfPageResult{}, thumbCache: map[int]image.Image{}}
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
	v := &pdfViewState{info: d.Info(), cache: map[int]pdfPageResult{0: {}}, thumbCache: map[int]image.Image{}, selected: 0}
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
	a.showPDFDraft(document.PDFCommand{Kind: "replace", Revision: d.Info().Revision, Text: "original"}, "edit")
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
