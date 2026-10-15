package desktopui

import (
	"bytes"
	"image"
	"image/png"
	"reflect"
	"strings"
	"testing"
	"time"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/wordfont"
	"glance/internal/wordlayout"
)

func TestWordListMarkerGutterFitsLongAndNestedMarkers(t *testing.T) {
	if got := wordListMarkerWidthDP(document.WordParagraph{ListMarker: "•"}); got != 24 {
		t.Fatalf("bullet marker gutter = %d dp; want 24", got)
	}
	if got := wordListMarkerWidthDP(document.WordParagraph{ListMarker: "100."}); got < 48 {
		t.Fatalf("long numbering marker gutter = %d dp; want at least 48", got)
	}
	if got := wordListMarkerWidthDP(document.WordParagraph{ListMarker: "1.1.", ListLevel: 1}); got != 66 {
		t.Fatalf("nested numbering marker gutter = %d dp; want 66", got)
	}
}

func TestWordContinuousBottomBorderFollowsTheSubtitleParagraph(t *testing.T) {
	border := document.WordParagraphBorder{Style: "single", SizePT: 1, SpacePT: 5, Color: "4F81BD"}
	title := document.WordParagraph{ID: "title", Layout: document.WordParagraphLayout{BottomBorder: border, Set: document.WordParagraphBottomBorderSet}}
	subtitle := document.WordParagraph{ID: "subtitle"}
	blocks := []document.WordBlock{
		{ID: "b0", Kind: document.WordParagraphBlock, Paragraph: &title},
		{ID: "b1", Kind: document.WordParagraphBlock, Paragraph: &subtitle},
	}
	if _, hasBorder := wordContinuousParagraphBottomBorder(blocks, 0); hasBorder {
		t.Fatal("border rendered under the title; Pen places it after the subtitle")
	}
	got, hasBorder := wordContinuousParagraphBottomBorder(blocks, 1)
	if !hasBorder || got != border {
		t.Fatalf("subtitle bottom border = %+v, present=%t; want title's blue line", got, hasBorder)
	}
}

func TestWordCanvasUsesDocumentPageMargins(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	state.wordLayoutResult = &wordlayout.Result{Pages: []wordlayout.PageScene{{
		PageSetup: document.WordPageSetup{MarginLeftPT: 67.5, MarginRightPT: 67.5, MarginTopPT: 50},
	}}}
	left, right, top := wordCanvasPageMarginsDP(layout.Context{}, state)
	if left != 90 || right != 90 || top != 67 {
		t.Fatalf("canvas margins = left %d, right %d, top %d dp; want 90, 90, 67", left, right, top)
	}
}

func TestWordRunLabelUsesPaginationFontFallbackOrder(t *testing.T) {
	run := document.WordTextRun{Style: document.WordTextStyle{FontFamily: "PingFang SC", Bold: true}}
	label := wordRunLabel(material.NewTheme(), document.WordParagraph{}, run)
	families := []string{"PingFang SC"}
	seen := map[string]bool{"pingfang sc": true}
	for _, family := range wordfont.FallbackFamilies() {
		key := strings.ToLower(strings.TrimSpace(family))
		if key != "" && !seen[key] {
			seen[key] = true
			families = append(families, family)
		}
	}
	wantTypeface := font.Typeface(strings.Join(families, ", "))
	if label.Font.Typeface != wantTypeface {
		t.Fatalf("Word editor typeface = %q; want DOCX family followed by pagination fallbacks %q", label.Font.Typeface, wantTypeface)
	}
	if label.Font.Weight != font.Bold {
		t.Fatalf("Word editor font weight = %v; want bold", label.Font.Weight)
	}
}

func TestWordAdjacentParagraphAllowsEmptyAndImageOnlyNeighbors(t *testing.T) {
	current := document.WordParagraph{ID: "current", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "current.run", Editable: true}}}}
	empty := document.WordParagraph{ID: "empty"}
	imageOnly := document.WordParagraph{ID: "image-only", Inlines: []document.WordInline{{Kind: document.WordImageInline, Image: &document.WordImage{ID: "image"}}}}
	for _, neighbor := range []document.WordParagraph{empty, imageOnly} {
		blocks := []document.WordBlock{
			{ID: "neighbor", Kind: document.WordParagraphBlock, Paragraph: &neighbor},
			{ID: "current", Kind: document.WordParagraphBlock, Paragraph: &current},
		}
		if !wordHasAdjacentParagraph(blocks, current.ID, document.WordJoinPrevious) {
			t.Errorf("paragraph %q was not recognized as joinable adjacent content", neighbor.ID)
		}
	}
}

