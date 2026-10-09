package desktopui

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"

	"glance/internal/wordlayout"
)

func TestWordPageSelectionCrossesFormattedRunBoundaries(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: "abc", Editable: true}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: "DEF", Editable: true}},
	}}
	state := newDocumentSessionState("")
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	result := &wordlayout.Result{Pages: []wordlayout.PageScene{{Text: []wordlayout.TextFragment{
		{RunID: "run-a", Text: "abc", RuneStart: 0, RuneEnd: 3, Editable: true},
		{RunID: "run-b", Text: "DEF", RuneStart: 0, RuneEnd: 3, Editable: true},
	}}}}
	selection := newWordPageTextSelection(result, wordPageCaretPosition{runID: "run-a", runeOffset: 2})
	if selection.runOrder != nil {
		t.Fatal("single-click caret allocated a document-wide run order map")
	}
	populateWordPageRunOrder(selection, result)
	selection.focus = wordPageCaretPosition{runID: "run-b", runeOffset: 2}
	state.wordPageTextSelection = selection

	start, end, ok := wordPageTextSelectionRange(selection, result.Pages[0].Text[0])
	if !ok || start != 2 || end != 3 {
		t.Fatalf("first run selection = [%d,%d), %v; want [2,3)", start, end, ok)
	}
	start, end, ok = wordPageTextSelectionRange(selection, result.Pages[0].Text[1])
	if !ok || start != 0 || end != 2 {
		t.Fatalf("second run selection = [%d,%d), %v; want [0,2)", start, end, ok)
	}
	if selected, ok := wordPageSelectedText(state); !ok || selected != "cDE" {
		t.Fatalf("selected text = %q, %v; want %q", selected, ok, "cDE")
	}
}

func TestWordPagePendingCaretDoesNotCollapseMouseDragSelection(t *testing.T) {
	state := newDocumentSessionState("pending-caret-selection.docx")
	editor := new(widget.Editor)
	editor.SetText("abcdef")
	editor.SetCaret(0, 5)
	state.wordPageTextSelection = &wordPageTextSelection{
		anchor: wordPageCaretPosition{runID: "run", runeOffset: 0},
		focus:  wordPageCaretPosition{runID: "run", runeOffset: 5},
	}
	state.wordPendingCaretRunID = "run"
	state.wordPendingCaretOffset = 0

	applyWordPagePendingCaret(state, editor, "run", wordlayout.PageScene{}, 1)

	start, end := editor.Selection()
	if start != 0 || end != 5 {
		t.Fatalf("pending click caret collapsed drag selection to [%d,%d); want [0,5)", start, end)
	}
	if state.wordPendingCaretRunID != "" {
		t.Fatalf("pending caret handoff remained queued for %q", state.wordPendingCaretRunID)
	}
}

func TestWordPageHitTestUsesRunRelativeGlyphOffsets(t *testing.T) {
	page := wordlayout.PageScene{Text: []wordlayout.TextFragment{{
		RunID: "run", Editable: true, Style: document.WordTextStyle{FontSize: 10}, BaselineYPT: 20,
		Glyphs: []wordlayout.PositionedGlyph{
			{RuneStart: 0, RuneEnd: 1, XPT: 10, AdvanceXPT: 10},
			{RuneStart: 1, RuneEnd: 2, XPT: 20, AdvanceXPT: 10},
			{RuneStart: 2, RuneEnd: 3, XPT: 30, AdvanceXPT: 10},
		},
	}}}
	for index, x := range []float32{10.1, 20.1, 30.1} {
		hit := wordPageHitTest(page, f32.Pt(x, 20))
		if !hit.found || hit.runeOffset != index {
			t.Errorf("click near glyph %d returned caret %d (found %v); want %d", index, hit.runeOffset, hit.found, index)
		}
	}
}

