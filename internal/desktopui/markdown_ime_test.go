package desktopui

import (
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"image"
	"testing"
	"time"
)

func TestMarkdownCompositionDoesNotCommitIntermediateText(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 250))}
		for {
			if _, ok := v.editor.Update(gtx); !ok {
				break
			}
		}
		a.flushMarkdownDraft()
		material.Editor(a.theme, &v.editor, "").Layout(gtx)
		router.Frame(&ops)
	}
	frame()
	router.Source().Execute(key.FocusCmd{Tag: &v.editor})
	frame()
	router.Queue(key.CompositionEvent{Start: 0, End: 1}, key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "中"})
	frame()
	if !markdownEditorComposing(&v.editor) {
		t.Fatal("composition state not detected")
	}
	if a.markdownDoc.Text() != "" {
		t.Fatalf("intermediate committed: %q", a.markdownDoc.Text())
	}
	if a.prepareMarkdownSave() {
		t.Fatal("save allowed during composition")
	}
	router.Queue(key.CompositionEvent{Start: -1, End: -1})
	frame()
	if a.markdownDoc.Text() != "中" {
		t.Fatalf("final text missing %q", a.markdownDoc.Text())
	}
}
