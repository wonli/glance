package desktopui

import (
	"fmt"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/format/markdowndoc"
	"image"
	"testing"
	"time"
)

func TestMarkdownTaskPreviewClickChangesOnlyMarkerAndRetainsSelection(t *testing.T) {
	source := "\ufeff# 中文\r\n\r\n- [ ] 待办\r\n  - [X] 子项\r\n\r\n```\r\n- [ ] literal\r\n```\r\n"
	for _, scale := range []float32{1, 1.5, 2} {
		a := markdownTestApp(t)
		v := a.markdownView
		v.editor.SetText(source)
		a.flushMarkdownDraft()
		v.editor.SetCaret(2, 4)
		p, err := markdowndoc.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		v.requested = source
		v.accept(markdownParseResult{source: source, snapshot: p, images: map[string]image.Image{}})
		var ops op.Ops
		var router input.Router
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(600, 900)}}
		for i, b := range p.Blocks {
			a.layoutMarkdownBlock(gtx, b, i)
		}
		offset := len([]rune("\ufeff# 中文\r\n\r\n- ["))
		id := fmt.Sprintf("task-%d", offset)
		click := v.buttons[id]
		if click == nil {
			t.Fatal("preview has no interactive checkbox")
		}
		for cycle := 0; cycle < 4; cycle++ {
			click.Click()
			a.handleMarkdownEvents(gtx)
			want := []rune(source)
			if cycle%2 == 0 {
				want[offset] = 'x'
			}
			if v.text() != string(want) || a.markdownDoc.Text() != string(want) {
				t.Fatalf("cycle%d changed wrong source: %q", cycle, v.text())
			}
			start, end := v.editor.Selection()
			if start != 2 || end != 4 {
				t.Fatalf("selection moved: %d,%d", start, end)
			}
		}
		if !a.isDirty() {
			t.Fatal("task edit was not marked dirty")
		}
		if v.parseCancel != nil {
			v.parseCancel()
		}
	}
}
func TestMarkdownFormulaPreviewReopensAndReplacesOriginal(t *testing.T) {
	for _, tc := range []struct {
		source  string
		block   int
		id      string
		display bool
		want    string
	}{{"中文 $x^2$ 后文", 0, "math-3-8", false, "中文 $y_1$ 后文"}, {"前文\r\n\r\n$$\r\nx^2\r\n$$\r\n\r\n后文", 1, "math-6-17", true, "前文\r\n\r\n$$\r\ny_1\r\n$$\r\n\r\n后文"}} {
		a := markdownTestApp(t)
		v := a.markdownView
		v.editor.SetText(tc.source)
		a.flushMarkdownDraft()
		p, err := markdowndoc.Parse(tc.source)
		if err != nil {
			t.Fatal(err)
		}
		v.requested = tc.source
		v.accept(markdownParseResult{source: tc.source, snapshot: p, images: map[string]image.Image{}})
		var ops op.Ops
		var router input.Router
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(600, 900)}}
		a.layoutMarkdownBlock(gtx, p.Blocks[tc.block], tc.block)
		button := v.buttons[tc.id]
		if button == nil {
			t.Fatalf("formula %q is not editable; buttons=%v", tc.id, v.buttons)
		}
		button.Click()
		a.handleMarkdownEvents(gtx)
		if !a.modal.Visible || a.modal.FormInputs[0].Text() != "x^2" {
			t.Fatal("formula edit dialog did not retain expression")
		}
		a.modal.FormInputs[0].SetText("y_1")
		a.modal.OnAction("confirm")
		if v.text() != tc.want || a.markdownDoc.Text() != tc.want {
			t.Fatalf("formula replacement got %q want %q", v.text(), tc.want)
		}
		if v.parseCancel != nil {
			v.parseCancel()
		}
	}
}

func TestMarkdownTaskPointerSurvivesResourceRefresh(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		a := markdownTestApp(t)
		v := a.markdownView
		source := "- [ ] 中文"
		v.editor.SetText(source)
		a.flushMarkdownDraft()
		v.focus = false
		v.requested = source
		p, _ := markdowndoc.Parse(source)
		v.accept(markdownParseResult{source: source, snapshot: p, images: map[string]image.Image{}})
		var ops op.Ops
		var router input.Router
		frame := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(600, 900)}}
			a.handleMarkdownEvents(gtx)
			a.layoutMarkdownBlock(gtx, v.snapshot.Blocks[0], 0)
			router.Frame(&ops)
		}
		frame()
		button := v.buttons["task-3"]
		position := f32.Pt(8*scale, 10*scale)
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
		frame()
		v.accept(markdownParseResult{source: source, generation: v.parseGeneration, snapshot: v.snapshot, images: v.images})
		if v.buttons["task-3"] != button {
			t.Fatal("resource refresh replaced checkbox widget")
		}
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
		frame()
		frame()
		if v.text() != "- [x] 中文" {
			t.Fatalf("pointer click lost at scale %v: %q", scale, v.text())
		}
		if v.parseCancel != nil {
			v.parseCancel()
		}
	}
}
func TestMarkdownFormulaToolbarEditsAndCancelRetainsSource(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	source := "中文 $x^2$ 后文"
	v.editor.SetText(source)
	a.flushMarkdownDraft()
	v.editor.SetCaret(5, 5)
	p, _ := markdowndoc.Parse(source)
	v.requested = source
	v.accept(markdownParseResult{source: source, snapshot: p, images: map[string]image.Image{}})
	var ops op.Ops
	gtx := layout.Context{Ops: &ops}
	a.handleMarkdownAction(gtx, "math")
	if !a.modal.Visible || a.modal.FormInputs[0].Text() != "x^2" {
		t.Fatal("toolbar did not recognize existing formula")
	}
	a.modal.FormInputs[0].SetText("y_1")
	a.modal.OnAction("cancel")
	if v.text() != source {
		t.Fatal("cancel changed formula")
	}
	start, end := v.editor.Selection()
	if start != 5 || end != 5 {
		t.Fatal("cancel moved source caret")
	}
	a.handleMarkdownAction(gtx, "math")
	v.editor.SetText(source + "改动")
	a.modal.OnAction("confirm")
	if v.text() != source+"改动" {
		t.Fatal("stale formula edit replaced newer text")
	}
}

func TestMarkdownTableFormulaToolbarReopensExistingExpression(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	source := "| 值 |\n| --- |\n| $x^2$ |\n"
	v.editor.SetText(source)
	a.flushMarkdownDraft()
	p, _ := markdowndoc.Parse(source)
	v.requested = source
	v.accept(markdownParseResult{source: source, snapshot: p, images: map[string]image.Image{}})
	formula := p.Blocks[0].TableSpans[1][0][0]
	v.editor.SetCaret(formula.SourceStart+2, formula.SourceStart+2)
	var ops op.Ops
	a.handleMarkdownAction(layout.Context{Ops: &ops}, "math")
	if !a.modal.Visible || a.modal.FormInputs[0].Text() != "x^2" {
		t.Fatal("table formula was treated as new insertion")
	}
	a.modal.FormInputs[0].SetText("y_1")
	a.modal.OnAction("confirm")
	if v.text() != "| 值 |\n| --- |\n| $y_1$ |\n" {
		t.Fatalf("table formula replacement %q", v.text())
	}
	if v.parseCancel != nil {
		v.parseCancel()
	}
}