func TestWordPageHitTestAtGlyphMidpointPrefersLeadingCaret(t *testing.T) {
	page := wordlayout.PageScene{Text: []wordlayout.TextFragment{{
		RunID: "run", Editable: true, Style: document.WordTextStyle{FontSize: 10}, BaselineYPT: 20,
		Glyphs: []wordlayout.PositionedGlyph{
			{RuneStart: 0, RuneEnd: 1, XPT: 10, AdvanceXPT: 10},
			{RuneStart: 1, RuneEnd: 2, XPT: 20, AdvanceXPT: 10},
			{RuneStart: 2, RuneEnd: 3, XPT: 30, AdvanceXPT: 10},
		},
	}}}
	for index, x := range []float32{15, 25, 35} {
		hit := wordPageHitTest(page, f32.Pt(x, 20))
		if !hit.found || hit.runeOffset != index {
			t.Errorf("click at center of glyph %d returned caret %d (found %v); want leading caret %d", index, hit.runeOffset, hit.found, index)
		}
	}
}

func TestWordPageDragSelectionContinuesAcrossVisibleRun(t *testing.T) {
	const firstText, secondText = "abc", "defghij"
	paragraph := document.WordParagraph{ID: "p", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: firstText, Editable: true}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: secondText, Editable: true}},
	}}
	page := wordlayout.PageScene{Index: 0, Clip: wordlayout.Rect{WidthPT: 120, HeightPT: 100}}
	fragments := []wordlayout.TextFragment{
		{RunID: "run-a", Text: firstText, RuneStart: 0, RuneEnd: len(firstText), Editable: true, BaselineYPT: 30, Style: document.WordTextStyle{FontSize: 10}},
		{RunID: "run-b", Text: secondText, RuneStart: 0, RuneEnd: len(secondText), Editable: true, BaselineYPT: 30, Style: document.WordTextStyle{FontSize: 10}},
	}
	for index := range firstText {
		fragments[0].Glyphs = append(fragments[0].Glyphs, wordlayout.PositionedGlyph{
			RuneStart: index, RuneEnd: index + 1, XPT: 10 + float64(index*7), AdvanceXPT: 7,
		})
	}
	for index := range secondText {
		fragments[1].Glyphs = append(fragments[1].Glyphs, wordlayout.PositionedGlyph{
			RuneStart: index, RuneEnd: index + 1, XPT: 31 + float64(index*7), AdvanceXPT: 7,
		})
	}
	page.Text = fragments
	locations := make(map[wordlayout.SourcePosition]wordlayout.Position, len(firstText)+len(secondText)+2)
	for index := 0; index <= len(firstText); index++ {
		locations[wordlayout.SourcePosition{RunID: "run-a", RuneOffset: index}] = wordlayout.Position{PageIndex: 0, XPT: 10 + float64(index*7), YPT: 30}
	}
	for index := 0; index <= len(secondText); index++ {
		locations[wordlayout.SourcePosition{RunID: "run-b", RuneOffset: index}] = wordlayout.Position{PageIndex: 0, XPT: 31 + float64(index*7), YPT: 30}
	}
	state := newDocumentSessionState("pointer-drag.docx")
	state.wordBlocks = []document.WordBlock{{ID: "p", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{page}, Locations: locations}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Source: router.Source(), Ops: &ops, Now: time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(600, 500)},
		}
		app.layoutWordPages(gtx, state, wordPageContinuous)
		router.Frame(&ops)
	}
	frame()
	if len(state.wordPageVisibleInteractions) != 1 {
		t.Fatalf("visible page interactions = %d; want one", len(state.wordPageVisibleInteractions))
	}
	interaction := state.wordPageVisibleInteractions[0]
	start := interaction.Transform.PageToScreen(f32.Pt(float32(fragments[0].Glyphs[0].XPT+1), 30))
	lastGlyph := fragments[1].Glyphs[len(fragments[1].Glyphs)-1]
	end := interaction.Transform.PageToScreen(f32.Pt(float32(lastGlyph.XPT+lastGlyph.AdvanceXPT-1), 50))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 51, Buttons: pointer.ButtonPrimary, Position: start})
	frame()
	if state.wordPageSelectionDrag == nil {
		t.Fatal("press did not start page selection drag")
	}
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 51, Buttons: pointer.ButtonPrimary, Position: end})
	frame()
	selection := state.wordPageTextSelection
	if selection == nil || selection.focus.runID != "run-b" || selection.focus.runeOffset != len(secondText) {
		got := wordPageCaretPosition{}
		if selection != nil {
			got = selection.focus
		}
		t.Fatalf("drag focus = %+v; want end of second run", got)
	}
	if selected, ok := wordPageSelectedText(state); !ok || selected != firstText+secondText {
		t.Fatalf("cross-run selected text = %q, %v; want %q", selected, ok, firstText+secondText)
	}
}

