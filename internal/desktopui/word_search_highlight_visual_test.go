package desktopui

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"glance/internal/document"
	"glance/internal/wordlayout"
)

func TestWordSearchHighlightsRenderAllMatchesAndKeepCurrentSelectionDistinct(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p1", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "first", Text: "beta beta", Editable: true}},
	}}
	blocks := []document.WordBlock{{Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	state := newDocumentSessionState("word-search-highlight.docx")
	state.wordBlocks = blocks
	results := findWordSearchResults(blocks, "beta", nil)
	state.setWordSearchHighlights("beta", results.ranges)
	state.selectedWordRunID = "first"
	editor := &widget.Editor{}
	editor.SetText("beta beta")
	editor.SetCaret(0, 4)
	state.wordRunEditors = map[string]*widget.Editor{"first": editor}
	app := &App{documentSessionState: state}
	app.modal.Visible = true
	app.modal.SearchReplace = true
	app.modal.Input.SetText("beta")

	glyphs := make([]wordlayout.PositionedGlyph, len([]rune("beta beta")))
	for index := range glyphs {
		glyphs[index] = wordlayout.PositionedGlyph{RuneStart: index, RuneEnd: index + 1, XPT: 4 + float64(index*5), AdvanceXPT: 5}
	}
	page := wordlayout.PageScene{Text: []wordlayout.TextFragment{{
		RunID: "first", BaselineYPT: 20, Style: document.WordTextStyle{FontSize: 12}, Glyphs: glyphs,
	}}}

	window, err := headless.NewWindow(64, 40)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	var ops op.Ops
	render := func() *image.RGBA {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(64, 40)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: image.Pt(64, 40)}.Op())
		app.drawWordPageSearchHighlights(gtx, state, page, image.Point{}, 1)
		drawWordPageSelection(gtx, state, page, image.Point{}, 1)
		if err := window.Frame(&ops); err != nil {
			t.Fatalf("render Word search highlights: %v", err)
		}
		shot := image.NewRGBA(image.Rect(0, 0, 64, 40))
		if err := window.Screenshot(shot); err != nil {
			t.Fatalf("capture Word search highlights: %v", err)
		}
		return shot
	}

	shot := render()
	current := color.NRGBAModel.Convert(shot.At(6, 12)).(color.NRGBA)
	other := color.NRGBAModel.Convert(shot.At(31, 12)).(color.NRGBA)
	if current == other || other.R < 245 || other.G <= other.B || current.R >= other.R {
		t.Fatalf("active and other search pixels = %#v / %#v; want blue active selection over a visible yellow highlight", current, other)
	}

	app.modal.Input.SetText("other")
	editor.SetCaret(0, 0)
	shot = render()
	stale := color.NRGBAModel.Convert(shot.At(31, 12)).(color.NRGBA)
	if stale != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("pixel after query changed = %#v; want the stale search highlight cleared", stale)
	}
}
