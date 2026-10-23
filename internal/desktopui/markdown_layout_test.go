package desktopui

import (
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	"image"
	"reflect"
	"sort"
	"testing"
)

func TestMarkdownModesAndOutline(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	v.editor.SetText("# 同名\n\n## 同名\n")
	a.flushMarkdownDraft()
	v.snapshot, _ = markdowndoc.Parse(v.editor.Text())
	v.editor.SetCaret(2, 4)
	var router input.Router
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Source: router.Source()}
	for _, id := range []string{"mode-preview", "mode-split", "mode-edit"} {
		a.handleMarkdownAction(gtx, id)
		if v.editor.Text() != "# 同名\n\n## 同名\n" {
			t.Fatal("mode lost draft")
		}
	}
	v.search.SetText("同名")
	if len(v.headings()) != 2 {
		t.Fatal("duplicate headings filtered incorrectly")
	}
	a.handleMarkdownAction(gtx, "heading-6")
	start, end := v.editor.Selection()
	if start != 6 || end != 6 {
		t.Fatalf("heading caret %d,%d", start, end)
	}
}
func TestMarkdownRibbonActionsAndBounds(t *testing.T) {
	a := markdownTestApp(t)
	for _, action := range markdownToolbarActions {
		icon, ok := markdownToolbarIcon(action, true)
		if !ok || icon.Bounds().Empty() {
			t.Errorf("missing %s icon", action.id)
		}
	}
	for _, scale := range []float32{1, 1.5, 2} {
		for _, width := range []int{1440, 640, 360} {
			var ops op.Ops
			var router input.Router
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(float32(width)*scale), int(float32(documentRibbonHeightDP)*scale)))}
			dims := a.layoutMarkdownRibbon(gtx)
			if dims.Size != gtx.Constraints.Max {
				t.Fatalf("ribbon scale %v width %d got %v want %v", scale, width, dims.Size, gtx.Constraints.Max)
			}
		}
	}
}

func TestMarkdownPreviewNaturalHeightAndInlineMath(t *testing.T) {
	a := markdownTestApp(t)
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(600, 0), Max: image.Pt(600, 700)}}
	dims := a.layoutMarkdownBlock(gtx, document.MarkdownBlock{Kind: "code", Text: "first\nsecond\n"}, 0)
	if dims.Size.Y > 120 || dims.Size.Y < 50 {
		t.Fatalf("two-line code height %d", dims.Size.Y)
	}
	a.markdownView.images = map[string]image.Image{"math:x": image.NewRGBA(image.Rect(0, 0, 32, 32))}
	dims = a.layoutMarkdownSpans(gtx, []document.MarkdownSpan{{Text: "before "}, {Kind: "math", Text: "x"}, {Text: " after"}}, 16, homeText, false, 0)
	if dims.Size.Y > 30 {
		t.Fatalf("inline math forced additional rows: %v", dims.Size)
	}
	dims = a.layoutMarkdownBlock(gtx, document.MarkdownBlock{Kind: "table", Rows: [][]string{{"名称", "值"}, {"中文", "42"}}}, 1)
	if dims.Size.Y > 120 {
		t.Fatalf("two-row table height %d", dims.Size.Y)
	}
}

func TestMarkdownEditorUsesBodyLineHeightAndSelectionRowsDoNotOverlap(t *testing.T) {
	a := markdownTestApp(t)
	source := "第一行 ⚠️ 安全提示\n第二行 English text"
	a.markdownView.editor.SetText(source)
	a.markdownView.editor.SetCaret(0, len([]rune(source)))

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Source:      router.Source(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1200, 700)),
	}
	a.layoutMarkdownEditor(gtx)

	wantScale := a.tokens().Typography.BodyLineHeightScale
	if got := a.markdownView.editor.LineHeightScale; got != wantScale {
		t.Fatalf("Markdown editor line-height scale=%v, want shared body token %v", got, wantScale)
	}
	regions := a.markdownView.editor.Regions(0, a.markdownView.editor.Len(), nil)
	if len(regions) < 2 {
		t.Fatalf("selected Markdown text produced %d regions, want at least two", len(regions))
	}
	sort.Slice(regions, func(i, j int) bool {
		if regions[i].Bounds.Min.Y == regions[j].Bounds.Min.Y {
			return regions[i].Bounds.Min.X < regions[j].Bounds.Min.X
		}
		return regions[i].Bounds.Min.Y < regions[j].Bounds.Min.Y
	})
	var rows []image.Rectangle
	for _, region := range regions {
		if len(rows) == 0 || rows[len(rows)-1].Min.Y != region.Bounds.Min.Y || rows[len(rows)-1].Max.Y != region.Bounds.Max.Y {
			rows = append(rows, region.Bounds)
		}
	}
	for i := 1; i < len(rows); i++ {
		if rows[i-1].Max.Y > rows[i].Min.Y {
			t.Fatalf("selection rows overlap: previous=%v next=%v", rows[i-1], rows[i])
		}
	}
}

func TestMarkdownPreviewUsesBodyLineHeightForTextRows(t *testing.T) {
	a := markdownTestApp(t)
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(600, 300)},
	}
	dims := a.layoutMarkdownSpans(gtx, []document.MarkdownSpan{{Text: "first\nsecond"}}, 16, homeText, false, "line-height")
	wantHeight := 2 * gtx.Sp(unit.Sp(16*a.tokens().Typography.BodyLineHeightScale))
	if dims.Size.Y < wantHeight {
		t.Fatalf("two-row Markdown preview height=%d, want at least %d from body line-height token", dims.Size.Y, wantHeight)
	}
}

