package desktopui

import (
	"image"
	"math"
	"testing"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"glance/internal/document"
	"glance/internal/wordlayout"
)

func TestWordRulerCanonicalGeometryTracksZoomAndScroll(t *testing.T) {
	for _, density := range []float32{1, 2} {
		for _, zoom := range []float64{.5, 1, 1.5} {
			page := wordlayout.PageScene{Clip: wordlayout.Rect{WidthPT: 600, HeightPT: 800}, Content: wordlayout.Rect{XPT: 60, WidthPT: 480}}
			state := newDocumentSessionState("ruler.docx")
			state.wordLayoutZoom = zoom
			state.wordPageScrollX = 120
			state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{page}}
			gtx := layout.Context{Metric: unit.Metric{PxPerDp: density, PxPerSp: density}, Constraints: layout.Exact(image.Pt(int(700*density), 500))}
			g := wordRulerGeometryForState(gtx, state)
			scale := float64(density) * 4 / 3 * zoom
			w := int(math.Ceil(600 * scale))
			scroll := min(120, max(0, w-int(700*density)))
			left := max(0, (int(700*density)-w)/2) - scroll
			if g.pageLeft != left || g.textLeft != left+int(math.Round(60*scale)) || g.textRight != left+int(math.Round(540*scale)) {
				t.Fatalf("density %v zoom %v: %+v", density, zoom, g)
			}
			if math.Abs(float64(g.pixels(wordEMUPerCM))-72/2.54*scale) > .51 {
				t.Fatal("centimetres stretched to viewport")
			}
			if got := g.emu(float64(wordEMUPerCM) * g.pixelsPerEMU); absRuler(got-wordEMUPerCM) > 635 {
				t.Fatalf("inverse conversion %d", got)
			}
		}
	}
}
func absRuler(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestWordRulerDragSemanticsAndDensity(t *testing.T) {
	for _, density := range []float64{1, 2} {
		for _, zoom := range []float64{.5, 1, 1.5} {
			g := wordRulerGeometry{pageLeft: 0, pageRight: int(900 * density * zoom), textLeft: int(100 * density * zoom), textRight: int(800 * density * zoom), pixelsPerEMU: density * zoom / float64(wordEMUPerDP)}
			start := wordRulerIndents{left: 20 * wordEMUPerDP, right: 30 * wordEMUPerDP, first: 10 * wordEMUPerDP}
			for _, side := range []int{-1, -2, -3, 1} {
				got := wordRulerDraggedIndents(g, start, side, 40*density*zoom)
				switch side {
				case -1:
					if got.left != 60*wordEMUPerDP || got.first != start.first {
						t.Fatalf("whole left drag %+v", got)
					}
				case -2:
					if got.left != start.left || got.first != 50*wordEMUPerDP {
						t.Fatalf("first-line drag %+v", got)
					}
				case -3:
					if got.left != 60*wordEMUPerDP || got.left+got.first != start.left+start.first {
						t.Fatalf("hanging drag %+v", got)
					}
				case 1:
					if got.right != -10*wordEMUPerDP || got.left != start.left {
						t.Fatalf("right drag %+v", got)
					}
				}
			}
			got := wordRulerDraggedIndents(g, start, -1, -10000)
			if got.left != g.emu(float64(-g.textLeft)) {
				t.Fatalf("negative indent clamping %+v", got)
			}
			got = wordRulerDraggedIndents(g, start, 1, -10000)
			if got.right+max(got.left, got.left+got.first) > g.emu(float64(g.textRight-g.textLeft))-12700 {
				t.Fatal("text width collapsed")
			}
		}
	}
}

func TestWordRulerClickPreservesExistingOutOfBoundsIndents(t *testing.T) {
	g := wordRulerGeometry{pageRight: 900, textLeft: 100, textRight: 800, pixelsPerEMU: 1 / float64(wordEMUPerDP)}
	for _, v := range []wordRulerIndents{{left: -150 * wordEMUPerDP}, {right: 850 * wordEMUPerDP}, {first: 900 * wordEMUPerDP}} {
		for _, side := range []int{-1, -2, -3, 1} {
			if got := wordRulerDraggedIndents(g, v, side, 0); got != v {
				t.Fatalf("click changed %+v to %+v", v, got)
			}
		}
	}
}

func TestWordRulerUsesSelectedCellAndColumnTextArea(t *testing.T) {
	for _, cell := range []bool{false, true} {
		state := newDocumentSessionState("context.docx")
		state.wordLayoutZoom = 1
		p := document.WordParagraph{ID: "selected", Inlines: []document.WordInline{{Text: &document.WordTextRun{ID: "run"}}}}
		area := wordlayout.Rect{XPT: 302, WidthPT: 196}
		page := wordlayout.PageScene{Clip: wordlayout.Rect{WidthPT: 600, HeightPT: 800}, Content: wordlayout.Rect{XPT: 60, WidthPT: 480}}
		if cell {
			// Empty cells also resolve correctly, without depending on a glyph fragment.
			state.wordBlocks = []document.WordBlock{{Table: &document.WordTable{Rows: []document.WordTableRow{{Cells: []document.WordTableCell{{ID: "cell", Paragraphs: []document.WordParagraph{p}}}}}}}}
			page.TableCells = []wordlayout.TableCellPlacement{{CellID: "cell", Bounds: wordlayout.Rect{XPT: 300, WidthPT: 200}}}
		} else {
			state.wordBlocks = []document.WordBlock{{Paragraph: &p}}
			page.Text = []wordlayout.TextFragment{{RunID: "run", ParagraphArea: area}}
		}
		state.selectedWordParagraphID = p.ID
		state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{page}}
		g := wordRulerGeometryForState(layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1000, 500))}, state)
		if g.textLeft != g.pageLeft+int(math.Round(area.XPT*4/3)) || g.textRight != g.pageLeft+int(math.Round((area.XPT+area.WidthPT)*4/3)) {
			t.Fatalf("cell=%v wrong context %+v", cell, g)
		}
		if cell {
			v := wordRulerDraggedIndents(g, wordRulerIndents{}, -1, -10000)
			if v.left < -2*12700-635 {
				t.Fatalf("cell drag escaped content: %+v", v)
			}
		}
	}
}

