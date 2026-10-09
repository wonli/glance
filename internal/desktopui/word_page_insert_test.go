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

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/wordlayout"
)

func TestWordPageBottomBlankClickInsertsAndFocusesEditableParagraph(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("blank.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())

	page := wordlayout.PageScene{
		Index:   0,
		Clip:    wordlayout.Rect{WidthPT: 200, HeightPT: 300},
		Content: wordlayout.Rect{XPT: 20, YPT: 20, WidthPT: 160, HeightPT: 260},
		Text: []wordlayout.TextFragment{{
			RunID:       "run-0",
			Text:        "首行文本",
			Editable:    true,
			BaselineYPT: 40,
			Style:       document.WordTextStyle{FontSize: 12},
		}},
	}
	state.wordLayoutResult = &wordlayout.Result{
		CanPreview: true,
		Pages:      []wordlayout.PageScene{page},
		Locations: map[wordlayout.SourcePosition]wordlayout.Position{
			{RunID: "run-0", RuneOffset: 0}: {PageIndex: 0, XPT: 20, YPT: 40},
		},
	}
	app := &App{
		documentSessionState: state,
		theme:                material.NewTheme(),
		wordCommandResults:   make(chan wordCommandResult, 2),
	}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Source:      router.Source(),
			Ops:         &ops,
			Now:         time.Now(),
			Metric:      metric,
			Constraints: layout.Constraints{Max: image.Pt(400, 500)},
		}
		app.layoutWordPages(gtx, state, wordPagePrint)
		router.Frame(&ops)
	}
	frame()

	if len(state.wordPageVisibleInteractions) == 0 {
		t.Fatal("no visible page interactions")
	}
	interaction := state.wordPageVisibleInteractions[0]
	// Click in the bottom blank area below text: Y = 150 (content bottom is ~44pt)
	bottomBlankScreen := interaction.Transform.PageToScreen(f32.Pt(50, 150))
	router.Queue(pointer.Event{
		Kind:      pointer.Press,
		Source:    pointer.Mouse,
		PointerID: 99,
		Buttons:   pointer.ButtonPrimary,
		Position:  bottomBlankScreen,
	})
	frame()
	router.Queue(pointer.Event{
		Kind:      pointer.Release,
		Source:    pointer.Mouse,
		PointerID: 99,
		Position:  bottomBlankScreen,
	})
	frame()

	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("insert paragraph failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("clicking blank page area timed out waiting for paragraph insert")
	}

	if len(state.wordBlocks) != 2 {
		t.Fatalf("wordBlocks count = %d; want 2", len(state.wordBlocks))
	}
	if state.selectedWordRunID == "" {
		t.Fatal("newly inserted paragraph text run is not selected")
	}
	if state.wordPendingFocusRunID != state.selectedWordRunID && state.wordLayoutFocusedRunID != state.selectedWordRunID {
		t.Fatalf("newly inserted run not focused: pending=%q layout=%q", state.wordPendingFocusRunID, state.wordLayoutFocusedRunID)
	}
}

func TestWordPageBottomBlankClickDeselectsImageWithoutInsert(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("blank.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordImageID = "image-1"

	page := wordlayout.PageScene{
		Index:   0,
		Clip:    wordlayout.Rect{WidthPT: 200, HeightPT: 300},
		Content: wordlayout.Rect{XPT: 20, YPT: 20, WidthPT: 160, HeightPT: 260},
		Text: []wordlayout.TextFragment{{
			RunID:       "run-0",
			Text:        "首行文本",
			Editable:    true,
			BaselineYPT: 40,
			Style:       document.WordTextStyle{FontSize: 12},
		}},
	}
	state.wordLayoutResult = &wordlayout.Result{
		CanPreview: true,
		Pages:      []wordlayout.PageScene{page},
	}
	app := &App{
		documentSessionState: state,
		theme:                material.NewTheme(),
		wordCommandResults:   make(chan wordCommandResult, 2),
	}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Source:      router.Source(),
			Ops:         &ops,
			Now:         time.Now(),
			Metric:      metric,
			Constraints: layout.Constraints{Max: image.Pt(400, 500)},
		}
		app.layoutWordPages(gtx, state, wordPagePrint)
		router.Frame(&ops)
	}
	frame()

	interaction := state.wordPageVisibleInteractions[0]
	bottomBlankScreen := interaction.Transform.PageToScreen(f32.Pt(50, 150))
	router.Queue(pointer.Event{
		Kind:      pointer.Press,
		Source:    pointer.Mouse,
		PointerID: 99,
		Buttons:   pointer.ButtonPrimary,
		Position:  bottomBlankScreen,
	})
	frame()
	router.Queue(pointer.Event{
		Kind:      pointer.Release,
		Source:    pointer.Mouse,
		PointerID: 99,
		Position:  bottomBlankScreen,
	})
	frame()

	if state.selectedWordImageID != "" {
		t.Fatalf("selectedWordImageID = %q; want empty after clicking blank area", state.selectedWordImageID)
	}
	if len(state.wordBlocks) != 1 {
		t.Fatalf("wordBlocks count = %d; want 1 (deselecting should not insert)", len(state.wordBlocks))
	}
}

