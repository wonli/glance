package desktopui

import (
	"testing"

	"glance/internal/document"
)

func TestPresentationCustomStyleDialogAppliesDraftAndRejectsStaleSelection(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.view.selectedShape = h.elem.ID
	h.app.showPresentationCustomStyleDialog(h.view, "font_size")
	h.app.modal.Input.SetText("27.5")
	h.app.modal.OnAction("confirm")
	elem := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	if elem.Paragraphs[0].Runs[0].SizePT != 27.5 {
		t.Fatalf("custom font size = %g, want latest draft 27.5", elem.Paragraphs[0].Runs[0].SizePT)
	}
	revision := h.app.pptxDoc.Revision()
	h.app.showPresentationCustomStyleDialog(h.view, "line_spacing")
	h.app.modal.Input.SetText("150%")
	h.app.modal.OnAction("cancel")
	if h.app.pptxDoc.Revision() != revision {
		t.Fatal("cancelled custom style dirtied the document")
	}
	h.app.showPresentationCustomStyleDialog(h.view, "line_spacing")
	h.app.modal.Input.SetText("24 pt")
	h.view.selectedShape = h.elem.ID + 999
	h.app.modal.OnAction("confirm")
	if h.app.pptxDoc.Revision() != revision {
		t.Fatal("stale custom style changed the document")
	}
}

func TestPresentationStyleOptionHasNoActiveValueForMixedFonts(t *testing.T) {
	elem := document.PresentationElement{Paragraphs: []document.PresentationParagraph{{Runs: []document.PresentationTextRun{
		{Text: "one", SizePT: 12}, {Text: "two", SizePT: 18},
	}}}}
	if presentationStyleOptionMatches(elem, document.PresentationElementStylePatch{FontSizePT: styleFloat(12)}) ||
		presentationStyleOptionMatches(elem, document.PresentationElementStylePatch{FontSizePT: styleFloat(18)}) {
		t.Fatal("mixed font sizes marked a guessed font-size preset as active")
	}
}

func TestPresentationUnsupportedStyleTargetsDisableControls(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.view.selectedShape = h.elem.ID
	elem := &h.view.snapshot.Slides[0].Elements[0]
	elem.CanEditTextStyle, elem.CanEditShapeStyle = false, false
	for _, action := range []string{"home_font", "home_line_spacing", "shape_fill", "shape_outline"} {
		if h.app.presentationActionEnabled(h.view, action) {
			t.Fatalf("unsupported target enabled %s", action)
		}
	}
}
