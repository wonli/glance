package desktopui

import (
	"image"

	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/io/input"
	"gioui.org/io/key"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/wordlayout"
)

func TestWordRightArrowMovesToNextStyledRun(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: "left", Editable: true}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: "right", Style: document.WordTextStyle{Bold: true}, Editable: true}},
	}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	_, router, frame := newWordJoinFrame(t, state)
	first := focusWordRunForInteraction(t, state, router, frame, "run-a")
	first.SetCaret(first.Len(), first.Len())
	frame()

	router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press})
	frame()
	next := state.wordRunEditors["run-b"]
	if next == nil || !router.Source().Focused(next) {
		t.Fatal("Right at a run end did not focus the next styled Word run")
	}
	if start, end := next.Selection(); start != 0 || end != 0 {
		t.Fatalf("caret after moving right between runs = %d:%d; want 0:0", start, end)
	}
	if state.selectedWordRunID != "run-b" || state.selectedWordParagraphID != "p0" {
		t.Fatalf("selection after run handoff = %q/%q; want run-b/p0", state.selectedWordRunID, state.selectedWordParagraphID)
	}
}

func TestWordLeftArrowMovesToPreviousParagraphRun(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	first := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: "first", Editable: true}}}}
	second := document.WordParagraph{ID: "p1", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: "second", Editable: true}}}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{
		{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &first},
		{ID: "p1", Kind: document.WordParagraphBlock, Paragraph: &second},
	}
	_, router, frame := newWordJoinFrame(t, state)
	secondEditor := focusWordRunForInteraction(t, state, router, frame, "run-b")
	secondEditor.SetCaret(0, 0)
	frame()

	router.Queue(key.Event{Name: key.NameLeftArrow, State: key.Press})
	frame()
	previous := state.wordRunEditors["run-a"]
	if previous == nil || !router.Source().Focused(previous) {
		t.Fatal("Left at a paragraph start did not focus the previous paragraph run")
	}
	if start, end := previous.Selection(); start != previous.Len() || end != previous.Len() {
		t.Fatalf("caret after moving left between paragraphs = %d:%d; want %d:%d", start, end, previous.Len(), previous.Len())
	}
}

func TestWordDocumentBoundaryShortcutsNavigateAndExtendSelection(t *testing.T) {
	for _, test := range []struct {
		name            string
		startInFirstRun bool
		caret           int
		keyName         key.Name
		modifiers       key.Modifiers
		wantFirstRun    bool
		wantOffset      int
		wantSelection   string
	}{
		{name: "Ctrl or Cmd Home moves to document start", caret: 3, keyName: key.NameHome, modifiers: key.ModShortcut, wantFirstRun: true, wantOffset: 0},
		{name: "Ctrl or Cmd End moves to document end", startInFirstRun: true, caret: 2, keyName: key.NameEnd, modifiers: key.ModShortcut, wantOffset: 6},
		{name: "Ctrl or Cmd Shift Home extends to document start", caret: 6, keyName: key.NameHome, modifiers: key.ModShortcut | key.ModShift, wantFirstRun: true, wantSelection: "first\nsecond"},
		{name: "Ctrl or Cmd Shift End extends to document end", startInFirstRun: true, caret: 0, keyName: key.NameEnd, modifiers: key.ModShortcut | key.ModShift, wantOffset: 6, wantSelection: "first\nsecond"},
	} {
		t.Run(test.name, func(t *testing.T) {
			word, state, _, router, frame := newWordJoinInteraction(t, "first", "second")
			defer word.Close()
			firstRunID := state.wordBlocks[0].Paragraph.Inlines[0].Text.ID
			lastBlock := state.wordBlocks[len(state.wordBlocks)-1]
			lastRunID := lastBlock.Paragraph.Inlines[0].Text.ID
			currentRunID := lastRunID
			if test.startInFirstRun {
				currentRunID = firstRunID
			}
			current := focusWordRunForInteraction(t, state, router, frame, currentRunID)
			current.SetCaret(test.caret, test.caret)
			frame()
			router.Queue(key.Event{Name: key.Name(test.keyName), State: key.Press, Modifiers: test.modifiers})
			frame()
			// A destination run later in the flow list receives its focus op in
			// the invalidated follow-up frame.
			frame()
			wantRunID := lastRunID
			if test.wantFirstRun {
				wantRunID = firstRunID
			}
			if state.selectedWordRunID != wantRunID {
				t.Fatalf("Ctrl/Cmd+%s selected run %q; want %q", test.keyName, state.selectedWordRunID, wantRunID)
			}
			if test.wantSelection != "" {
				if got, ok := wordPageSelectedText(state); !ok || got != test.wantSelection {
					t.Fatalf("Ctrl/Cmd+Shift+%s selected %q, %v; want %q", test.keyName, got, ok, test.wantSelection)
				}
				return
			}
			target := state.wordRunEditors[wantRunID]
			if target == nil || !router.Source().Focused(target) {
				t.Fatalf("Ctrl/Cmd+%s did not focus target run %q", test.keyName, wantRunID)
			}
			start, end := target.Selection()
			if start != test.wantOffset || end != test.wantOffset {
				t.Fatalf("Ctrl/Cmd+%s caret = %d:%d; want %d:%d", test.keyName, start, end, test.wantOffset, test.wantOffset)
			}
		})
	}
}