func TestMarkdownWrapTokensPreservesEmojiWithinNormalTextRuns(t *testing.T) {
	for _, source := range []struct {
		text string
		want []string
	}{
		{text: "🚀 Markdown", want: []string{"🚀 ", "Markdown"}},
		{text: "⚠️ 安全", want: []string{"⚠️ ", "安", "全"}},
		{text: "👩🏽‍💻 editor", want: []string{"👩🏽‍💻 ", "editor"}},
		{text: "🇺🇸 flag", want: []string{"🇺🇸 ", "flag"}},
		{text: "⚠️Title", want: []string{"⚠️Title"}},
	} {
		if got := markdownWrapTokens(source.text); !reflect.DeepEqual(got, source.want) {
			t.Errorf("markdownWrapTokens(%q)=%q, want %q", source.text, got, source.want)
		}
	}
}

func TestMarkdownAssetErrorTextUsesBodyLineHeight(t *testing.T) {
	a := markdownTestApp(t)
	span := document.MarkdownSpan{Kind: "diagram", Language: "mermaid", Text: "graph TD; A-->B"}
	v := a.ensureMarkdownView()
	if v.errors == nil {
		v.errors = make(map[string]string)
	}
	v.errors[markdownAssetKey(span)] = "test error"

	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(700, 300)},
	}
	dims := a.layoutMarkdownAsset(gtx, span, false)
	wantMinHeight := 2 * gtx.Sp(unit.Sp(14*a.tokens().Typography.BodyLineHeightScale))
	if dims.Size.Y < wantMinHeight {
		t.Fatalf("multiline diagram error height=%d, want at least %d from body line-height token", dims.Size.Y, wantMinHeight)
	}
}

func TestMarkdownSourceCaretIgnoresSoftWrapping(t *testing.T) {
	v := newMarkdownView("中文长段落\n第二行")
	v.editor.SetCaret(5, 5)
	line, column := v.sourceCaret()
	if line != 1 || column != 6 {
		t.Fatalf("source caret %d,%d", line, column)
	}
	v.editor.SetCaret(7, 7)
	line, column = v.sourceCaret()
	if line != 2 || column != 2 {
		t.Fatalf("source caret %d,%d", line, column)
	}
}

func TestMarkdownFormulaScalesWithScreenMetric(t *testing.T) {
	a := markdownTestApp(t)
	a.markdownView.images = map[string]image.Image{"math:x": image.NewRGBA(image.Rect(0, 0, 64, 48))}
	for _, scale := range []float32{1, 1.5, 2} {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(600, 700)}}
		dims := a.layoutMarkdownAsset(gtx, document.MarkdownSpan{Kind: "math", Text: "x"}, false)
		want := image.Pt(int(32*scale), int(24*scale))
		if dims.Size != want {
			t.Fatalf("scale %v math size=%v want=%v", scale, dims.Size, want)
		}
	}
}

func TestMarkdownDisplayFormulaDoesNotCenter(t *testing.T) {
	a := markdownTestApp(t)
	a.markdownView.images = map[string]image.Image{"math:display:x": image.NewRGBA(image.Rect(0, 0, 100, 80))}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(600, 0), Max: image.Pt(600, 700)}}
	dims := a.layoutMarkdownBlock(gtx, document.MarkdownBlock{Kind: "math", Text: "x"}, 0)
	if dims.Size.X != 50 {
		t.Fatalf("formula expanded into centered row: %v", dims.Size)
	}
}

func TestMarkdownNarrowRibbonCanReachLastAction(t *testing.T) {
	a := markdownTestApp(t)
	for _, scale := range []float32{1, 1.5, 2} {
		var router input.Router
		var ops op.Ops
		a.markdownView.toolbar.ScrollTo(len(markdownToolbarActions) - 1)
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(360*scale), int(float32(documentRibbonHeightDP)*scale)))}
		a.layoutMarkdownRibbon(gtx)
		router.Frame(&ops)
		found := false
		var visit func([]input.SemanticNode)
		visit = func(nodes []input.SemanticNode) {
			for _, n := range nodes {
				if n.Desc.Description == "公式" {
					found = true
					if n.Desc.Bounds.Empty() || n.Desc.Bounds.Max.Y > gtx.Constraints.Max.Y {
						t.Fatalf("formula caption clipped: %v", n.Desc.Bounds)
					}
				}
				visit(n.Children)
			}
		}
		visit(router.AppendSemantics(nil))
		if !found {
			t.Fatalf("last action unreachable at scale %v", scale)
		}
	}
}

func TestMarkdownTableAndParagraphLinksHaveIndependentTargets(t *testing.T) {
	a := markdownTestApp(t)
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(600, 700)}}
	a.layoutMarkdownBlock(gtx, document.MarkdownBlock{Kind: "table", Rows: [][]string{{"first", "table link"}}, TableSpans: [][][]document.MarkdownSpan{{{{Text: "first"}}, {{Text: "table link", Destination: "https://example.com/table"}}}}}, 0)
	a.layoutMarkdownBlock(gtx, document.MarkdownBlock{Kind: "paragraph", Spans: []document.MarkdownSpan{{Text: "paragraph link", Destination: "https://example.com/paragraph"}}}, 1)
	targets := map[string]bool{}
	for _, target := range a.markdownView.links {
		targets[target] = true
	}
	if !targets["https://example.com/table"] || !targets["https://example.com/paragraph"] {
		t.Fatalf("targets collided: %v", a.markdownView.links)
	}
}
