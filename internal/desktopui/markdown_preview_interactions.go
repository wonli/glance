package desktopui

import (
	"fmt"
	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	"glance/internal/i18n"
	"image"
	"strings"
)

func (a *App) layoutMarkdownTask(gtx layout.Context, s document.MarkdownSpan) layout.Dimensions {
	v := a.ensureMarkdownView()
	id := fmt.Sprintf("task-%d", s.SourceStart)
	v.tasks[id] = s.SourceStart
	return v.button(id).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		pointer.CursorPointer.Add(gtx.Ops)
		semantic.LabelOp("切换待办").Add(gtx.Ops)
		semantic.CheckBox.Add(gtx.Ops)
		semantic.SelectedOp(s.Checked).Add(gtx.Ops)
		box := image.Rect(gtx.Dp(2), gtx.Dp(3), gtx.Dp(18), gtx.Dp(19))
		border := clip.UniformRRect(box, gtx.Dp(3))
		color := mutedText
		if s.Checked {
			color = jsonBlue
			paint.FillShape(gtx.Ops, jsonBlue, border.Op(gtx.Ops))
		}
		paint.FillShape(gtx.Ops, color, clip.Stroke{Path: border.Path(gtx.Ops), Width: float32(max(1, gtx.Dp(1)))}.Op())
		if s.Checked {
			var path clip.Path
			path.Begin(gtx.Ops)
			path.MoveTo(f32.Pt(float32(gtx.Dp(5)), float32(gtx.Dp(11))))
			path.LineTo(f32.Pt(float32(gtx.Dp(9)), float32(gtx.Dp(15))))
			path.LineTo(f32.Pt(float32(gtx.Dp(15)), float32(gtx.Dp(7))))
			paint.FillShape(gtx.Ops, contentSurface, clip.Stroke{Path: path.End(), Width: float32(gtx.Dp(2))}.Op())
		}
		return layout.Dimensions{Size: image.Pt(gtx.Dp(24), gtx.Dp(24))}
	})
}
func (a *App) toggleMarkdownTask(offset int) error {
	v := a.ensureMarkdownView()
	source := v.text()
	if markdownEditorComposing(&v.editor) {
		return fmt.Errorf("请先完成输入法选字")
	}
	if source != v.snapshot.Source {
		return fmt.Errorf("预览正在更新，请稍后切换待办")
	}
	out, err := markdowndoc.ToggleTask(source, offset)
	if err != nil {
		return err
	}
	start, end := v.editor.Selection()
	r := []rune(out)
	v.editor.SetCaret(offset, offset+1)
	v.editor.Insert(string(r[offset]))
	v.editor.SetCaret(start, end)
	v.snapshot.Source = out
	for i := range v.snapshot.Blocks {
		for j := range v.snapshot.Blocks[i].Spans {
			s := &v.snapshot.Blocks[i].Spans[j]
			if s.Kind == "task" && s.SourceStart == offset {
				s.Checked = r[offset] == 'x' || r[offset] == 'X'
				s.Text = "☐ "
				if s.Checked {
					s.Text = "☑ "
				}
			}
		}
	}
	a.observeMarkdownDraft()
	a.flushMarkdownDraft()
	if v.pending {
		if v.parseCancel != nil {
			v.parseCancel()
		}
		v.pending = false
		v.images = nil
		v.parseGeneration++
	}
	a.requestMarkdownParse()
	return nil
}
func (a *App) layoutMarkdownFormulaTarget(gtx layout.Context, s document.MarkdownSpan, draw layout.Widget) layout.Dimensions {
	if s.Kind != "math" || s.SourceEnd <= s.SourceStart {
		return draw(gtx)
	}
	v := a.ensureMarkdownView()
	id := fmt.Sprintf("math-%d-%d", s.SourceStart, s.SourceEnd)
	v.formulas[id] = s
	return v.button(id).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		pointer.CursorPointer.Add(gtx.Ops)
		semantic.LabelOp("编辑公式").Add(gtx.Ops)
		return draw(gtx)
	})
}
func (a *App) showMarkdownFormulaEdit(formula document.MarkdownSpan) {
	v := a.ensureMarkdownView()
	source := v.text()
	r := []rune(source)
	if markdownEditorComposing(&v.editor) {
		a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.finish_chinese_input_composition_first_b4284e"}, feedbackWarning, notificationSuppressed)
		return
	}
	if source != v.snapshot.Source || formula.SourceStart < 0 || formula.SourceEnd > len(r) || formula.SourceEnd <= formula.SourceStart {
		a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_preview_is_updating_try_editing_the_fo_2d56d6"}, feedbackInfo, notificationToast)
		return
	}
	original := string(r[formula.SourceStart:formula.SourceEnd])
	delim := "$"
	if formula.DisplayMath {
		delim = "$$"
	}
	if !strings.HasPrefix(original, delim) || !strings.HasSuffix(original, delim) {
		a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_formula_position_changed_2ed4c5"}, feedbackInfo, notificationSuppressed)
		return
	}
	inner := original[len(delim) : len(original)-len(delim)]
	if strings.TrimSpace(inner) != formula.Text {
		a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_formula_content_changed_997f14"}, feedbackInfo, notificationSuppressed)
		return
	}
	state := a.documentSessionState
	start, end := v.editor.Selection()
	a.modal.ShowFormMessage("markdown.edit.math", i18n.Message{ID: "markdown.edit.math.title"}, i18n.Message{}, []i18n.Message{{ID: "markdown.insert.math.label"}}, []i18n.Message{{ID: "markdown.insert.math.hint"}}, []string{formula.Text}, func(id string) {
		if id == "cancel" {
			a.modal.Hide()
			return
		}
		if id != "confirm" {
			return
		}
		if a.documentSessionState != state || v.text() != source {
			a.modal.Hide()
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_document_content_changed_the_edit_was_f32be1"}, feedbackInfo, notificationSuppressed)
			return
		}
		if markdownEditorComposing(&v.editor) {
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.finish_chinese_input_composition_first_b4284e"}, feedbackWarning, notificationSuppressed)
			return
		}
		expr := a.modal.FormInputs[0].Text()
		if _, err := markdowndoc.RenderMath(expr, formula.DisplayMath); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "markdown.error.operation_failed"}, err, notificationToast)
			return
		}
		leading := inner[:len(inner)-len(strings.TrimLeft(inner, " \t\r\n"))]
		trailing := inner[len(strings.TrimRight(inner, " \t\r\n")):]
		replacement := delim + leading + expr + trailing + delim
		v.editor.SetCaret(formula.SourceStart, formula.SourceEnd)
		v.editor.Insert(replacement)
		delta := len([]rune(replacement)) - (formula.SourceEnd - formula.SourceStart)
		rebase := func(pos int) int {
			if pos <= formula.SourceStart {
				return pos
			}
			if pos >= formula.SourceEnd {
				return pos + delta
			}
			return formula.SourceStart + len([]rune(replacement))
		}
		v.editor.SetCaret(rebase(start), rebase(end))
		a.observeMarkdownDraft()
		a.flushMarkdownDraft()
		a.modal.Hide()
		a.requestMarkdownParse()
	}, modalDialogActionSpec{ID: "cancel", Label: "取消"}, modalDialogActionSpec{ID: "confirm", Label: "确认"})
}

func (a *App) markdownFormulaAtSelection() (document.MarkdownSpan, bool) {
	v := a.ensureMarkdownView()
	if v.snapshot.Source != v.text() {
		return document.MarkdownSpan{}, false
	}
	start, end := v.editor.Selection()
	if start > end {
		start, end = end, start
	}
	for _, block := range v.snapshot.Blocks {
		spans := append([]document.MarkdownSpan(nil), block.Spans...)
		for _, row := range block.TableSpans {
			for _, cell := range row {
				spans = append(spans, cell...)
			}
		}
		if block.Kind == "math" {
			spans = []document.MarkdownSpan{{Kind: "math", Text: block.Text, DisplayMath: true, SourceStart: block.SourceStart, SourceEnd: block.SourceEnd}}
		}
		for _, span := range spans {
			if span.Kind == "math" && span.SourceStart <= start && start < span.SourceEnd && end <= span.SourceEnd {
				return span, true
			}
		}
	}
	return document.MarkdownSpan{}, false
}
