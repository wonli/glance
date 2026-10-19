package desktopui

import (
	"errors"
	"glance/internal/application"
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

// layoutPresentationDocument is the top-level presentation editor layout matching Pen frame C2TtCZ.
func (a *App) layoutPresentationDocument(gtx layout.Context) layout.Dimensions {
	v := a.ensurePresentationView()

	// If in fullscreen presentation slideshow mode
	if v.slideshowMode {
		return a.layoutPresentationSlideshow(gtx, v)
	}

	a.syncPresentationNotesDraft(v)
	if !v.notesDraftConflict {
		if err := a.flushPresentationNotesDraft(v); err != nil && !errors.Is(err, application.ErrPresentationNotesConflict) {
			a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// 1. Category Bar (34dp)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutPresentationCategoryBar(gtx, v)
		}),
		// 2. Ribbon Toolbar (68dp) referencing Excel's ribbon icon implementation
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutPresentationRibbon(gtx, v)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if v.stylePopover != "" && v.stylePopoverShapeID != v.selectedShape {
				v.stylePopover = ""
				v.stylePopoverShapeID = 0
			}
			return a.layoutPresentationStylePopover(gtx, v)
		}),
		// 3. Main Workspace (Sidebar + Canvas + Notes)
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutAgentSidebarWorkspace(gtx, func(gtx layout.Context) layout.Dimensions {
				if v.sorterMode {
					return a.layoutPresentationSorter(gtx, v)
				}
				return a.layoutPresentationMainWorkspace(gtx, v)
			})
		}),
		// 4. Status Bar (34dp)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutPresentationStatusBar(gtx, v)
		}),
	)
}

// layoutPresentationCategoryBar renders the 34dp top tabs matching Pen frame C2TtCZ.
func (a *App) layoutPresentationCategoryBar(gtx layout.Context, v *presentationViewState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(pptxCategoryHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height

	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
	paint.FillShape(gtx.Ops, pptxBorderColor, clip.Rect{Min: image.Pt(0, height-1), Max: gtx.Constraints.Max}.Op())

	tabs := a.localizedPresentationRibbonTabs(v)
	if v.activeRibbonTab == "picture" {
		if _, _, ok := presentationSelectedEditablePicture(v); !ok {
			v.activeRibbonTab = "home"
		}
	}
	if v.activeRibbonTab == "shape" {
		if _, elem, ok := presentationSelectedStyleElement(v); !ok || elem.Kind == document.ElementKindPicture {
			v.activeRibbonTab = "home"
		}
	}

	return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var children []layout.FlexChild
		for _, tab := range tabs {
			t := tab
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btn := v.ribbonBtn("tab_" + t.id)
				if btn.Clicked(gtx) {
					if v.pictureCrop.active && t.id != "picture" {
						a.cancelPresentationPictureCrop(v)
					}
					v.activeRibbonTab = t.id
				}
				selected := v.activeRibbonTab == t.id

				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						textColor := pptxTextSecondary
						if selected {
							textColor = pptxAccentColor
						}
						lbl := material.Label(a.theme, unit.Sp(13), t.label)
						lbl.Color = textColor
						if selected {
							lbl.Font.Weight = font.SemiBold
						}
						dims := lbl.Layout(gtx)

						if selected {
							barH := gtx.Dp(unit.Dp(2))
							paint.FillShape(gtx.Ops, pptxAccentColor, clip.Rect{
								Min: image.Pt(0, dims.Size.Y+gtx.Dp(unit.Dp(4))),
								Max: image.Pt(dims.Size.X, dims.Size.Y+gtx.Dp(unit.Dp(4))+barH),
							}.Op())
						}
						return dims
					})
				})
			}))
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(unit.Dp(4))}.Layout(gtx, children...)
	})
}