func TestWordFlowKeepsParagraphAndTableCellOrder(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "p0.i0", Text: "Before"}},
		{Kind: document.WordImageInline, Image: &document.WordImage{ID: "img-1"}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "p0.i2", Text: "After"}},
	}}
	cellA := document.WordTableCell{ID: "cell-a", Paragraphs: []document.WordParagraph{{ID: "cell-a.p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "cell-a.p0.i0", Text: "Cell A"}}}}}}
	cellB := document.WordTableCell{ID: "cell-b", Paragraphs: []document.WordParagraph{{ID: "cell-b.p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "cell-b.p0.i0", Text: "Cell B"}}}}}}
	blocks := []document.WordBlock{
		{ID: "b0", Kind: document.WordParagraphBlock, Paragraph: &paragraph},
		{ID: "b1", Kind: document.WordTableBlock, Table: &document.WordTable{Rows: []document.WordTableRow{{Cells: []document.WordTableCell{cellA, cellB}}}}},
		{ID: "b2", Kind: document.WordUnsupportedBlock, Unsupported: &document.WordUnsupported{ID: "b2.unsupported", Description: "Retained content"}},
	}
	order := wordContentOrder(blocks)
	want := []string{"p0.i0", "img-1", "p0.i2", "cell-a.p0.i0", "cell-b.p0.i0", "b2.unsupported"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("Word content order = %#v; want %#v", order, want)
	}
}

func TestWordRunSelectionHandsTextToStableMultilineEditor(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)
	state := newDocumentSessionState("sample.docx")
	state.word = word
	blocks := word.WordBlocks()
	paragraph := blocks[0].Paragraph
	run := paragraph.Inlines[0].Text
	if !selectWordRun(state, paragraph.ID, run.ID) {
		t.Fatal("could not select an editable Word run")
	}
	if state.selectedWordRunID != run.ID || state.selectedWordParagraphID != paragraph.ID || state.wordEditor.Text() != run.Text {
		t.Fatalf("Word selection/editor handoff = run %q paragraph %q text %q", state.selectedWordRunID, state.selectedWordParagraphID, state.wordEditor.Text())
	}
	if state.wordEditor.SingleLine {
		t.Fatal("Word editor should support multiline text")
	}
	first := wordRunEditor(state, *run)
	if first == nil || first != wordRunEditor(state, *run) {
		t.Fatal("Word in-place editor must persist across layout frames")
	}
}

func TestWordFlowLayoutRendersWithStableVerticalListAndRibbon(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("sample.docx")
	state.word = word
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(900, 700)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	dimensions := app.layoutWord(gtx)
	if dimensions.Size != image.Pt(900, 700) {
		t.Fatalf("Word flow dimensions = %v; want viewport size", dimensions.Size)
	}
	if state.wordListState.List.Axis != layout.Vertical || state.wordToolbarActionList.List.Axis != layout.Horizontal {
		t.Fatalf("Word widget axes = flow %v toolbar %v", state.wordListState.List.Axis, state.wordToolbarActionList.List.Axis)
	}
	if state.wordToolbarActionButtons["bold"] == nil {
		t.Fatal("Word Ribbon did not retain its action widgets after layout")
	}
}

func TestWordFlowLayoutRemainsResponsiveAtNarrowWidth(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("sample.docx")
	state.word = word
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(480, 600)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	dimensions := app.layoutWordFlow(gtx)
	if dimensions.Size != image.Pt(480, 600) {
		t.Fatalf("narrow Word flow dimensions = %v; want viewport size", dimensions.Size)
	}
	if state.wordListState.List.Axis != layout.Vertical || state.wordToolbarTabList.List.Axis != layout.Horizontal || state.wordToolbarActionList.List.Axis != layout.Horizontal {
		t.Fatalf("narrow Word list axes = flow %v categories %v commands %v", state.wordListState.List.Axis, state.wordToolbarTabList.List.Axis, state.wordToolbarActionList.List.Axis)
	}
}

func TestWordFlowExposesReferenceRibbonRulersAndStatus(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("sample.docx")
	state.word = word
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Constraints: layout.Constraints{Max: image.Pt(1440, 900)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	app.layoutWordFlow(gtx)
	router.Frame(&ops)

	want := map[string]bool{
		"Word 功能区":  false,
		"Word 水平标尺": false,
		"Word 状态栏":  false,
	}
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if _, ok := want[node.Desc.Description]; ok && !node.Desc.Bounds.Empty() {
				want[node.Desc.Description] = true
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	for label, found := range want {
		if !found {
			t.Errorf("Word workspace is missing visible semantic bounds for %q", label)
		}
	}
}

func TestWordStatusBarMatchesPenFsQrkGeometry(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("sample.docx")
	state.word = word
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Constraints: layout.Constraints{Max: image.Pt(1440, 34)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	app.layoutWordStatusBar(gtx, state)
	router.Frame(&ops)
	want := map[string]image.Rectangle{
		"Word 状态栏":    image.Rect(0, 0, 1440, 34),
		"Word 页码":     image.Rect(18, 11, 136, 27),
		"Word 字数":     image.Rect(142, 11, 188, 27),
		"Word 语言":     image.Rect(250, 11, 303, 27),
		"Word 辅助功能图标": image.Rect(350, 9, 365, 24),
		"Word 辅助功能":   image.Rect(371, 11, 448, 27),
		"Word 专注模式":   image.Rect(1112, 9, 1127, 24),
		"Word 打印布局":   image.Rect(1145, 9, 1160, 24),
		"Word Web版式":  image.Rect(1178, 9, 1193, 24),
		"Word 缩小":     image.Rect(1230, 9, 1245, 24),
		"Word 缩放轨道":   image.Rect(1257, 16, 1349, 18),
		"Word 缩放滑块":   image.Rect(1304, 11, 1315, 22),
		"Word 放大":     image.Rect(1361, 9, 1376, 24),
		"Word 缩放百分比":  image.Rect(1390, 11, 1420, 24),
	}
	got := make(map[string]image.Rectangle, len(want))
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if _, ok := want[node.Desc.Description]; ok {
				got[node.Desc.Description] = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	for label, expected := range want {
		if actual := got[label]; actual != expected {
			t.Errorf("Pen FsQrk element %q bounds = %v; want %v", label, actual, expected)
		}
	}
}

func TestWordHighlightGeometryTracksTextLinesInsteadOfRunWidth(t *testing.T) {
	theme := material.NewTheme()
	theme.Face = font.Typeface("Go")
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	textValue := "highlight wraps across multiple lines"
	label := material.Body1(theme, textValue)
	gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(72, 0), Max: image.Pt(72, 500)}}
	rects := wordTextHighlightRects(gtx, theme.Shaper, label, textValue)
	if len(rects) < 2 {
		t.Fatalf("highlight geometry = %#v; want one rectangle per wrapped text line", rects)
	}
	for index, rect := range rects {
		if rect.Empty() || rect.Min.X < 0 || rect.Max.X > 72 || rect.Dx() >= 72 {
			t.Errorf("highlight rectangle %d = %v; want a nonempty text-width rectangle within the 72 px run", index, rect)
		}
		if index > 0 && rect.Min.Y <= rects[index-1].Min.Y {
			t.Errorf("highlight rectangles are not ordered by line: %#v", rects)
		}
	}
}

func TestWordUnsupportedPlaceholderUsesMeasuredHeight(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Max: image.Pt(640, 720)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	dimensions := wordReadOnlyPlaceholder(gtx, material.NewTheme(), "Unsupported content remains visible and read-only")
	if dimensions.Size.Y <= 0 || dimensions.Size.Y >= gtx.Constraints.Max.Y/2 {
		t.Fatalf("unsupported-content placeholder height = %d; want its measured label height, independent of the %dpx viewport", dimensions.Size.Y, gtx.Constraints.Max.Y)
	}
}

func TestWordTableCellParagraphsExposeIndependentSelections(t *testing.T) {
	cell := document.WordTableCell{ID: "a", Paragraphs: []document.WordParagraph{
		{ID: "a.p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "a.p0.i0", Text: "First", Editable: true}}}},
		{ID: "a.p1", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "a.p1.i0", Text: "Second", Editable: true}}}},
	}}
	table := document.WordTable{Rows: []document.WordTableRow{{Cells: []document.WordTableCell{cell}}}}
	blocks := []document.WordBlock{{ID: "b0", Kind: document.WordTableBlock, Table: &table}}
	paragraphs := allWordParagraphs(blocks)
	if len(paragraphs) != 2 || paragraphs[0].ID == paragraphs[1].ID {
		t.Fatalf("table cell paragraph projections = %#v", paragraphs)
	}
	if _, ok := selectedWordParagraph(blocks, "a.p1"); !ok {
		t.Fatal("second table-cell paragraph is not independently selectable")
	}
}

func TestWordSelectionStyleCanBeReadFromSelectedRun(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "p0.i0", Style: document.WordTextStyle{Bold: true, FontColor: "2196F3"}}}}}
	blocks := []document.WordBlock{{ID: "b0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	if run, ok := selectedWordRun(blocks, "p0.i0"); !ok || !run.Style.Bold || run.Style.FontColor != "2196F3" {
		t.Fatalf("selected Word run style = %#v, %v", run, ok)
	}
}

func TestWordRunEditCommitsThroughAsyncCommandWithoutLosingSelection(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("sample.docx")
	state.word = word
	paragraph := word.WordBlocks()[0].Paragraph
	run := paragraph.Inlines[0].Text
	if !selectWordRun(state, paragraph.ID, run.ID) {
		t.Fatal("could not select the blank Word run")
	}
	value := "edited content"
	state.wordEditor.SetText(value)
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	app.runWordCommand("编辑文字", func(editor document.WordTextEditor) error {
		return editor.ReplaceTextRun(run.ID, value)
	})
	deadline := time.After(3 * time.Second)
	for {
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("async Word run edit failed: %v", result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
			if state.formatRunning.Load() {
				t.Fatal("Word format operation remained marked as running")
			}
			updated, ok := selectedWordRun(word.WordBlocks(), run.ID)
			if !ok || updated.Text != "edited content" {
				t.Fatalf("edited run = %#v, exists=%v", updated, ok)
			}
			if state.selectedWordRunID != run.ID || state.wordEditor.Text() != "edited content" {
				t.Fatalf("selection draft after edit = run %q text %q", state.selectedWordRunID, state.wordEditor.Text())
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for asynchronous Word edit")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestWordHistoryRefreshesProjectionAndInvalidatesImageCache(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	imageID, err := word.InsertInlineImage(paragraph.ID, "", "image/png", tinyToolbarPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("sample.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordImageID = imageID
	state.wordImageCache.Put(imageID, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	app := &App{documentSessionState: state, documentGeneration: 0, wordCommandResults: make(chan wordCommandResult, 1)}

	applyAndWait := func(undo bool) {
		t.Helper()
		app.applyWordHistory(undo)
		deadline := time.After(3 * time.Second)
		for {
			select {
			case result := <-app.wordCommandResults:
				if result.err != nil {
					t.Fatalf("Word history operation failed: %v", result.err)
				}
				app.wordCommandResults <- result
				app.processAsyncResults()
				return
			case <-deadline:
				t.Fatal("timed out waiting for Word history operation")
			default:
				time.Sleep(time.Millisecond)
			}
		}
	}

	applyAndWait(true)
	if len(state.wordBlocks) == 0 || findWordImage(state.wordBlocks, imageID) != nil {
		t.Fatal("undo did not refresh the Word projection without the inserted image")
	}
	if state.wordImageCache.Bytes() != 0 || state.selectedWordImageID != "" {
		t.Fatalf("undo kept image cache/selection: bytes=%d selection=%q", state.wordImageCache.Bytes(), state.selectedWordImageID)
	}

	applyAndWait(false)
	if findWordImage(state.wordBlocks, imageID) == nil {
		t.Fatal("redo did not restore the image in the Word projection")
	}
	if state.wordImageCache.Bytes() != 0 {
		t.Fatalf("redo should invalidate decoded image previews; got %d bytes", state.wordImageCache.Bytes())
	}
}

func TestWordImageInsertOversizedLayoutSuccess(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	state := newDocumentSessionState("test.docx")
	state.word = doc
	state.setWordBlocks(doc.WordBlocks())

	paragraphs := allWordParagraphs(state.wordBlocks)
	if len(paragraphs) == 0 {
		t.Fatal("expected at least one paragraph")
	}

	largeImg := image.NewRGBA(image.Rect(0, 0, 1500, 2500))
	var buf bytes.Buffer
	if err := png.Encode(&buf, largeImg); err != nil {
		t.Fatal(err)
	}

	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 2),
		wordLayoutResults:    make(chan wordLayoutResult, 4),
		shutdown:             make(chan struct{}),
	}

	app.runWordImageInsertCommand(paragraphs[0].ID, "", "image/png", buf.Bytes())

	select {
	case res := <-app.wordCommandResults:
		if res.err != nil {
			t.Fatalf("insert image failed: %v", res.err)
		}
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for image insert")
	}

	app.startWordPagination(state)

	select {
	case layoutRes := <-app.wordLayoutResults:
		if layoutRes.err != nil {
			t.Fatalf("word layout failed with error: %v", layoutRes.err)
		}
		for _, diag := range layoutRes.result.Diagnostics {
			if diag.Severity == wordlayout.DiagnosticError {
				t.Fatalf("unexpected layout error diagnostic: %+v", diag)
			}
		}
		if len(layoutRes.result.Pages) == 0 || len(layoutRes.result.Pages[0].Images) == 0 {
			t.Fatalf("expected placed image on first page, got %d pages", len(layoutRes.result.Pages))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for word layout result")
	}
}
