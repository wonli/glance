package desktopui

import (
	"image"
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
	position := f32.Pt(188, 124) // center of View > Print Layout at 2× density
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
	position := f32.Pt(1592, 124) // center of Insert > Symbol at 2× density
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
	frame()
	if !app.wordSymbolPickerVisible {
		t.Fatal("clicking Insert > Symbol did not open the character panel")
	}
}
