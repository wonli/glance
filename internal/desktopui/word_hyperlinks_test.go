package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/document"
)

func TestWordExternalHyperlinkURLAllowsOnlySupportedSchemes(t *testing.T) {
	for _, target := range []string{
		"https://example.test/path?q=1",
		"http://example.test/path",
		"mailto:hello@example.test",
	} {
		got, err := wordExternalHyperlinkURL(target)
		if err != nil || got != target {
			t.Errorf("wordExternalHyperlinkURL(%q) = %q, %v; want same URL", target, got, err)
		}
	}
	for _, target := range []string{"javascript:alert(1)", "file:///etc/passwd", "//example.test/path", "https:///missing-host"} {
		if got, err := wordExternalHyperlinkURL(target); err == nil {
			t.Errorf("wordExternalHyperlinkURL(%q) = %q; want rejection", target, got)
		}
	}
}

func TestWordHyperlinkActivationRequiresPlatformShortcut(t *testing.T) {
	if !wordHyperlinkActivationShortcut(key.ModShortcut) {
		t.Fatal("Command/Ctrl modifier did not activate a Word hyperlink")
	}
	if wordHyperlinkActivationShortcut(0) || wordHyperlinkActivationShortcut(key.ModShift) {
		t.Fatal("unmodified click activated a Word hyperlink")
	}
}

func TestWordPageHyperlinkShortcutOpensAndPlainClickRemainsSelectable(t *testing.T) {
	app, state, router, frame := newWordSelectionPageTest(t, `<w:p><w:r><w:t>link text</w:t></w:r></w:p>`)
	const target = "https://example.test/word"
	run := state.wordBlocks[0].Paragraph.Inlines[0].Text
	run.HyperlinkTarget = target
	opened := make(chan string, 1)
	app.wordHyperlinkLauncher = func(value string) error {
		opened <- value
		return nil
	}
	transform := state.wordPageVisibleInteractions[0].Transform
	start := transform.PageToScreen(f32.Pt(12, 30))
	end := transform.PageToScreen(f32.Pt(99, 30))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 201, Buttons: pointer.ButtonPrimary, Position: start})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 201, Buttons: pointer.ButtonPrimary, Position: end})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 201, Position: end})
	frame()
	if state.wordRunEditors[run.ID] == nil {
		t.Fatal("drag on hyperlink text did not create its editor")
	}
	if selected, ok := wordPageSelectedText(state); !ok || selected != "link text" {
		t.Fatalf("drag selection on hyperlink text = %q, %v; want %q", selected, ok, "link text")
	}
	select {
	case value := <-opened:
		t.Fatalf("plain click opened hyperlink %q; want link text selection", value)
	default:
	}

	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 202, Buttons: pointer.ButtonPrimary, Modifiers: key.ModShortcut, Position: start})
	frame()
	select {
	case value := <-opened:
		if value != target {
			t.Fatalf("opened hyperlink = %q; want %q", value, target)
		}
	case <-time.After(time.Second):
		t.Fatal("Command/Ctrl+click did not open the hyperlink")
	}
	if state.wordPageSelectionDrag != nil {
		t.Fatal("Command/Ctrl+click started a text selection drag")
	}
}

func TestWordInternalHyperlinkScrollsToBookmark(t *testing.T) {
	state := newDocumentSessionState("bookmark.docx")
	state.wordViewMode = wordViewWebLayout
	state.wordBlocks = []document.WordBlock{
		{ID: "before", Kind: document.WordParagraphBlock, Paragraph: &document.WordParagraph{ID: "before", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "before.run", Text: "before", Editable: true}}}}},
		{ID: "target", Kind: document.WordParagraphBlock, Paragraph: &document.WordParagraph{ID: "target", BookmarkNames: []string{"SectionOne"}, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "target.run", Text: "target", Editable: true}}}}},
	}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(400, 300)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	app.activateWordHyperlink(gtx, state, "#SectionOne")
	if state.wordListState.Position.First != 1 {
		t.Fatalf("Word list scroll position = %d; want bookmark block 1", state.wordListState.Position.First)
	}
	if state.wordPagePendingScrollRunID != "" {
		t.Fatalf("web layout queued a page scroll for %q", state.wordPagePendingScrollRunID)
	}
}

func TestTextEditorParagraphSelectionObservesCommandClick(t *testing.T) {
	router := new(input.Router)
	selection := new(textEditorParagraphSelection)
	editor := new(widget.Editor)
	editor.SetText("link")
	var ops op.Ops
	var activated int
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Constraints: layout.Constraints{Max: image.Pt(120, 30)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
		selection.layoutWithDoubleClickAtTextSizeAndPointer(gtx, editor, func(layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(100, 24)}
		}, nil, unit.Sp(12), new(wordHyperlinkClickTag), func(_ layout.Context, event pointer.Event) {
			if event.Modifiers.Contain(key.ModShortcut) && event.Buttons&pointer.ButtonPrimary != 0 {
				activated++
			}
		})
		router.Frame(&ops)
	}
	frame()
	position := f32.Pt(20, 12)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 203, Buttons: pointer.ButtonPrimary, Modifiers: key.ModShortcut, Position: position})
	frame()
	if activated != 1 {
		t.Fatalf("observed Command/Ctrl pointer presses = %d; want 1", activated)
	}
}

func TestWordWebLayoutHyperlinkShortcutOpens(t *testing.T) {
	app, state, router, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>web link</w:t></w:r></w:p>`)
	state.wordViewMode = wordViewWebLayout
	run := state.wordBlocks[0].Paragraph.Inlines[0].Text
	run.HyperlinkTarget = "https://example.test/web"
	opened := make(chan string, 1)
	app.wordHyperlinkLauncher = func(value string) error {
		opened <- value
		return nil
	}
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Constraints: layout.Constraints{Max: image.Pt(600, 500)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
		app.layoutWordParagraph(gtx, *state.wordBlocks[0].Paragraph, false)
		router.Frame(&ops)
	}
	frame()
	position := f32.Pt(20, 15)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 204, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 204, Position: position})
	frame()
	if state.selectedWordRunID != run.ID {
		t.Fatalf("plain web-layout click selected run %q; want %q", state.selectedWordRunID, run.ID)
	}
	select {
	case value := <-opened:
		t.Fatalf("plain web-layout click opened hyperlink %q", value)
	default:
	}
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 205, Buttons: pointer.ButtonPrimary, Modifiers: key.ModShortcut, Position: position})
	frame()
	select {
	case value := <-opened:
		if value != run.HyperlinkTarget {
			t.Fatalf("opened hyperlink = %q; want %q", value, run.HyperlinkTarget)
		}
	case <-time.After(time.Second):
		t.Fatal("web layout Command/Ctrl+click did not open the hyperlink")
	}
}
