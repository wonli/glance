package desktopui

import (
	"fmt"
	"glance/internal/document"
	"glance/internal/i18n"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// layoutPresentationMainWorkspace renders the 248dp left sidebar + center canvas + collapsible bottom notes.
func (a *App) layoutPresentationMainWorkspace(gtx layout.Context, v *presentationViewState) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		// 1. Left Thumbnail Navigation Sidebar (248dp)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutPresentationSidebar(gtx, v)
		}),
		// 2. Right Canvas Area + Bottom Speaker Notes
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Canvas
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return a.layoutPresentationCanvas(gtx, v)
				}),
				// Notes panel (collapsible, 76dp when expanded)
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !v.notesExpanded {
						return layout.Dimensions{}
					}
					return a.layoutPresentationNotesPanel(gtx, v)
				}),
			)
		}),
	)
}

// layoutPresentationSidebar renders the 248dp slide list matching Pen frame C2TtCZ.
func (a *App) layoutPresentationSidebar(gtx layout.Context, v *presentationViewState) layout.Dimensions {
	width := gtx.Dp(unit.Dp(pptxSidebarWidthDP))
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width

	paint.FillShape(gtx.Ops, pptxSidebarBg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	paint.FillShape(gtx.Ops, pptxBorderColor, clip.Rect{Min: image.Pt(width-1, 0), Max: gtx.Constraints.Max}.Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Sidebar Header: a.textID("presentation.workspace.slides"), "+ 新建"
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			h := gtx.Dp(unit.Dp(48))
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = h, h
			return layout.Stack{Alignment: layout.W}.Layout(gtx,
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(5.5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(a.theme, unit.Sp(13), a.textID("presentation.workspace.slides"))
									lbl.Color = pptxAccentColor
									lbl.Font.Weight = font.SemiBold
									return lbl.Layout(gtx)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if v.addSlideButton.Clicked(gtx) {
									a.handlePresentationAddSlide(v)
								}
								return v.addSlideButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									btnBg := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
									if v.addSlideButton.Hovered() {
										btnBg = pptxAccentBg
									}
									gtx.Constraints.Min.Y = 0
									return layout.Background{}.Layout(gtx,
										func(gtx layout.Context) layout.Dimensions {
											shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(4)))
											paint.FillShape(gtx.Ops, btnBg, shape.Op(gtx.Ops))
											paint.FillShape(gtx.Ops, pptxBorderColor, clip.Stroke{
												Path:  shape.Path(gtx.Ops),
												Width: 1.0,
											}.Op())
											return layout.Dimensions{Size: gtx.Constraints.Min}
										},
										func(gtx layout.Context) layout.Dimensions {
											return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(10), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(unit.Dp(3))}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return homeIcon(gtx, "plus", unit.Dp(12), pptxAccentColor)
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return layout.Inset{Top: unit.Dp(4.5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															lbl := material.Label(a.theme, unit.Sp(12), a.textID("presentation.ribbon.action.file_new"))
															lbl.Color = pptxAccentColor
															return lbl.Layout(gtx)
														})
													}),
												)
											})
										},
									)
								})
							}),
						)
					})
				}),
			)
		}),
		// Slide list
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			slides := v.snapshot.Slides
			return material.List(a.theme, &v.slideList).Layout(gtx, len(slides), func(gtx layout.Context, index int) layout.Dimensions {
				return a.layoutSlideThumbnailItem(gtx, v, index, slides[index])
			})
		}),
	)
}