func TestWordPageClickAtGlyphMidpointPlacesCaretBeforeThatRune(t *testing.T) {
	const text = "大猫OA软件"
	runes := []rune(text)
	paragraph := document.WordParagraph{ID: "p", Inlines: []document.WordInline{{
		Kind: document.WordTextInline,
		Text: &document.WordTextRun{ID: "run", Text: text, Editable: true},
	}}}
	page := wordlayout.PageScene{Index: 0, Clip: wordlayout.Rect{WidthPT: 120, HeightPT: 100}}
	fragment := wordlayout.TextFragment{
		RunID: "run", Text: text, RuneStart: 0, RuneEnd: len(runes), Editable: true,
		BaselineYPT: 30, Style: document.WordTextStyle{FontSize: 10},
	}
	for index := range runes {
		fragment.Glyphs = append(fragment.Glyphs, wordlayout.PositionedGlyph{
			RuneStart: index, RuneEnd: index + 1, XPT: 10 + float64(index*7), AdvanceXPT: 7,
		})
	}
	page.Text = []wordlayout.TextFragment{fragment}
	locations := make(map[wordlayout.SourcePosition]wordlayout.Position, len(runes)+1)
	for index := 0; index <= len(runes); index++ {
		locations[wordlayout.SourcePosition{RunID: "run", RuneOffset: index}] = wordlayout.Position{PageIndex: 0, XPT: 10 + float64(index*7), YPT: 30}
	}
	state := newDocumentSessionState("pointer-click.docx")
	state.wordLayoutZoom = 0.375 // With a 2x device metric, this makes page points map 1:1 to pointer pixels.
	state.wordBlocks = []document.WordBlock{{ID: "p", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{page}, Locations: locations}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Source: router.Source(), Ops: &ops, Now: time.Now(),
			Metric:      unit.Metric{PxPerDp: 2, PxPerSp: 2},
			Constraints: layout.Constraints{Max: image.Pt(600, 500)},
		}
		app.layoutWordPages(gtx, state, wordPageContinuous)
		router.Frame(&ops)
	}
	frame()
	interaction := state.wordPageVisibleInteractions[0]
	for index := 0; index < len(runes); index++ {
		glyph := fragment.Glyphs[index]
		position := interaction.Transform.PageToScreen(f32.Pt(float32(glyph.XPT+glyph.AdvanceXPT/2), 30))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 52, Buttons: pointer.ButtonPrimary, Position: position})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 52, Position: position})
		frame()
		editor := state.wordRunEditors["run"]
		if editor == nil {
			t.Fatalf("click at glyph %d did not create a run editor", index)
		}
		start, end := editor.Selection()
		if start != index || end != index {
			t.Fatalf("click at glyph %d midpoint placed caret at [%d,%d); want leading caret [%d,%d)", index, start, end, index, index)
		}
		caret := state.wordLayoutResult.Locations[wordlayout.SourcePosition{RunID: "run", RuneOffset: index}]
		if caret.XPT != glyph.XPT {
			t.Fatalf("midpoint click at glyph %d placed visual caret at %.2fpt; want %.2fpt", index, caret.XPT, glyph.XPT)
		}
	}
}

