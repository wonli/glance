package desktopui

import (
	"fmt"
	"image"

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
	"glance/internal/format/docx"
)

func newWordJoinInteraction(t *testing.T, first, second string) (*docx.Document, *documentSessionState, *App, *input.Router, func()) {
	t.Helper()
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	firstRun := word.WordBlocks()[0].Paragraph.Inlines[0].Text
	if err := word.ReplaceTextRun(firstRun.ID, first); err != nil {
		t.Fatal(err)
	}
	paragraphID, err := any(word).(document.WordBodyEditor).InsertBodyParagraph()
	if err != nil {
		t.Fatal(err)
	}
	var secondRun *document.WordTextRun
	for _, block := range word.WordBlocks() {
		if block.Paragraph != nil && block.Paragraph.ID == paragraphID {
			secondRun = block.Paragraph.Inlines[0].Text
		}
	}
	if secondRun == nil {
		t.Fatal("new Word paragraph has no text run")
	}
	if err := word.ReplaceTextRun(secondRun.ID, second); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("join.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app, router, frame := newWordJoinFrame(t, state)
	return word, state, app, router, frame
}

func newWordJoinFrame(t *testing.T, state *documentSessionState) (*App, *input.Router, func()) {
	t.Helper()
	app := &App{documentSessionState: state, theme: material.NewTheme(), wordCommandResults: make(chan wordCommandResult, 4)}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(1100, 700)}}
		app.layoutWord(gtx)
		router.Frame(&ops)
	}
	return app, router, frame
}

func focusWordRunForInteraction(t *testing.T, state *documentSessionState, router *input.Router, frame func(), runID string) *widget.Editor {
	t.Helper()
	frame()
	var bounds image.Rectangle
	var findRun func([]input.SemanticNode)
	findRun = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "正文文字 "+runID {
				bounds = node.Desc.Bounds
			}
			findRun(node.Children)
		}
	}
	findRun(router.AppendSemantics(nil))
	if bounds.Empty() {
		t.Fatalf("Word text run %q has no accessible hit bounds", runID)
	}
	position := f32.Pt(float32(bounds.Min.X+bounds.Dx()/2), float32(bounds.Min.Y+bounds.Dy()/2))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
	frame()
	editor := state.wordRunEditors[runID]
	if editor == nil || !router.Source().Focused(editor) {
		t.Fatalf("Word text run %q did not receive focus", runID)
	}
	return editor
}

func waitWordInteraction(t *testing.T, app *App, done func() bool) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		app.processAsyncResults()
		if done() && len(app.wordCommandResults) == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Word paragraph join did not complete before timeout")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func newLongWordFlowState(t *testing.T, paragraphs int) (*documentSessionState, func()) {
	t.Helper()
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	blocks := make([]document.WordBlock, paragraphs)
	for i := range blocks {
		id := fmt.Sprintf("p%d", i)
		paragraph := document.WordParagraph{ID: id, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: id + ".r0", Text: strings.Repeat("scroll regression content ", 28), Editable: true}}}}
		blocks[i] = document.WordBlock{ID: fmt.Sprintf("b%d", i), Kind: document.WordParagraphBlock, Paragraph: &paragraph}
	}
	state := newDocumentSessionState("scroll.docx")
	state.word = word
	state.wordBlocks = blocks
	return state, func() { _ = word.Close() }
}
