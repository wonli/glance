package desktopui

import (
	"gioui.org/op/paint"
	"image"
	"testing"

	"glance/internal/document"
	"glance/internal/format/pdfdoc"
)

func TestClosedPDFViewDropsDocumentResources(t *testing.T) {
	data := pdfTestBytes(t)
	d, err := pdfdoc.OpenSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	a := New(nil, "")
	defer close(a.shutdown)
	if err := a.installDocument(loadedDocument{path: "report.pdf", pdfDoc: d}); err != nil {
		t.Fatal(err)
	}

	v := a.ensurePDFView()
	// Populate cache with rendered raster page images.
	v.cache[0] = pdfPageResult{
		page:   document.PDFPage{Index: 0},
		pixels: image.NewRGBA(image.Rect(0, 0, 800, 1000)),
	}
	v.thumbCache[0] = paint.NewImageOp(image.NewRGBA(image.Rect(0, 0, 100, 140)))

	state := a.documentSessionState
	if state.pdfView == nil {
		t.Fatal("expected non-nil pdfView on documentSessionState")
	}

	signal := v.workerSignal()

	// Closing session state must drop view, cache, and cancel workers.
	closeSessionState(state)

	if state.pdfView != nil {
		t.Fatal("closed view still owns pdfView reference")
	}
	if v.cache != nil {
		t.Fatal("closed view still holds raster page image cache")
	}
	if v.thumbCache != nil {
		t.Fatal("closed view still holds thumbnail image cache")
	}
	select {
	case <-signal:
	default:
		t.Fatal("closing view did not signal worker cancellation")
	}

	// Repeated teardown must be harmless.
	closeSessionState(state)
}

func TestPDFWorkerCancellationOnClose(t *testing.T) {
	v := &pdfViewState{
		info:       document.PDFInfo{Pages: 4},
		cache:      map[int]pdfPageResult{},
		thumbCache: map[int]paint.ImageOp{},
		results:    make(chan pdfPageResult, 1),
	}
	signal := v.workerSignal()
	v.cancelWorkers()

	select {
	case <-signal:
	default:
		t.Fatal("expected workerSignal to be closed after cancelWorkers")
	}

	// Calling cancel again is safe.
	v.cancelWorkers()
}

func TestPDFDocCloseReleasesMemory(t *testing.T) {
	data := pdfTestBytes(t)
	d, err := pdfdoc.OpenSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}

	p, err := d.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	// Apply command to create undo history.
	if err := d.Apply(document.PDFCommand{Kind: "replace", Page: 0, Revision: p.Revision, Run: 0, Text: "MODIFIED"}); err != nil {
		t.Fatal(err)
	}

	if err := d.Close(); err != nil {
		t.Fatal(err)
	}

	// Verify all operations fail closed.
	if _, err := d.Snapshot(); err == nil {
		t.Fatal("Snapshot succeeded on closed document")
	}
	if _, err := d.Page(0); err == nil {
		t.Fatal("Page succeeded on closed document")
	}
	if _, _, _, err := d.Render(0, 72); err == nil {
		t.Fatal("Render succeeded on closed document")
	}
	if err := d.Apply(document.PDFCommand{Kind: "undo"}); err == nil {
		t.Fatal("Apply succeeded on closed document")
	}

	// Verify repeated Close is harmless.
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
}