func TestWordPageBottomBlankClickMiddlePageInsertsAfterPageParagraph(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	bodyEditor := any(word).(document.WordBodyEditor)
	if _, err := bodyEditor.InsertBodyParagraph(); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("two-pages.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	firstID := state.wordBlocks[0].Paragraph.ID

	firstRunID := state.wordBlocks[0].Paragraph.Inlines[0].Text.ID
	secondRunID := state.wordBlocks[1].Paragraph.Inlines[0].Text.ID

	page0 := wordlayout.PageScene{
		Index:   0,
		Clip:    wordlayout.Rect{WidthPT: 200, HeightPT: 300},
		Content: wordlayout.Rect{XPT: 20, YPT: 20, WidthPT: 160, HeightPT: 260},
		Text: []wordlayout.TextFragment{{
			RunID:       firstRunID,
			Text:        "第一页文本",
			Editable:    true,
			BaselineYPT: 40,
			Style:       document.WordTextStyle{FontSize: 12},
		}},
	}
	page1 := wordlayout.PageScene{
		Index:   1,
		Clip:    wordlayout.Rect{WidthPT: 200, HeightPT: 300},
		Content: wordlayout.Rect{XPT: 20, YPT: 20, WidthPT: 160, HeightPT: 260},
		Text: []wordlayout.TextFragment{{
			RunID:       secondRunID,
			Text:        "第二页文本",
			Editable:    true,
			BaselineYPT: 40,
			Style:       document.WordTextStyle{FontSize: 12},
		}},
	}
	state.wordLayoutResult = &wordlayout.Result{
		CanPreview: true,
		Pages:      []wordlayout.PageScene{page0, page1},
		Locations: map[wordlayout.SourcePosition]wordlayout.Position{
			{RunID: firstRunID, RuneOffset: 0}:  {PageIndex: 0, XPT: 20, YPT: 40},
			{RunID: secondRunID, RuneOffset: 0}: {PageIndex: 1, XPT: 20, YPT: 40},
		},
	}
	app := &App{
		documentSessionState: state,
		theme:                material.NewTheme(),
		wordCommandResults:   make(chan wordCommandResult, 2),
	}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Source:      router.Source(),
			Ops:         &ops,
			Now:         time.Now(),
			Metric:      metric,
			Constraints: layout.Constraints{Max: image.Pt(400, 700)},
		}
		app.layoutWordPages(gtx, state, wordPagePrint)
		router.Frame(&ops)
	}
	frame()

	var interaction0 wordVisiblePageInteraction
	for _, inter := range state.wordPageVisibleInteractions {
		if inter.PageIndex == 0 {
			interaction0 = inter
			break
		}
	}
	if interaction0.PageRect.Empty() {
		t.Fatal("page 0 not visible")
	}

	// Click in the bottom blank area of page 0
	bottomBlankScreen := interaction0.Transform.PageToScreen(f32.Pt(50, 150))
	router.Queue(pointer.Event{
		Kind:      pointer.Press,
		Source:    pointer.Mouse,
		PointerID: 101,
		Buttons:   pointer.ButtonPrimary,
		Position:  bottomBlankScreen,
	})
	frame()
	router.Queue(pointer.Event{
		Kind:      pointer.Release,
		Source:    pointer.Mouse,
		PointerID: 101,
		Position:  bottomBlankScreen,
	})
	frame()

	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("insert paragraph failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("clicking blank page area timed out waiting for paragraph insert")
	}

	if len(state.wordBlocks) != 3 {
		t.Fatalf("wordBlocks count = %d; want 3", len(state.wordBlocks))
	}
	// The inserted paragraph should be right after firstID (index 1)
	if state.wordBlocks[0].Paragraph.ID != firstID {
		t.Fatalf("first block ID = %q; want %q", state.wordBlocks[0].Paragraph.ID, firstID)
	}
	if state.wordBlocks[1].Paragraph.ID != state.selectedWordParagraphID {
		t.Fatalf("second block ID = %q; want selected %q", state.wordBlocks[1].Paragraph.ID, state.selectedWordParagraphID)
	}
}
