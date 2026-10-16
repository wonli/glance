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
)

func TestWordSearchDismissalReturnsFocusToCrossRunMatch(t *testing.T) {
	for _, dismiss := range []struct {
		name string
		act  func(*input.Router, func())
	}{
		{name: "Escape", act: func(router *input.Router, frame func()) {
			router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
			frame()
		}},
		{name: "close button", act: func(router *input.Router, frame func()) {
			position := f32.Pt(650, 104)
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position, Time: time.Second})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position, Time: time.Second + time.Millisecond})
			frame()
			frame()
		}},
	} {
		t.Run(dismiss.name, func(t *testing.T) {
			app, state, router, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>need</w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>le</w:t></w:r></w:p>`)
			state.wordViewMode = wordViewWebLayout
			paragraph := state.wordBlocks[0].Paragraph
			endRunID := paragraph.Inlines[1].Text.ID
			app.showWordFindDialog()
			app.modal.Input.SetText("needle")
			app.findNextWordText("needle")

			var ops op.Ops
			frame := func() {
				ops.Reset()
				gtx := layout.Context{
					Source:      router.Source(),
					Ops:         &ops,
					Now:         time.Now(),
					Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
					Constraints: layout.Constraints{Max: image.Pt(820, 500)},
				}
				app.handleOverlayEscape(gtx)
				app.layoutWordParagraph(gtx, *paragraph, false)
				app.layoutModalDialog(gtx)
				router.Frame(&ops)
			}
			frame()
			if !router.Source().Focused(&app.modal.Input) {
				t.Fatal("Find query did not receive focus while the dialog was open")
			}

			dismiss.act(router, frame)
			if app.modal.Visible {
				t.Fatal("dismiss action left the Find dialog open")
			}
			editor := state.wordRunEditors[endRunID]
			if editor == nil || !router.Source().Focused(editor) {
				t.Fatal("closing Find did not return keyboard focus to the matched run")
			}
			if selected, ok := wordPageSelectedText(state); !ok || selected != "needle" {
				t.Fatalf("matched selection after closing Find = %q, %v; want needle", selected, ok)
			}
			if start, end := editor.Selection(); start != 0 || end != 2 {
				t.Fatalf("focused run selection = [%d,%d); want [0,2)", start, end)
			}
		})
	}
}
