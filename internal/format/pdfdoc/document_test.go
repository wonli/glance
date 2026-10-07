package pdfdoc

import (
	"bytes"
	"github.com/SalvioniDigitalSolutions/gopdf"
	"glance/internal/document"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	d := gopdf.New()
	p := d.AddPage()
	p.SetFont(gopdf.Helvetica, 12)
	p.Text(40, 60, "Hello PDF")
	p.AddNote(90, 90, "keep note", gopdf.NoteOptions{})
	p = d.AddPage()
	p.Text(40, 60, "untouched page")
	var b bytes.Buffer
	if _, e := d.WriteTo(&b); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

func TestNewBlank(t *testing.T) {
	d, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	if !d.Dirty() {
		t.Fatal("NewBlank document must be dirty")
	}
	info := d.Info()
	if info.Pages != 1 {
		t.Fatalf("pages = %d, want 1", info.Pages)
	}
}
func TestSingleRevisionEditUndoSave(t *testing.T) {
	d, e := OpenSnapshot(fixture(t))
	if e != nil {
		t.Fatal(e)
	}
	p, e := d.Page(0)
	if e != nil {
		t.Fatal(e)
	}
	before, _ := d.Snapshot()
	if e = d.Apply(document.PDFCommand{Kind: "replace", Page: 0, Revision: p.Revision, Run: 0, Text: "Hello PDF"}); e != nil {
		t.Fatal(e)
	}
	if d.Dirty() {
		t.Fatal("no-op dirtied document")
	}
	if e = d.Apply(document.PDFCommand{Kind: "replace", Page: 0, Revision: p.Revision, Run: 0, Text: "Changed PDF"}); e != nil {
		t.Fatal(e)
	}
	current, _ := d.Page(0)
	if current.Text[0].Text != "Changed PDF" {
		t.Fatal(current.Text)
	}
	if e = d.Apply(document.PDFCommand{Kind: "replace", Page: 0, Revision: p.Revision, Run: 0, Text: "stale"}); e == nil {
		t.Fatal("stale edit accepted")
	}
	out := filepath.Join(t.TempDir(), "edited.pdf")
	if e = d.SaveAs(out); e != nil {
		t.Fatal(e)
	}
	r, e := gopdf.Open(out)
	if e != nil {
		t.Fatal(e)
	}
	text, _ := r.PageText(1)
	if !bytes.Contains([]byte(text), []byte("untouched page")) {
		t.Fatal(text)
	}
	ann, _ := r.Annotations(0)
	if len(ann) != 1 {
		t.Fatal("annotation lost")
	}
	if !d.Undo() {
		t.Fatal("undo")
	}
	got, _ := d.Snapshot()
	if !bytes.Equal(got, before) {
		t.Fatal("undo did not restore model")
	}
	if !d.Dirty() {
		t.Fatal("undo after save must dirty")
	}
	if !d.Redo() {
		t.Fatal("redo")
	}
	if d.Dirty() {
		t.Fatal("redo saved revision should be clean")
	}
}
func TestFailedSaveAndInvalidEditPreserveModel(t *testing.T) {
	d, _ := OpenSnapshot(fixture(t))
	p, _ := d.Page(0)
	before, _ := d.Snapshot()
	if e := d.Apply(document.PDFCommand{Kind: "replace", Page: 0, Revision: p.Revision, Run: 999, Text: "bad"}); e == nil {
		t.Fatal("bad run accepted")
	}
	got, _ := d.Snapshot()
	if !bytes.Equal(got, before) {
		t.Fatal("failed edit mutated model")
	}
	if e := d.Apply(document.PDFCommand{Kind: "note", Page: 0, Revision: p.Revision, Text: "new note", X: 50, Y: 100}); e != nil {
		t.Fatal(e)
	}
	if e := d.SaveAs(filepath.Join(t.TempDir(), "missing", "a.pdf")); e == nil {
		t.Fatal("save succeeded")
	}
	if !d.Dirty() {
		t.Fatal("failed save cleared dirty")
	}
}

func TestAnnotationsInkRotationRenderRoundTrip(t *testing.T) {
	d, err := OpenSnapshot(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	original, _, _, err := d.Render(0, 72)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []document.PDFCommand{
		{Kind: "highlight", X: 40, Y: 48, W: 50, H: 15, Text: "highlight"},
		{Kind: "note", X: 100, Y: 150, Text: "review note"},
		{Kind: "ink", X: 200, Y: 200, Points: []document.PDFPoint{{X: 200, Y: 200}, {X: 250, Y: 250}, {X: 300, Y: 200}}},
	} {
		command.Revision = d.Info().Revision
		if err = d.Apply(command); err != nil {
			t.Fatal(err)
		}
	}
	rendered, page, _, err := d.Render(0, 72)
	if err != nil {
		t.Fatal(err)
	}
	if page.Revision != d.Info().Revision || len(page.Annotations) != 3 {
		t.Fatalf("scene disagrees: %+v", page)
	}
	if original.At(225, 225) == rendered.At(225, 225) {
		t.Fatal("ink missing from current model render")
	}
	path := filepath.Join(t.TempDir(), "annotated.pdf")
	if err = d.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := reopened.Page(0)
	if err != nil || len(p.Annotations) != 3 {
		t.Fatalf("annotations lost: %v %v", p, err)
	}
	if err = d.Apply(document.PDFCommand{Kind: "rotate", Revision: d.Info().Revision}); err != nil {
		t.Fatal(err)
	}
	rotated, p, _, err := d.Render(0, 72)
	if err != nil {
		t.Fatal(err)
	}
	if p.Rotation != 90 || rotated.Bounds().Dx() != rendered.Bounds().Dy() {
		t.Fatalf("rotation failed: %+v %v", p, rotated.Bounds())
	}
}