func TestWordPrintLayoutDocumentBoundaryShortcuts(t *testing.T) {
	for _, test := range []struct {
		name          string
		startInFirst  bool
		caret         int
		keyName       key.Name
		modifiers     key.Modifiers
		wantFirstRun  bool
		wantOffset    int
		wantSelection string
	}{
		{name: "Cmd Home moves to document start", caret: 6, keyName: key.NameHome, modifiers: key.ModShortcut, wantFirstRun: true},
		{name: "Cmd Shift End selects to document end", startInFirst: true, caret: 0, keyName: key.NameEnd, modifiers: key.ModShortcut | key.ModShift, wantSelection: "first\nsecond", wantOffset: 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			word, state, app, router, _ := newWordJoinInteraction(t, "first", "second")
			defer word.Close()
			runs := wordNavigationRuns(state.wordBlocks)
			firstRun, lastRun := runs[0].run, runs[len(runs)-1].run
			currentRun, targetRun := lastRun, firstRun
			if test.startInFirst {
				currentRun, targetRun = firstRun, lastRun
			}
			page := wordlayout.PageScene{
				Index: 0, Clip: wordlayout.Rect{WidthPT: 600, HeightPT: 800},
				Content: wordlayout.Rect{WidthPT: 500, HeightPT: 700},
			}
			locations := make(map[wordlayout.SourcePosition]wordlayout.Position)
			for runIndex, item := range runs {
				length := utf8.RuneCountInString(item.run.Text)
				fragment := wordlayout.TextFragment{RunID: item.run.ID, Text: item.run.Text, RuneStart: 0, RuneEnd: length, Editable: true, XPT: 20, BaselineYPT: float64(30 + runIndex*24)}
				fragment.Style.FontSize = 12
				page.Text = append(page.Text, fragment)
				for offset := 0; offset <= length; offset++ {
					locations[wordlayout.SourcePosition{RunID: item.run.ID, RuneOffset: offset}] = wordlayout.Position{PageIndex: 0, XPT: 20 + float64(offset*8), YPT: fragment.BaselineYPT}
				}
			}
			state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{page}, Locations: locations}
			state.selectedWordRunID = currentRun.ID
			state.selectedWordParagraphID = runs[len(runs)-1].paragraphID
			if test.startInFirst {
				state.selectedWordParagraphID = runs[0].paragraphID
			}
			editor := wordRunEditor(state, currentRun)
			editor.SetCaret(test.caret, test.caret)
			state.wordPendingFocusRunID = currentRun.ID
			state.wordPendingCaretRunID = currentRun.ID
			state.wordPendingCaretOffset = test.caret
			var ops op.Ops
			frame := func() {
				t.Helper()
				ops.Reset()
				gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(800, 1100)}}
				app.layoutWordPageFocusedEditor(gtx, state, page, 0, image.Rect(0, 0, 800, 1100), 1)
				router.Frame(&ops)
			}
			frame()
			if !router.Source().Focused(editor) {
				t.Fatal("print-layout editor did not receive initial focus")
			}
			router.Queue(key.Event{Name: test.keyName, State: key.Press, Modifiers: test.modifiers})
			frame()
			frame()
			wantRun := lastRun
			if test.wantFirstRun {
				wantRun = firstRun
			}
			if state.selectedWordRunID != wantRun.ID {
				t.Fatalf("print-layout Ctrl/Cmd+%s selected run %q; want %q", test.keyName, state.selectedWordRunID, wantRun.ID)
			}
			if test.wantSelection != "" {
				if got, ok := wordPageSelectedText(state); !ok || got != test.wantSelection {
					t.Fatalf("print-layout Ctrl/Cmd+Shift+%s selected %q, %v; want %q", test.keyName, got, ok, test.wantSelection)
				}
				return
			}
			target := state.wordRunEditors[targetRun.ID]
			if target == nil || !router.Source().Focused(target) {
				t.Fatalf("print-layout Ctrl/Cmd+%s did not focus target run %q", test.keyName, targetRun.ID)
			}
			start, end := target.Selection()
			if start != test.wantOffset || end != test.wantOffset {
				t.Fatalf("print-layout Ctrl/Cmd+%s caret = %d:%d; want %d:%d", test.keyName, start, end, test.wantOffset, test.wantOffset)
			}
		})
	}
}