func TestWordRulerFollowsCaretInContinuedColumnAndPairedPage(t *testing.T) {
	for _, paired := range []bool{false, true} {
		state := newDocumentSessionState("continued.docx")
		state.wordLayoutZoom = 1
		state.selectedWordRunID = "run"
		editor := new(widget.Editor)
		editor.SetText("abcdefghijklmnopqrst")
		editor.SetCaret(12, 12)
		state.wordRunEditors = map[string]*widget.Editor{"run": editor}
		first := wordlayout.Rect{XPT: 60, WidthPT: 200}
		second := wordlayout.Rect{XPT: 300, WidthPT: 200}
		page := wordlayout.PageScene{Clip: wordlayout.Rect{WidthPT: 600, HeightPT: 800}, Content: wordlayout.Rect{XPT: 60, WidthPT: 480}, Text: []wordlayout.TextFragment{{RunID: "run", RuneStart: 0, RuneEnd: 10, ParagraphArea: first}, {RunID: "run", RuneStart: 10, RuneEnd: 20, ParagraphArea: second}}}
		result := &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{page}, Locations: map[wordlayout.SourcePosition]wordlayout.Position{{RunID: "run", RuneOffset: 12}: {PageIndex: 0}}}
		if paired {
			state.wordViewMode = wordViewPrintLayout
			state.wordMultiplePagesMode = true
			result.Pages = append(result.Pages, page)
			result.Pages[0].Text = result.Pages[0].Text[:1]
			result.Locations[wordlayout.SourcePosition{RunID: "run", RuneOffset: 12}] = wordlayout.Position{PageIndex: 1}
		}
		state.wordLayoutResult = result
		g := wordRulerGeometryForState(layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1800, 500))}, state)
		if g.textLeft != g.pageLeft+400 {
			t.Fatalf("paired=%v ruler ignored caret column: %+v", paired, g)
		}
		if paired && g.pageLeft < 900 {
			t.Fatalf("ruler stayed on first page: %+v", g)
		}
	}
}