func TestWordPageMouseClickKeepsViewportPositionAndOnlyRegistersVisiblePages(t *testing.T) {
	const pageCount = 100
	blocks := make([]document.WordBlock, pageCount)
	pages := make([]wordlayout.PageScene, pageCount)
	locations := make(map[wordlayout.SourcePosition]wordlayout.Position, pageCount)
	for index := range pageCount {
		runID := fmt.Sprintf("run-%d", index)
		paragraph := document.WordParagraph{ID: fmt.Sprintf("p-%d", index), Inlines: []document.WordInline{{
			Kind: document.WordTextInline,
			Text: &document.WordTextRun{ID: runID, Text: "中", Editable: true},
		}}}
		blocks[index] = document.WordBlock{ID: paragraph.ID, Kind: document.WordParagraphBlock, Paragraph: &paragraph}
		pages[index] = wordlayout.PageScene{
			Index: index, Clip: wordlayout.Rect{WidthPT: 120, HeightPT: 100},
			Text: []wordlayout.TextFragment{{
				RunID: runID, Text: "中", RuneStart: 0, RuneEnd: 1, Editable: true,
				BaselineYPT: 30, Style: document.WordTextStyle{FontSize: 10},
				Glyphs: []wordlayout.PositionedGlyph{{RuneStart: 0, RuneEnd: 1, XPT: 10, AdvanceXPT: 10}},
			}},
		}
		locations[wordlayout.SourcePosition{RunID: runID, RuneOffset: 0}] = wordlayout.Position{PageIndex: index, XPT: 10, YPT: 30}
	}
	state := newDocumentSessionState("mouse-does-not-scroll.docx")
	state.wordBlocks = blocks
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: pages, Locations: locations}
	state.wordLayoutZoom = 0.375
	state.wordPageListState.Position.First = 4
	state.wordPageListState.Position.Offset = 7
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Source: router.Source(), Ops: &ops, Now: time.Now(),
			Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}, Constraints: layout.Constraints{Max: image.Pt(600, 250)},
		}
		app.layoutWordPages(gtx, state, wordPagePrint)
		router.Frame(&ops)
	}
	frame()
	first, offset := state.wordPageListState.Position.First, state.wordPageListState.Position.Offset
	if first != 4 {
		t.Fatalf("visible window starts at row %d; want page 5 / row 4", first)
	}
	if len(state.wordPageVisibleInteractions) == 0 || len(state.wordPageVisibleInteractions) >= pageCount {
		t.Fatalf("registered %d page interactions for %d pages; want only visible pages", len(state.wordPageVisibleInteractions), pageCount)
	}
	for _, visible := range state.wordPageVisibleInteractions {
		if visible.PageIndex < first {
			t.Fatalf("off-window page %d has an interaction tag while row %d is visible", visible.PageIndex, first)
		}
	}
	wantPages := make(map[string]struct{}, len(state.wordPageVisibleInteractions))
	for _, visible := range state.wordPageVisibleInteractions {
		wantPages[fmt.Sprintf("Word 第 %d 页，共 %d 页", visible.PageIndex+1, pageCount)] = struct{}{}
	}
	paintedPages := make(map[string][]image.Rectangle)
	var visitSemantics func([]input.SemanticNode)
	visitSemantics = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			description := node.Desc.Description
			if strings.HasPrefix(description, "Word 第 ") && strings.Contains(description, "共 100 页") {
				paintedPages[description] = append(paintedPages[description], node.Desc.Bounds)
			}
			visitSemantics(node.Children)
		}
	}
	visitSemantics(router.AppendSemantics(nil))
	viewport := image.Rect(0, 0, 600, 250)
	visibleScenes := make(map[string]struct{})
	for description, bounds := range paintedPages {
		for _, bound := range bounds {
			if bound.Overlaps(viewport) {
				visibleScenes[description] = struct{}{}
			}
		}
	}
	if len(visibleScenes) != len(wantPages) {
		t.Fatalf("layout emitted %d visible page scenes for a 100-page document; visible page window contains %d (all semantic bounds: %v)", len(visibleScenes), len(wantPages), paintedPages)
	}
	for description := range visibleScenes {
		if _, visible := wantPages[description]; !visible {
			t.Fatalf("off-window page scene intersects the viewport: %q; visible scenes=%v; all semantic bounds=%v", description, wantPages, paintedPages)
		}
	}
	interaction, ok := visibleWordPageInteraction(state.wordPageVisibleInteractions, 4)
	if !ok {
		t.Fatal("current page 5 is not in the visible interaction window")
	}
	position := interaction.Transform.PageToScreen(f32.Pt(10.25, 30))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 93, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 93, Position: position})
	frame()
	if state.wordPageListState.Position.First != first || state.wordPageListState.Position.Offset != offset {
		t.Fatalf("mouse click moved page viewport from row/offset %d/%d to %d/%d", first, offset, state.wordPageListState.Position.First, state.wordPageListState.Position.Offset)
	}
}

