package desktopui

import (
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"glance/internal/i18n"
	"image"
)

func (a *App) handleMarkdownEvents(gtx layout.Context) {
	v := a.ensureMarkdownView()
	if !a.modal.Visible {
		for {
			if _, ok := v.editor.Update(gtx); !ok {
				break
			}
		}
		a.observeMarkdownDraft()
		a.flushMarkdownDraft()
		current := a.markdownDoc.Text()
		if current != v.sourceSynced && v.text() == v.sourceSynced && !markdownEditorComposing(&v.editor) {
			v.editor.SetText(current)
			v.sourceSynced = current
			v.observedDraft = current
			a.observeMarkdownDraft()
		}
		for {
			if _, ok := v.search.Update(gtx); !ok {
				break
			}
		}
	}
	a.pollMarkdownFileResults()
	a.pollMarkdownImagePicker()
	a.pollMarkdownEditResult(v)
	a.requestMarkdownParse()
	select {
	case result := <-v.results:
		v.accept(result)
	default:
	}
	if a.modal.Visible {
		return
	}
	if v.mode != 1 && v.pageClick.Clicked(gtx) {
		v.focus = true
	}
	for id, b := range v.buttons {
		if b.Clicked(gtx) && !a.isSaving() && !v.exportPending && !v.editPending {
			a.handleMarkdownAction(gtx, id)
		}
	}
	if v.focus && v.mode != 1 {
		gtx.Execute(key.FocusCmd{Tag: &v.editor})
		v.focus = false
	}
}
func (a *App) layoutMarkdownDocument(gtx layout.Context) layout.Dimensions {
	a.handleMarkdownEvents(gtx)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(a.layoutMarkdownRibbon), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return a.layoutAgentSidebarWorkspace(gtx, a.layoutMarkdownWorkspace)
	}), layout.Rigid(a.layoutMarkdownStatus))
}
func (a *App) layoutMarkdownWorkspace(gtx layout.Context) layout.Dimensions {
	v := a.ensureMarkdownView()
	width := gtx.Constraints.Max.X
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if v.collapsed || width < gtx.Dp(680) {
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(40), gtx.Dp(40)
				return jsonPanel(gtx, jsonSidebarColor, false, false, func(gtx layout.Context) layout.Dimensions {
					return layout.N.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.markdownTextButton(gtx, "outline", "☰", 36, 36, false)
					})
				})
			}
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(248), gtx.Dp(248)
			return a.layoutMarkdownOutline(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if v.mode == 1 {
				return a.layoutMarkdownPreview(gtx)
			}
			if v.mode == 2 {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, layout.Flexed(1, a.layoutMarkdownEditor), layout.Flexed(1, a.layoutMarkdownPreview))
			}
			return a.layoutMarkdownEditor(gtx)
		}),
	)
}
func markdownColumn(gtx layout.Context, child layout.Widget) layout.Dimensions {
	return layout.Inset{Top: 48, Bottom: 32, Left: 24, Right: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := min(gtx.Constraints.Max.X, gtx.Dp(768))
		return layout.N.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return child(gtx)
		})
	})
}
func (a *App) layoutMarkdownEditor(gtx layout.Context) layout.Dimensions {
	v := a.ensureMarkdownView()
	return v.pageClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		pointer.CursorText.Add(gtx.Ops)
		return jsonPanel(gtx, contentSurface, false, false, func(gtx layout.Context) layout.Dimensions {
			return markdownColumn(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
				style := material.Editor(a.theme, &v.editor, a.textID("markdown.editor.placeholder"))
				style.TextSize = 16
				style.LineHeightScale = a.tokens().Typography.BodyLineHeightScale
				style.Color = homeText
				return style.Layout(gtx)
			})
		})
	})
}
func (a *App) markdownTextButton(gtx layout.Context, id, label string, width, height unit.Dp, selected bool) layout.Dimensions {
	v := a.ensureMarkdownView()
	if width > 0 {
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(width), gtx.Dp(width)
	}
	gtx = jsonFixedHeight(gtx, height)
	return v.button(id).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return jsonLabel(gtx, a, label, 12, jsonBlue, selected, false)
		})
	})
}
func (a *App) layoutMarkdownStatus(gtx layout.Context) layout.Dimensions {
	v := a.ensureMarkdownView()
	gtx = jsonFixedHeight(gtx, 34)
	line, column := v.sourceCaret()
	return jsonPanel(gtx, chromeSurface, false, false, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				locale := a.localeSnapshot().Resolved
				text := a.textArgs("markdown.status.document_stats", i18n.Args{"Characters": i18n.FormatNumber(locale, float64(v.snapshot.Characters)), "Minutes": i18n.FormatNumber(locale, float64(v.snapshot.ReadingMinutes))})
				if v.exportMessage.ID != "" {
					text = a.text(v.exportMessage)
				} else if v.err != nil {
					text = a.text(i18n.Message{ID: "markdown.error.parse_failed"})
				} else if len(v.errors) > 0 {
					text += "    " + a.textArgs("markdown.status.preview_warnings", i18n.Args{"Count": i18n.FormatNumber(locale, float64(len(v.errors)))})
				}
				return jsonLabel(gtx, a, text, 11, mutedText, false, false)
			}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if v.lastPDFPath == "" {
					return layout.Dimensions{}
				}
				return a.markdownTextButton(gtx, "reveal-pdf", a.textID("markdown.action.open_output_folder"), 80, 34, false)
			}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = 0
				text := a.textArgs("markdown.status.cursor", i18n.Args{"Line": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(line)), "Column": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(column))})
				if gtx.Constraints.Max.X < gtx.Dp(260) {
					text = "Markdown"
				}
				return jsonLabel(gtx, a, text, 11, mutedText, false, false)
			}))
		})
	})
}
func markdownLayoutSize(gtx layout.Context) image.Point { return gtx.Constraints.Max }