// layoutSlideThumbnailItem renders an individual slide card in the left navigation sidebar.
func (a *App) layoutSlideThumbnailItem(gtx layout.Context, v *presentationViewState, index int, slide document.PresentationSlide) layout.Dimensions {
	btn := v.slideClick(index)
	if btn.Clicked(gtx) {
		if err := a.flushPresentationNotesDraft(v); err == nil {
			if err := a.flushPresentationTextDraft(); err == nil {
				v.activeSlide = index
				clearPresentationSelection(v)
				if v.activeRibbonTab == "picture" {
					v.activeRibbonTab = "home"
				}
				v.editingShape = 0
				v.shapeDraftText = ""
				a.resetPresentationNotesEditor(v, &slide)
			}
		}
	}
	isActive := v.activeSlide == index

	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(unit.Dp(8))}.Layout(gtx,
				// Slide index number (e.g. 1, 2)
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					// Keep the column wide enough for every slide number in this deck.
					digits := len(fmt.Sprintf("%d", max(1, len(v.snapshot.Slides))))
					idxW := gtx.Dp(unit.Dp(16 + max(0, digits-1)*8))
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = idxW, idxW
					lbl := material.Label(a.theme, unit.Sp(12), fmt.Sprintf("%d", index+1))
					lbl.MaxLines = 1
					if isActive {
						lbl.Color = pptxAccentColor
						lbl.Font.Weight = font.Bold
					} else {
						lbl.Color = pptxTextMuted
					}
					return lbl.Layout(gtx)
				}),
				// 16:9 Thumbnail card
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					cardH := gtx.Dp(unit.Dp(108))
					gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = cardH, cardH

					// Border & background
					borderColor := pptxBorderColor
					borderWidth := gtx.Dp(unit.Dp(1))
					if isActive {
						borderColor = pptxAccentColor
						borderWidth = gtx.Dp(unit.Dp(2))
					}
					r := gtx.Dp(unit.Dp(6))
					shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, r)
					paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, shape.Op(gtx.Ops))

					// Draw thumbnail outline
					paint.FillShape(gtx.Ops, borderColor, clip.Stroke{
						Path:  shape.Path(gtx.Ops),
						Width: float32(borderWidth),
					}.Op())

					// Content inside thumbnail preview
					return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(10), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutPresentationThumbnail(gtx, slide)
						})
					})
				}),
			)
		})
	})
}

// layoutPresentationNotesPanel renders the 76dp bottom speaker notes area.
func (a *App) layoutPresentationNotesPanel(gtx layout.Context, v *presentationViewState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(pptxNotesHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height

	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
	paint.FillShape(gtx.Ops, pptxBorderColor, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())

	return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(a.theme, unit.Sp(11), a.textID("presentation.presenter.notes"))
						lbl.Color = pptxTextMuted
						lbl.Font.Weight = font.Medium
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !v.notesDraftConflict {
							return layout.Dimensions{}
						}
						if v.notesReloadButton.Clicked(gtx) {
							a.resetPresentationNotesEditor(v, v.currentSlide())
							a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_latest_speaker_notes_were_loaded_9d030c"}, feedbackSuccess, notificationToast)
						}
						return v.notesReloadButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(a.theme, unit.Sp(11), a.textID("presentation.workspace.load_latest"))
							lbl.Color = pptxAccentColor
							return lbl.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !v.notesDraftConflict {
							return layout.Dimensions{}
						}
						if v.notesKeepButton.Clicked(gtx) {
							if slide := v.currentSlide(); slide != nil {
								v.notesEditorBase = slide.Notes
								v.notesEditorSlideID = slide.ID
								v.notesDraftConflict = false
								if a.documentSessionState != nil && a.documentSessionState.pptxDoc == a.pptxDoc {
									a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationNotesDraft, v.notesEditor.Text() != v.notesEditorBase)
								}
								a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_local_speaker_notes_draft_will_be_kept_e66fd6"}, feedbackInfo, notificationToast)
							}
						}
						return v.notesKeepButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(a.theme, unit.Sp(11), a.textID("presentation.workspace.keep_changes"))
							lbl.Color = pptxAccentColor
							return lbl.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if v.notesToggleBtn.Clicked(gtx) {
							v.notesExpanded = false
						}
						return v.notesToggleBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(a.theme, unit.Sp(11), a.textID("presentation.workspace.collapse"))
							lbl.Color = pptxTextMuted
							return lbl.Layout(gtx)
						})
					}),
				)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				v.notesEditor.ReadOnly = a.saving
				ed := material.Editor(a.theme, &v.notesEditor, a.textID("presentation.workspace.notes_hint"))
				ed.TextSize = unit.Sp(12)
				ed.Color = pptxTextDark
				dims := ed.Layout(gtx)
				a.syncPresentationNotesDraft(v)
				return dims
			}),
		)
	})
}