func TestWordPageClickAtRoundedGlyphMidpointUsesLeadingCaret(t *testing.T) {
	const text = "大猫OA"
	runes := []rune(text)
	advances := []float64{11, 11, 8.75, 8.25}
	paragraph := document.WordParagraph{ID: "p", Inlines: []document.WordInline{{
		Kind: document.WordTextInline,
		Text: &document.WordTextRun{ID: "run", Text: text, Editable: true},
	}}}
	page := wordlayout.PageScene{Index: 0, Clip: wordlayout.Rect{WidthPT: 595, HeightPT: 842}}
	fragment := wordlayout.TextFragment{
		RunID: "run", Text: text, RuneStart: 0, RuneEnd: len(runes), Editable: true,
		BaselineYPT: 30, Style: document.WordTextStyle{FontSize: 10.5}, XPT: 90,
	}
	locations := make(map[wordlayout.SourcePosition]wordlayout.Position, len(runes)+1)
	x := fragment.XPT
	for index, advance := range advances {
		fragment.Glyphs = append(fragment.Glyphs, wordlayout.PositionedGlyph{
			RuneStart: index, RuneEnd: index + 1, XPT: x, AdvanceXPT: advance,
		})
		locations[wordlayout.SourcePosition{RunID: "run", RuneOffset: index}] = wordlayout.Position{PageIndex: 0, XPT: x, YPT: 30}
		x += advance
	}
	locations[wordlayout.SourcePosition{RunID: "run", RuneOffset: len(runes)}] = wordlayout.Position{PageIndex: 0, XPT: x, YPT: 30}
	page.Text = []wordlayout.TextFragment{fragment}
	state := newDocumentSessionState("pixel-rounded-caret.docx")
	state.wordBlocks = []document.WordBlock{{ID: "p", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{page}, Locations: locations}
	state.wordLayoutZoom = 1
	state.wordPageFitMode = false
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Source: router.Source(), Ops: &ops, Now: time.Now(),
			Metric:      unit.Metric{PxPerDp: 2, PxPerSp: 2},
			Constraints: layout.Constraints{Max: image.Pt(2559, 2600)},
		}
		app.layoutWordPages(gtx, state, wordPageContinuous)
		router.Frame(&ops)
	}
	frame()
	interaction := state.wordPageVisibleInteractions[0]
	for index, glyph := range fragment.Glyphs {
		midpoint := interaction.Transform.PageToScreen(f32.Pt(float32(glyph.XPT+glyph.AdvanceXPT/2), float32(fragment.BaselineYPT)))
		// Native pointer devices report integer screen pixels. Rounding the
		// midpoint can move a geometrical tie slightly to the trailing side.
		position := f32.Pt(float32(math.Round(float64(midpoint.X))), float32(math.Round(float64(midpoint.Y))))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 105, Buttons: pointer.ButtonPrimary, Position: position})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 105, Position: position})
		frame()
		start, end := state.wordRunEditors["run"].Selection()
		if start != index || end != index {
			t.Fatalf("rounded midpoint click on glyph %q placed caret at [%d,%d); want leading caret %d", runes[index], start, end, index)
		}
	}
}