func TestWordHistoryShortcutsUseDocumentHistory(t *testing.T) {
	for _, test := range []struct {
		name      string
		keyName   string
		modifiers key.Modifiers
		undoFirst bool
		want      string
	}{
		{name: "undo with Z", keyName: "Z", modifiers: key.ModShortcut, want: "before"},
		{name: "redo with Y", keyName: "Y", modifiers: key.ModShortcut, undoFirst: true, want: "after"},
		{name: "redo with Shift Z", keyName: "Z", modifiers: key.ModShortcut | key.ModShift, undoFirst: true, want: "after"},
	} {
		t.Run(test.name, func(t *testing.T) {
			word, err := docx.NewBlank()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = word.Close() })
			runID := word.WordBlocks()[0].Paragraph.Inlines[0].Text.ID
			if err := word.ReplaceTextRun(runID, "before"); err != nil {
				t.Fatal(err)
			}
			state := newDocumentSessionState("redo.docx")
			state.word = word
			state.setWordBlocks(cloneWordBlockSnapshot(word.WordBlocks()))
			app, router, frame := newWordJoinFrame(t, state)
			app.shutdown = make(chan struct{})
			t.Cleanup(func() {
				close(app.shutdown)
				app.stopAndWaitManagedBackground()
			})
			if err := word.ReplaceTextRun(runID, "after"); err != nil {
				t.Fatal(err)
			}
			if test.undoFirst {
				if applied, err := any(state.word).(document.WordHistory).Undo(); err != nil || !applied {
					t.Fatalf("prepare redo history with undo = %v, %v", applied, err)
				}
			}
			editor := focusWordRunForInteraction(t, state, router, frame, runID)
			if !router.Source().Focused(editor) {
				t.Fatal("Word editor lost focus before history shortcut")
			}
			router.Queue(key.Event{Name: key.Name(test.keyName), State: key.Press, Modifiers: test.modifiers})
			frame()
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				app.processAsyncResults()
				if state.word.WordBlocks()[0].Paragraph.Inlines[0].Text.Text == test.want && !state.formatRunning.Load() && len(app.wordCommandResults) == 0 {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if got := state.word.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != test.want {
				t.Fatalf("Ctrl/Cmd+%s document text = %q, want %q", test.keyName, got, test.want)
			}
		})
	}
}

func TestWordModifiedArrowKeepsNativeSelectionAndDocumentEdges(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: "selection", Editable: true}}}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	_, router, frame := newWordJoinFrame(t, state)
	editor := focusWordRunForInteraction(t, state, router, frame, "run-a")
	editor.SetCaret(0, 0)
	frame()

	router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press, Modifiers: key.ModShift})
	frame()
	if start, end := editor.Selection(); min(start, end) != 0 || max(start, end) != 1 {
		t.Fatalf("Shift+Right selection = %d:%d; want native selection across offsets 0 and 1", start, end)
	}
	editor.SetCaret(0, 0)
	frame()
	router.Queue(key.Event{Name: key.NameLeftArrow, State: key.Press})
	frame()
	if !router.Source().Focused(editor) {
		t.Fatal("Left at the document start moved focus outside the document")
	}
	if start, end := editor.Selection(); start != 0 || end != 0 {
		t.Fatalf("caret at document start after Left = %d:%d; want 0:0", start, end)
	}
}

