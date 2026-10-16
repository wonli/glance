package desktopui

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/format/docx"
	"glance/internal/wordlayout"
)

func TestWordRibbonPrintLayoutButtonChangesViewMode(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("print-layout.docx")
	state.word = word
	state.documentID = "print-layout-ribbon-test"
	state.wordToolbarTab = wordTabView
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true}
	state.wordLayoutAcceptedKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevisionAtomic.Load()}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 2, PxPerSp: 2}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Source:      router.Source(),
			Ops:         &ops,
			Now:         time.Now(),
			Metric:      metric,
			Constraints: layout.Constraints{Max: image.Pt(2560, 1600)},
		}
		app.layoutWord(gtx)
		router.Frame(&ops)
	}

	frame()
	var position f32.Point
	var findPrintLayout func([]input.SemanticNode)
	findPrintLayout = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "打印布局" {
				b := node.Desc.Bounds
				position = f32.Pt(float32(b.Min.X+b.Dx()/2), float32(b.Min.Y+b.Dy()/2))
				return
			}
			findPrintLayout(node.Children)
		}
	}
	findPrintLayout(router.AppendSemantics(nil))
	if position == (f32.Point{}) {
		position = f32.Pt(188, 124)
	}
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
	frame()

	if state.wordViewMode != wordViewPrintLayout {
		t.Fatalf("Word view mode = %v after clicking Print Layout; want print layout", state.wordViewMode)
	}
}

func TestWordRibbonSymbolButtonOpensCharacterPanel(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	state := newDocumentSessionState("symbol-ribbon.docx")
	state.word = word
	state.wordToolbarTab = wordTabInsert
	state.wordToolbarActionList.List.Position.First = 2
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true}
	state.wordLayoutAcceptedKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevisionAtomic.Load()}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 2, PxPerSp: 2}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Source:      router.Source(),
			Ops:         &ops,
			Now:         time.Now(),
			Metric:      metric,
			Constraints: layout.Constraints{Max: image.Pt(2560, 1600)},
		}
		app.layoutWord(gtx)
		router.Frame(&ops)
	}

	frame()
	var symbolPos f32.Point
	var findSymbol func([]input.SemanticNode)
	findSymbol = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "符号" {
				b := node.Desc.Bounds
				symbolPos = f32.Pt(float32(b.Min.X+b.Dx()/2), float32(b.Min.Y+b.Dy()/2))
				return
			}
			findSymbol(node.Children)
		}
	}
	findSymbol(router.AppendSemantics(nil))
	if symbolPos == (f32.Point{}) {
		symbolPos = f32.Pt(1592, 124)
	}
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: symbolPos})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: symbolPos})
	frame()
	if !app.wordSymbolPickerVisible {
		t.Fatal("clicking Insert > Symbol did not open the character panel")
	}
}

func TestWordStyleGalleryPointerActionsPersistParagraphStyles(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	if err := word.SetParagraphStyles([]string{paragraph.ID}, "Heading1"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("style-gallery.docx")
	state.documentID = "style-gallery-pointer-test"
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	state.wordToolbarTab = wordTabHome
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true}
	state.wordLayoutAcceptedKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevisionAtomic.Load()}
	app := &App{documentSessionState: state, theme: material.NewTheme(), wordCommandResults: make(chan wordCommandResult, 1)}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 2, PxPerSp: 2}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric,
			Constraints: layout.Constraints{Max: image.Pt(2560, 1600)},
		}
		app.layoutWord(gtx)
		router.Frame(&ops)
	}

	for _, step := range []struct {
		action, label, style string
	}{
		{action: "style-body", label: "正文", style: "Normal"},
		{action: "style-heading", label: "标题 1", style: "Heading1"},
		{action: "style-heading-2", label: "标题 2", style: "Heading2"},
	} {
		frame()
		var bounds image.Rectangle
		var find func([]input.SemanticNode)
		find = func(nodes []input.SemanticNode) {
			for _, node := range nodes {
				if node.Desc.Description == step.label {
					bounds = node.Desc.Bounds
					return
				}
				find(node.Children)
			}
		}
		find(router.AppendSemantics(nil))
		if bounds.Empty() {
			t.Fatalf("Word style card %q has no semantic hit bounds", step.label)
		}
		position := f32.Pt(float32(bounds.Min.X+bounds.Dx()/2), float32(bounds.Min.Y+bounds.Dy()/2))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 51, Buttons: pointer.ButtonPrimary, Position: position})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 51, Position: position})
		frame()
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("click %s: %v", step.label, result.err)
			}
			app.wordCommandResults <- result
			app.processWordCommandAsyncResult()
		case <-time.After(3 * time.Second):
			t.Fatalf("clicking Word style card %q did not apply its paragraph style", step.label)
		}
		updated, ok := selectedWordParagraph(state.wordBlocks, paragraph.ID)
		if !ok || updated.Style != step.style {
			t.Fatalf("clicking %q produced style %q (found=%v); want %q", step.label, updated.Style, ok, step.style)
		}
		if !wordToolbarActionActive(state, ribbonAction{id: step.action}) {
			t.Fatalf("Word style card %q is not active after applying %s", step.label, step.style)
		}
	}

	path := filepath.Join(t.TempDir(), "style-gallery.docx")
	if err := word.SaveAs(path); err != nil {
		t.Fatalf("save styled DOCX: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen styled DOCX: %v", err)
	}
	defer reopened.Close()
	if got := reopened.WordBlocks()[0].Paragraph.Style; got != "Heading2" {
		t.Fatalf("reopened paragraph style = %q; want Heading2", got)
	}
}