// layoutPresentationRibbon renders the 68dp command bar referencing Excel's ribbon icon implementation.
func (a *App) layoutPresentationRibbon(gtx layout.Context, v *presentationViewState) layout.Dimensions {
	a.handlePresentationClipboardEvents(gtx, v)
	height := gtx.Dp(unit.Dp(pptxRibbonHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height

	actions := presentationActionsForView(v)
	iconSize := toolbarIconSize

	paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
	paint.FillShape(gtx.Ops, pptxBorderColor, clip.Rect{Min: image.Pt(0, height-1), Max: gtx.Constraints.Max}.Op())

	return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(a.layoutAgentRibbonEntry),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return v.ribbonActionList.Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
					action := actions[index]
					localizedAction := a.localizedPresentationAction(action)
					button := v.ribbonBtn(action.id)
					enabled := a.presentationActionEnabled(v, action.id)
					clicked := button.Clicked(gtx)
					if enabled && clicked {
						switch action.id {
						case "home_copy":
							a.startPresentationTextCopy(v)
						case "home_paste":
							a.startPresentationClipboardPaste(gtx, v)
						default:
							a.handlePresentationRibbonAction(v, action.id)
						}
					}

					active := a.isPresentationActionActive(v, action.id)
					iconColor, background := toolbarIconColor(enabled), contentSurface
					if active {
						background = toolbarActiveBackground
					}
					icon, _ := pptxToolbarIcon(action.icon, iconColor)

					return layoutToolbarActionItem(gtx, action, index, actions, func(gtx layout.Context) layout.Dimensions {
						btnGtx := gtx
						btnGtx.Constraints.Min = image.Pt(gtx.Dp(excelToolbarButtonWidth), gtx.Dp(excelToolbarButtonHeight))
						btnGtx.Constraints.Max = btnGtx.Constraints.Min
						return toolbarIconButton(btnGtx, a.theme, button, icon, iconSize, localizedAction.label, localizedAction.label, enabled, false, background)
					})
				})
			}),
		)
	})
}

func (a *App) presentationActionEnabled(v *presentationViewState, actionID string) bool {
	state := a.documentSessionState
	busy := state == nil || state.formatRunning.Load() || a.isSaving()
	switch actionID {
	case "design_theme", "design_colors":
		return !busy && v != nil && !v.commandPending && a.pptxDoc != nil && state.pptxDoc == a.pptxDoc && a.applicationSessionRegistered(state.documentID)
	case "home_group":
		return !busy && v != nil && !v.commandPending && len(presentationSelectedIDs(v)) >= 2 && a.pptxDoc != nil && state.pptxDoc == a.pptxDoc && a.applicationSessionRegistered(state.documentID)
	case "home_copy", "home_paste":
		if busy || v == nil || v.commandPending || v.clipboardReadPending || a.pptxDoc == nil || state.pptxDoc != a.pptxDoc || !a.applicationSessionRegistered(state.documentID) {
			return false
		}
		_, _, ok := presentationTextElement(v)
		return ok
	case "picture_replace":
		if _, _, ok := presentationSelectedEditablePicture(v); !ok || busy || a.pptxPicPickerRunning || (v != nil && v.pictureCrop.active) {
			return false
		}
		return a.pptxDoc != nil && state.pptxDoc == a.pptxDoc
	case "picture_crop":
		if _, _, ok := presentationSelectedEditablePicture(v); !ok || busy || a.pptxPicPickerRunning || (v != nil && v.pictureCrop.active) {
			return false
		}
		return a.pptxDoc != nil && state.pptxDoc == a.pptxDoc
	case "picture_crop_done", "picture_crop_reset", "picture_crop_cancel":
		return v != nil && v.pictureCrop.active && !v.pictureCrop.committing && !busy && a.pptxDoc != nil && state.pptxDoc == a.pptxDoc
	case "insert_table", "insert_chart", "insert_video", "insert_audio":
		return !busy && v != nil && !v.commandPending && !a.pptxMediaPickerRunning && a.pptxDoc != nil && state.pptxDoc == a.pptxDoc && a.applicationSessionRegistered(state.documentID)
	case "home_font", "home_font_size", "home_bold", "home_italic", "home_underline", "home_font_color", "home_bullets", "home_align_left", "home_align_center", "home_align_right", "home_align_justify", "home_line_spacing", "home_space_before", "home_space_after":
		if busy || v == nil || v.pictureCrop.active || a.pptxDoc == nil || state.pptxDoc != a.pptxDoc {
			return false
		}
		_, elem, ok := presentationSelectedStyleElement(v)
		return ok && elem.CanEditTextStyle
	case "home_fill", "shape_fill", "shape_no_fill", "shape_outline", "shape_line_width":
		if busy || v == nil || v.pictureCrop.active || a.pptxDoc == nil || state.pptxDoc != a.pptxDoc {
			return false
		}
		_, elem, ok := presentationSelectedStyleElement(v)
		return ok && elem.CanEditShapeStyle
	case "picture_outline", "picture_line_width":
		if busy || v == nil || v.pictureCrop.active || a.pptxDoc == nil || state.pptxDoc != a.pptxDoc {
			return false
		}
		_, _, ok := presentationSelectedEditablePicture(v)
		return ok
	case "review_access", "review_spell":
		if busy || v == nil || v.commandPending || a.pptxDoc == nil || state.pptxDoc != a.pptxDoc {
			return false
		}
		return a.applicationSessionRegistered(state.documentID)
	case "trans_fade", "trans_push", "trans_options", "trans_duration", "trans_apply_all":
		return !busy && v != nil && !v.commandPending && a.pptxDoc != nil && state.pptxDoc == a.pptxDoc && a.applicationSessionRegistered(state.documentID)
	case "design_size":
		return !busy && v != nil && !v.commandPending && !v.pictureCrop.active && a.pptxDoc != nil && state.pptxDoc == a.pptxDoc && a.applicationSessionRegistered(state.documentID)
	case "home_align":
		return !busy && v != nil && v.selectedShape != 0 && !v.commandPending && !v.pictureCrop.active && a.pptxDoc != nil && state.pptxDoc == a.pptxDoc && a.applicationSessionRegistered(state.documentID)
	default:
		return true
	}
}