func TestWordSelectAllShortcutSelectsTheWholeDocumentAfterPendingInput(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: "left", Editable: true}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: " right", Editable: true}},
	}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	_, router, frame := newWordJoinFrame(t, state)
	editor := focusWordRunForInteraction(t, state, router, frame, "run-a")
	router.Queue(
		key.EditEvent{Text: "新中文 ", Range: key.Range{Start: 0, End: 0}},
		key.Event{Name: key.Name("A"), State: key.Press, Modifiers: key.ModShortcut},
	)
	frame()
	if state.wordPageTextSelection == nil || state.wordPageTextSelection.scope != "document" {
		t.Fatalf("Cmd/Ctrl+A selection = %+v; want a document-scope selection", state.wordPageTextSelection)
	}
	if got, ok := wordPageSelectedText(state); !ok || got != "新中文 left right" {
		t.Fatalf("document text selected after pending input = %q, %v; want %q", got, ok, "新中文 left right")
	}
	if state.selectedWordRunID != "run-b" {
		t.Fatalf("active run after select all = %q; want final editable run-b", state.selectedWordRunID)
	}
	if got := editor.Text(); got != "新中文 left" {
		t.Fatalf("focused run after pending input = %q; want %q", got, "新中文 left")
	}
	lastEditor := state.wordRunEditors["run-b"]
	if lastEditor == nil || !router.Source().Focused(lastEditor) {
		t.Fatal("Cmd/Ctrl+A did not move the active editor to the final selected run")
	}
}

func TestWordPageSelectAllShortcutKeepsTheDocumentSelection(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: "left", Editable: true}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: " right", Editable: true}},
	}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	state.selectedWordRunID = "run-a"
	editor := wordRunEditor(state, *paragraph.Inlines[0].Text)
	page := wordlayout.PageScene{
		Index: 0, Clip: wordlayout.Rect{WidthPT: 595, HeightPT: 842},
		Content: wordlayout.Rect{WidthPT: 500, HeightPT: 700},
		Text: []wordlayout.TextFragment{
			{RunID: "run-a", Text: "left", RuneStart: 0, RuneEnd: 4, Editable: true},
			{RunID: "run-b", Text: " right", RuneStart: 0, RuneEnd: 6, Editable: true},
		},
	}
	locations := make(map[wordlayout.SourcePosition]wordlayout.Position)
	for offset := 0; offset <= 16; offset++ {
		locations[wordlayout.SourcePosition{RunID: "run-a", RuneOffset: offset}] = wordlayout.Position{PageIndex: 0, XPT: float64(offset), YPT: 24}
	}
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{page}, Locations: locations}
	state.wordPageFocusedMetaRunID = "run-a"
	state.wordPageFocusedParagraph = paragraph
	state.wordPendingFocusRunID = "run-a"
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Source: router.Source(), Ops: &ops, Now: time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(800, 1100)},
		}
		app.layoutWordPageFocusedEditor(gtx, state, page, 0, image.Rect(0, 0, 800, 1100), 1)
		router.Frame(&ops)
	}
	frame()
	if !router.Source().Focused(editor) {
		t.Fatal("print-layout editor did not receive focus")
	}
	router.Queue(
		key.EditEvent{Text: "新中文 ", Range: key.Range{Start: 0, End: 0}},
		key.Event{Name: key.Name("A"), State: key.Press, Modifiers: key.ModShortcut},
	)
	frame()
	if state.wordPageTextSelection == nil || state.wordPageTextSelection.scope != "document" {
		t.Fatalf("Cmd/Ctrl+A page selection = %+v; want a document-scope selection", state.wordPageTextSelection)
	}
	if got, ok := wordPageSelectedText(state); !ok || got != "新中文 left right" {
		t.Fatalf("page-layout text selected after pending input = %q, %v; want %q", got, ok, "新中文 left right")
	}
}

func TestWordNavigationRunOrderIncludesEditableTableCellsOnly(t *testing.T) {
	paragraph := func(id, text string, editable bool) document.WordParagraph {
		return document.WordParagraph{ID: id, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: id + ".run", Text: text, Editable: editable}}}}
	}
	first := paragraph("body", "body", true)
	readOnly := paragraph("locked", "locked", false)
	cellA := document.WordTableCell{ID: "a", Paragraphs: []document.WordParagraph{paragraph("a.p0", "A1", true), paragraph("a.p1", "A2", true)}}
	cellB := document.WordTableCell{ID: "b", Paragraphs: []document.WordParagraph{paragraph("b.p0", "B1", true)}}
	blocks := []document.WordBlock{
		{ID: "body", Kind: document.WordParagraphBlock, Paragraph: &first},
		{ID: "locked", Kind: document.WordParagraphBlock, Paragraph: &readOnly},
		{ID: "table", Kind: document.WordTableBlock, Table: &document.WordTable{Rows: []document.WordTableRow{{Cells: []document.WordTableCell{cellA, cellB}}}}},
	}
	var got []string
	for _, item := range wordNavigationRuns(blocks) {
		got = append(got, item.run.ID)
	}
	want := []string{"body.run", "a.p0.run", "a.p1.run", "b.p0.run"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Word navigation order = %#v; want row-major editable runs %#v", got, want)
	}
}

