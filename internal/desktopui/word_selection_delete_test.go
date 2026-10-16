package desktopui

import (
	"archive/zip"
	"bytes"
	"image"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"glance/internal/format/docx"
	"glance/internal/wordlayout"
)

func newWordSelectionPageTest(t *testing.T, body string) (*App, *documentSessionState, *input.Router, func()) {
	t.Helper()
	var data bytes.Buffer
	z := zip.NewWriter(&data)
	for name, content := range map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body + `<w:sectPr/></w:body></w:document>`,
	} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	word, err := docx.OpenSnapshot(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("selection.docx")
	state.word = word
	state.setWordBlocks(word.WordBlocks())
	page := wordlayout.PageScene{Index: 0, Clip: wordlayout.Rect{WidthPT: 200, HeightPT: 200}, Content: wordlayout.Rect{WidthPT: 200, HeightPT: 200}}
	locations := make(map[wordlayout.SourcePosition]wordlayout.Position)
	for pi, block := range state.wordBlocks {
		if block.Paragraph == nil {
			continue
		}
		x, y := 10., 30.+float64(pi)*30
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text == nil {
				continue
			}
			r := inline.Text
			n := utf8.RuneCountInString(r.Text)
			f := wordlayout.TextFragment{RunID: r.ID, Text: r.Text, RuneEnd: n, Editable: true, XPT: x, BaselineYPT: y, Style: r.Style}
			f.Style.FontSize = 12
			for i := 0; i < n; i++ {
				f.Glyphs = append(f.Glyphs, wordlayout.PositionedGlyph{RuneStart: i, RuneEnd: i + 1, XPT: x + float64(i)*10, YPT: y, AdvanceXPT: 10})
			}
			for i := 0; i <= n; i++ {
				locations[wordlayout.SourcePosition{RunID: r.ID, RuneOffset: i}] = wordlayout.Position{PageIndex: 0, XPT: x + float64(i)*10, YPT: y}
			}
			page.Text = append(page.Text, f)
			x += float64(n) * 10
		}
	}
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{page}, Locations: locations}
	app := &App{documentSessionState: state, theme: material.NewTheme(), wordCommandResults: make(chan wordCommandResult, 8), shutdown: make(chan struct{})}
	t.Cleanup(func() {
		close(app.shutdown)
		app.stopAndWaitManagedBackground()
		app.completeQueuedWordSplitsOnClose()
		word.Close()
	})
	router := new(input.Router)
	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(600, 500)}}
		app.layoutWordPages(gtx, state, wordPageContinuous)
		router.Frame(&ops)
	}
	frame()
	return app, state, router, frame
}

func TestWordPageDeleteRemovesEntireDraggedSelection(t *testing.T) {
	for _, name := range []key.Name{key.NameDeleteBackward, key.NameDeleteForward} {
		t.Run(string(name), func(t *testing.T) {
			app, state, router, frame := newWordSelectionPageTest(t, `<w:p><w:r><w:t>甲乙</w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>丙丁</w:t></w:r></w:p>`)
			transform := state.wordPageVisibleInteractions[0].Transform
			start, end := transform.PageToScreen(f32.Pt(20, 30)), transform.PageToScreen(f32.Pt(40, 30))
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: start, Time: time.Second})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: end, Time: time.Second + time.Millisecond*20})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: end, Time: time.Second + time.Millisecond*40})
			frame()
			if text, ok := wordPageSelectedText(state); !ok || text != "乙丙" {
				t.Fatalf("selected %q, %v; want 乙丙", text, ok)
			}
			router.Queue(key.Event{Name: name, State: key.Press})
			frame()
			// Process asynchronous document mutation, if one was queued.
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				app.processAsyncResults()
				if !state.formatRunning.Load() && len(app.wordCommandResults) == 0 {
					break
				}
				time.Sleep(time.Millisecond)
			}
			blocks := state.word.WordBlocks()
			got := ""
			for _, in := range blocks[0].Paragraph.Inlines {
				if in.Text != nil {
					got += in.Text.Text
				}
			}
			if got != "甲丁" {
				t.Fatalf("Delete selected text produced %q; want 甲丁", got)
			}
			frame()
			if wordPageTextSelectionActive(state.wordPageTextSelection) {
				t.Fatal("deleted selection remains highlighted")
			}
		})
	}
}

func TestWordPageDoubleClickSelectsEntireFormattedParagraph(t *testing.T) {
	_, state, router, frame := newWordSelectionPageTest(t, `<w:p><w:r><w:t>甲乙</w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>丙丁</w:t></w:r></w:p><w:p><w:r><w:t>别的段落</w:t></w:r></w:p>`)
	point := state.wordPageVisibleInteractions[0].Transform.PageToScreen(f32.Pt(20, 30))
	for i := 0; i < 2; i++ {
		when := time.Second + time.Duration(i)*100*time.Millisecond
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point, Time: when})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point, Time: when + 20*time.Millisecond})
		frame()
	}
	if text, ok := wordPageSelectedText(state); !ok || text != "甲乙丙丁" {
		t.Fatalf("double click selected %q, %v; want current paragraph", text, ok)
	}
}

func TestWordPageDeleteReverseAndCrossParagraphSelection(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		from, to   f32.Point
	}{
		{"reverse single run", `<w:p><w:r><w:t>甲乙丙丁</w:t></w:r></w:p>`, f32.Pt(40, 30), f32.Pt(20, 30)},
		{"across paragraphs", `<w:p><w:r><w:t>甲乙</w:t></w:r></w:p><w:p><w:r><w:t>丙丁</w:t></w:r></w:p>`, f32.Pt(20, 30), f32.Pt(20, 60)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, state, router, frame := newWordSelectionPageTest(t, tc.body)
			transform := state.wordPageVisibleInteractions[0].Transform
			start, end := transform.PageToScreen(tc.from), transform.PageToScreen(tc.to)
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: start, Time: time.Second})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: end, Time: time.Second + 20*time.Millisecond})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: end, Time: time.Second + 40*time.Millisecond})
			frame()
			router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
			frame()
			waitWordInteraction(t, app, func() bool { return state.wordSelectionDeleteIntent == nil && !state.formatRunning.Load() })
			blocks := state.word.WordBlocks()
			if len(blocks) != 1 {
				t.Fatalf("paragraph count=%d; want merged paragraph", len(blocks))
			}
			got := ""
			for _, in := range blocks[0].Paragraph.Inlines {
				if in.Text != nil {
					got += in.Text.Text
				}
			}
			if got != "甲丁" {
				t.Fatalf("deleted range left %q; want 甲丁", got)
			}
		})
	}
}