func (a *App) isPresentationActionActive(v *presentationViewState, actionID string) bool {
	switch actionID {
	case "home_notes", "view_notes":
		return v.notesExpanded
	case "view_fit":
		return v.fitMode
	case "picture_crop":
		return v.pictureCrop.active
	case "design_size":
		return false
	case "home_bold", "home_italic", "home_underline":
		return presentationSelectedRunStyle(v, actionID)
	case "home_bullets":
		_, elem, ok := presentationSelectedStyleElement(v)
		return ok && len(elem.Paragraphs) == 1 && elem.Paragraphs[0].Bullet
	case "home_align_left", "home_align_center", "home_align_right", "home_align_justify":
		_, elem, ok := presentationSelectedStyleElement(v)
		align := map[string]string{"home_align_left": "left", "home_align_center": "center", "home_align_right": "right", "home_align_justify": "justify"}[actionID]
		return ok && presentationStyleOptionMatches(elem, document.PresentationElementStylePatch{ParagraphAlignment: &align})
	case "view_normal":
		return !v.slideshowMode && !v.sorterMode
	case "view_sorter":
		return v.sorterMode
	case "view_ruler":
		return v.rulerVisible
	default:
		return false
	}
}

func (a *App) handlePresentationRibbonAction(v *presentationViewState, actionID string) {
	if a.runPresentationTransitionAction(v, actionID) {
		return
	}
	if a.runPresentationViewRibbonAction(v, actionID) {
		return
	}
	if a.handlePresentationStyleAction(v, actionID) {
		return
	}
	switch actionID {
	case "file_new":
		a.handlePresentationAddSlide(v)
	case "file_open":
		a.startOpenFilePicker()
	case "file_save":
		a.startSave()
	case "file_saveas":
		a.startSaveAsPicker("pptx")
	case "file_export":
		a.requestPresentationPDFExport()
	case "file_print":
		a.requestPresentationPDFPrint()
	case "home_new_slide":
		a.handlePresentationAddSlide(v)
	case "home_layout":
		if !a.startPresentationLayoutList(v) {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_read_the_presentation_layout_cat_856ec3"}, feedbackWarning, notificationToast)
		}
	case "home_textbox", "insert_textbox":
		a.handlePresentationAddTextBox(v)
	case "home_notes", "view_notes":
		v.notesExpanded = !v.notesExpanded
	case "home_slideshow", "show_from_start":
		v.activeSlide = 0
		v.slideshowMode = true
	case "show_from_current":
		v.slideshowMode = true
	case "show_rehearse":
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.rehearsal_timer_ready_75bb0a"}, feedbackInfo, notificationSuppressed)
	case "view_fit":
		v.fitMode = true
		v.zoom = 1.0
	case "view_sorter":
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.slide_sorter_view_633a4a"}, feedbackInfo, notificationSuppressed)
	case "design_theme":
		a.showPresentationThemeOptions(v)
	case "design_colors":
		a.showPresentationPaletteOptions(v)
	case "design_bg":
		a.showPresentationBackgroundOptions(v)
	case "design_size":
		a.showPresentationSlideSizeOptions(v)
	case "home_bullets":
		a.setStatusMessage(i18n.Message{ID: "word.ribbon.action.bullet_list"}, feedbackInfo, notificationSuppressed)
	case "home_shapes", "insert_shapes":
		a.handlePresentationAddShape(v)
	case "home_align":
		a.showPresentationAlignmentOptions(v)
	case "home_group":
		a.startPresentationGroup(v)
	case "insert_table":
		a.showPresentationTableInsertOptions(v)
	case "insert_picture":
		a.startPresentationPictureInsertPicker()
	case "picture_replace":
		a.startPresentationPicturePicker()
	case "picture_crop":
		if !a.beginPresentationPictureCrop(v) {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_crop_the_current_picture_3ec3f0"}, feedbackWarning, notificationToast)
		}
	case "picture_crop_done":
		a.commitPresentationPictureCrop(v)
	case "picture_crop_cancel":
		a.cancelPresentationPictureCrop(v)
	case "picture_crop_reset":
		a.resetPresentationPictureCrop(v)
	case "insert_chart":
		a.showPresentationChartInsertOptions(v)
	case "insert_video":
		if !a.startPresentationMediaInsertPicker(v, "video") {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_start_video_insertion_ef329a"}, feedbackWarning, notificationToast)
		}
	case "insert_audio":
		if !a.startPresentationMediaInsertPicker(v, "audio") {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_start_audio_insertion_a0638d"}, feedbackWarning, notificationToast)
		}
	case "review_spell":
		if !a.startPresentationProofread(v) {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_start_presentation_text_review_a6efd3"}, feedbackWarning, notificationToast)
		}
	case "review_new_comment":
		a.showPresentationNewCommentDialog(v)
	case "review_del_comment":
		if !a.startPresentationDeleteComment(v) {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_start_reading_comments_c79d84"}, feedbackWarning, notificationToast)
		}
	case "review_access":
		if !a.startPresentationAccessibilityReview(v) {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_start_the_presentation_accessibi_40747a"}, feedbackWarning, notificationToast)
		}
	case "anim_pane":
		if !a.startPresentationAnimationPane(v) {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_open_the_animation_pane_4894cb"}, feedbackWarning, notificationToast)
		}
	case "anim_appear":
		if !a.startPresentationElementAnimation(v, "appear", 1) {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_start_the_appear_animation_38e8d4"}, feedbackWarning, notificationToast)
		}
	case "anim_fadein":
		if !a.startPresentationElementAnimation(v, "fade", 500) {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_start_the_fade_in_animation_29fec0"}, feedbackWarning, notificationToast)
		}
	case "anim_flyin":
		if !a.startPresentationElementAnimation(v, "fly_in", 500) {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_start_the_fly_in_animation_e9d688"}, feedbackWarning, notificationToast)
		}
	case "anim_add":
		a.showPresentationAnimationAddOptions(v)
	case "anim_duration":
		a.showPresentationAnimationDurationOptions(v)
	default:
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.ready_ddc094"}, feedbackSuccess, notificationSuppressed)
	}
}