func TestWordUpDownKeepsPreferredVisualColumnAcrossVisualLines(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	text := strings.Repeat("abcdefghij", 4) + "\n" + strings.Repeat("abcdefghij", 3) + "\n" + strings.Repeat("abc", 20)
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: text, Editable: true}}}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(620, 700)}}
		app.layoutWord(gtx)
		router.Frame(&ops)
	}
	editor := focusWordRunForInteraction(t, state, router, frame, "run-a")
	editor.SetCaret(8, 8)
	frame()
	geometry := state.wordNavLayouts["run-a"]
	start := editor.CaretCoords()
	startY := geometry.y + int(start.Y)
	for move := 0; move < 2; move++ {
		router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
		frame()
		coords := editor.CaretCoords()
		if gotY := geometry.y + int(coords.Y); gotY <= startY {
			t.Fatalf("Down %d left caret on the same visual line: y=%d, start=%d, caret=%v geometry=%+v", move+1, gotY, startY, coords, geometry)
		}
		if !state.wordPreferredVerticalXSet {
			t.Fatal("Up/Down did not retain a preferred horizontal column")
		}
		visualX := geometry.x + int(coords.X)
		if distance := absWordInt(visualX - state.wordPreferredVerticalX); distance > 10 {
			t.Fatalf("Down %d moved to x=%d; preferred visual x=%d (distance %d)", move+1, visualX, state.wordPreferredVerticalX, distance)
		}
		startY = geometry.y + int(coords.Y)
		geometry = state.wordNavLayouts["run-a"]
	}
}

func TestWordUpDownCrossesParagraphsAtPreferredVisualColumn(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	text := strings.Repeat("0123456789", 10)
	first := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: text, Editable: true}}}}
	second := document.WordParagraph{ID: "p1", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: text, Editable: true}}}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &first}, {ID: "p1", Kind: document.WordParagraphBlock, Paragraph: &second}}
	_, router, frame := newWordJoinFrame(t, state)
	firstEditor := focusWordRunForInteraction(t, state, router, frame, "run-a")
	firstEditor.SetCaret(100, 100)
	frame()
	preferred := state.wordNavLayouts["run-a"].x + int(firstEditor.CaretCoords().X)

	router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
	frame()
	frame()
	secondEditor := state.wordRunEditors["run-b"]
	if secondEditor == nil || !router.Source().Focused(secondEditor) {
		t.Fatalf("Down at the last visual line did not focus the next paragraph: selected=%q/%q pending=%q/%q preferred=%v/%d sourceFocusedA=%v bounds=%#v", state.selectedWordRunID, state.selectedWordParagraphID, state.wordPendingFocusRunID, state.wordPendingCaretRunID, state.wordPreferredVerticalXSet, state.wordPreferredVerticalX, router.Source().Focused(firstEditor), state.wordNavLayouts)
	}
	if !state.wordPreferredVerticalXSet {
		t.Fatal("vertical caret column was cleared while moving to the next paragraph")
	}
	secondX := state.wordNavLayouts["run-b"].x + int(secondEditor.CaretCoords().X)
	if distance := absWordInt(secondX - preferred); distance > 10 {
		t.Fatalf("next paragraph caret x=%d; preferred x=%d (distance %d)", secondX, preferred, distance)
	}

	router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press})
	frame()
	frame()
	if !router.Source().Focused(firstEditor) {
		t.Fatal("Up at the first visual line did not return to the previous paragraph")
	}
	firstX := state.wordNavLayouts["run-a"].x + int(firstEditor.CaretCoords().X)
	if distance := absWordInt(firstX - preferred); distance > 10 {
		t.Fatalf("previous paragraph caret x=%d; preferred x=%d (distance %d)", firstX, preferred, distance)
	}
}
