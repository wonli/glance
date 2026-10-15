package desktopui

import (
	"strings"
	"testing"

	"gioui.org/widget/material"
	"glance/internal/document"

	"glance/internal/fontcatalog"
	"glance/internal/format/docx"
	"glance/internal/wordlayout"
)

func TestWordNavigationRibbonActionTogglesPaneWithoutDirtyingDocument(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)

	state := newDocumentSessionState("navigation.docx")
	state.word = word
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	action := findWordViewTestAction(t, "view-navigation")
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("View > Navigation Pane should be enabled for a Word document")
	}
	if wordToolbarActionActive(state, action) {
		t.Fatal("Navigation Pane should be closed initially")
	}

	app.executeWordToolbarAction(action.id)
	if !wordToolbarActionActive(state, action) {
		t.Fatal("View > Navigation Pane did not open")
	}
	app.executeWordToolbarAction(action.id)
	if wordToolbarActionActive(state, action) {
		t.Fatal("View > Navigation Pane did not close")
	}
	if word.Dirty() {
		t.Fatal("toggling Navigation Pane changed DOCX dirty state")
	}
}

func TestWordNavigationMovesPrintLayoutPageAndStatusFollowsVisibleSpread(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)
	state := newDocumentSessionState("navigation-pages.docx")
	state.word = word
	state.wordViewMode = wordViewPrintLayout
	state.wordMultiplePagesMode = true
	state.wordBlocks = []document.WordBlock{{ID: "block", Kind: document.WordParagraphBlock, Paragraph: &document.WordParagraph{
		ID: "paragraph", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run", Text: "needle target", Editable: true}}},
	}}}
	pages := make([]wordlayout.PageScene, 7)
	for index := range pages {
		pages[index] = wordlayout.PageScene{Index: index, Clip: wordlayout.Rect{WidthPT: 300, HeightPT: 300}}
	}
	state.wordLayoutResult = &wordlayout.Result{
		CanPreview: true,
		Pages:      pages,
		Locations: map[wordlayout.SourcePosition]wordlayout.Position{
			{RunID: "run", RuneOffset: 0}: {PageIndex: 2},
			{RunID: "run", RuneOffset: 3}: {PageIndex: 0},
		},
	}
	state.wordLayoutAcceptedKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision, FontVersion: fontcatalog.Default().Version}
	state.wordNavigationSearchEditor.SetText("needle")
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	app.toggleWordNavigationPane()
	wordGridlineSemantics(app)
	button := state.wordNavigationButtons["result:paragraph:run:0"]
	if button == nil {
		t.Fatal("print-layout search result has no navigation control")
	}
	button.Click()
	wordGridlineSemantics(app)
	if state.wordPageListState.Position.First != 1 || state.wordLayoutActivePage != 2 {
		t.Fatalf("clicking page-three search result positioned row=%d activePage=%d; want row 1 / page index 2", state.wordPageListState.Position.First, state.wordLayoutActivePage)
	}
	state.wordRunEditors["run"].SetCaret(3, 3)
	wordGridlineSemantics(app)
	if got := wordPaginationStatusText(state); !strings.Contains(got, "第 3 页") {
		t.Fatalf("print status = %q; want the visible page, even when the selected caret maps elsewhere (accepted=%+v pending=%+v)", got, state.wordLayoutAcceptedKey, state.wordLayoutPendingKey)
	}
	if word.Dirty() {
		t.Fatal("navigating in Print Layout changed DOCX dirty state")
	}
}

func TestWordNavigationPaneShowsHeadingsAndLiveSearchResults(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)

	state := newDocumentSessionState("navigation-index.docx")
	state.word = word
	state.wordLayoutPendingKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision, FontVersion: fontcatalog.Default().Version}
	state.wordBlocks = []document.WordBlock{
		{ID: "heading-block", Kind: document.WordParagraphBlock, Paragraph: &document.WordParagraph{
			ID: "heading-paragraph", Style: "Heading1", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "heading-run", Text: "项目规划", Editable: true}}},
		}},
		{ID: "body-block", Kind: document.WordParagraphBlock, Paragraph: &document.WordParagraph{
			ID: "body-paragraph", Style: "Normal", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "body-run", Text: "推进目标与里程碑推进目标", Editable: true}}},
		}},
	}
	state.wordNavigationSearchEditor.SetText("推进目标")
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	action := findWordViewTestAction(t, "view-navigation")
	app.executeWordToolbarAction(action.id)

	semantics := wordGridlineSemantics(app)
	for _, want := range []string{"Word 导航窗格", "Word 导航搜索", "项目规划", "2 个搜索结果"} {
		if !strings.Contains(semantics, want) {
			t.Fatalf("Navigation Pane is missing %q: %s", want, semantics)
		}
	}
	button := state.wordNavigationButtons["result:body-paragraph:body-run:0"]
	if button == nil {
		t.Fatal("first search result does not have a navigation control")
	}
	button.Click()
	wordGridlineSemantics(app)
	if state.selectedWordRunID != "body-run" || state.selectedWordParagraphID != "body-paragraph" {
		t.Fatalf("search result selected %q/%q; want body-run/body-paragraph", state.selectedWordRunID, state.selectedWordParagraphID)
	}
	start, end := state.wordRunEditors["body-run"].Selection()
	if start != 0 || end != len([]rune("推进目标")) {
		t.Fatalf("search result selection = %d:%d; want 0:%d", start, end, len([]rune("推进目标")))
	}
	state.wordNavigationSearchEditor.SetText("里程碑")
	wordGridlineSemantics(app)
	if state.wordNavigationButtons["result:body-paragraph:body-run:0"] != nil {
		t.Fatal("updating live search retained stale result controls")
	}
	if state.wordNavigationButtons["result:body-paragraph:body-run:5"] == nil {
		t.Fatal("updating live search did not create the new matching result control")
	}
	if word.Dirty() {
		t.Fatal("reading a navigation heading or selecting a search result changed DOCX dirty state")
	}
}

func TestWordNavigationPaneCloseButtonClosesPane(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)
	state := newDocumentSessionState("navigation-close.docx")
	state.word = word
	state.wordLayoutPendingKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision, FontVersion: fontcatalog.Default().Version}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	app.toggleWordNavigationPane()
	wordGridlineSemantics(app)
	state.wordNavigationCloseButton.Click()
	wordGridlineSemantics(app)
	if state.wordNavigationVisible {
		t.Fatal("Navigation Pane close button did not close the pane")
	}
	if word.Dirty() {
		t.Fatal("closing Navigation Pane changed DOCX dirty state")
	}
}

func TestWordNavigationHeadingLevelFollowsBasedOnStyles(t *testing.T) {
	styles := []document.WordStyle{
		{ID: "CustomChapter", BasedOn: "Heading2"},
		{ID: "CycleA", BasedOn: "CycleB"},
		{ID: "CycleB", BasedOn: "CycleA"},
		{ID: "BodyText"},
	}
	for _, test := range []struct {
		style string
		want  int
	}{
		{style: "Heading1", want: 1},
		{style: "heading 2", want: 2},
		{style: "CustomChapter", want: 2},
		{style: "CycleA", want: 0},
		{style: "BodyText", want: 0},
		{style: "Heading10", want: 0},
	} {
		if got := wordNavigationHeadingLevel(test.style, styles); got != test.want {
			t.Errorf("heading level for style %q = %d; want %d", test.style, got, test.want)
		}
	}
}

func TestWordNavigationEntryCacheTracksRevisionAndLiveDrafts(t *testing.T) {
	state := newDocumentSessionState("navigation-cache.docx")
	state.wordRunCommitted["heading-run"] = "Old title"
	blocks := []document.WordBlock{{ID: "heading-block", Kind: document.WordParagraphBlock, Paragraph: &document.WordParagraph{
		ID: "heading", Style: "Heading1", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "heading-run", Text: "Old title", Editable: true}}},
	}}}
	first, _, _ := wordNavigationCachedEntries(state, blocks, "")
	second, _, _ := wordNavigationCachedEntries(state, blocks, "")
	if &first[0] != &second[0] {
		t.Fatal("unchanged navigation input rebuilt the document index")
	}
	captureWordTextDraft(state, "heading-run", "New title")
	third, _, _ := wordNavigationCachedEntries(state, blocks, "")
	if &third[0] == &first[0] || !strings.Contains(third[1].label, "New title") {
		t.Fatal("a live heading draft did not refresh the navigation index")
	}
	state.wordRevision = state.wordRevisionAtomic.Add(1)
	fourth, _, _ := wordNavigationCachedEntries(state, blocks, "")
	if &fourth[0] == &third[0] {
		t.Fatal("document revision did not refresh the navigation index")
	}
}

func TestWordNavigationSearchBoundsDisplayedResults(t *testing.T) {
	textValue := strings.TrimSpace(strings.Repeat("x ", wordNavigationResultLimit+1))
	blocks := []document.WordBlock{{ID: "results-block", Kind: document.WordParagraphBlock, Paragraph: &document.WordParagraph{
		ID: "results-paragraph", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "results-run", Text: textValue, Editable: true}}},
	}}}
	entries, _, resultCount := wordNavigationEntries(blocks, nil, "x", nil)
	displayedResults := 0
	for _, entry := range entries {
		if entry.kind == wordNavigationResult {
			displayedResults++
		}
	}
	if resultCount != wordNavigationResultLimit+1 || displayedResults != wordNavigationResultLimit {
		t.Fatalf("search counted %d matches and displayed %d; want %d and cap %d", resultCount, displayedResults, wordNavigationResultLimit+1, wordNavigationResultLimit)
	}
	if !strings.Contains(entries[2].label, "显示前 500 个") {
		t.Fatalf("bounded search header = %q; want an explicit display-limit message", entries[2].label)
	}
}
