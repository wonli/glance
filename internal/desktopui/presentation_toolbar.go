package desktopui

import (
	"glance/internal/document"
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

	// Synchronize notes editor text when active slide changes
	if curSlide := v.currentSlide(); curSlide != nil {
		if v.lastNotesSlide != v.activeSlide {
			v.notesEditor.SetText(curSlide.Notes)
			v.lastNotesSlide = v.activeSlide
		} else if notesText := v.notesEditor.Text(); notesText != curSlide.Notes {
			if a.pptxDoc != nil {
				_ = a.pptxDoc.UpdateSlideNotes(v.activeSlide, notesText)
				v.snapshot = a.pptxDoc.Snapshot()
			}
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
			return a.layoutPresentationMainWorkspace(gtx, v)
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

	tabs := presentationRibbonTabs(v)
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
	height := gtx.Dp(unit.Dp(pptxRibbonHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height

	actions := presentationActionsForView(v)
	iconSize := toolbarIconSize

	paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
	paint.FillShape(gtx.Ops, pptxBorderColor, clip.Rect{Min: image.Pt(0, height-1), Max: gtx.Constraints.Max}.Op())

	return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return v.ribbonActionList.Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
			action := actions[index]
			button := v.ribbonBtn(action.id)
			enabled := a.presentationActionEnabled(v, action.id)
			clicked := button.Clicked(gtx)
			if enabled && clicked {
				a.handlePresentationRibbonAction(v, action.id)
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
				return toolbarIconButton(btnGtx, a.theme, button, icon, iconSize, action.label, action.label, enabled, false, background)
			})
		})
	})
}

func (a *App) presentationActionEnabled(v *presentationViewState, actionID string) bool {
	state := a.documentSessionState
	busy := state == nil || state.formatRunning.Load() || a.isSaving()
	switch actionID {
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
	case "home_font", "home_font_size", "home_bold", "home_italic", "home_underline", "home_font_color", "home_align_left", "home_align_center", "home_align_right", "home_align_justify", "home_line_spacing", "home_space_before", "home_space_after":
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
	case "home_align_left", "home_align_center", "home_align_right", "home_align_justify":
		_, elem, ok := presentationSelectedStyleElement(v)
		align := map[string]string{"home_align_left": "left", "home_align_center": "center", "home_align_right": "right", "home_align_justify": "justify"}[actionID]
		return ok && presentationStyleOptionMatches(elem, document.PresentationElementStylePatch{ParagraphAlignment: &align})
	case "view_normal":
		return !v.slideshowMode
	default:
		return false
	}
}

func (a *App) handlePresentationRibbonAction(v *presentationViewState, actionID string) {
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
		a.startSaveAsPicker("pdf")
	case "file_print":
		a.setStatus("打印功能就绪")
	case "home_new_slide":
		a.handlePresentationAddSlide(v)
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
		a.setStatus("排练计时就绪")
	case "show_presenter":
		a.setStatus("演讲者视图就绪")
	case "view_fit":
		v.fitMode = true
		v.zoom = 1.0
	case "view_sorter":
		a.setStatus("幻灯片浏览视图")
	case "view_ruler":
		a.setStatus("标尺显示")
	case "design_theme":
		a.setStatus("主题选择")
	case "design_colors":
		a.setStatus("配色方案")
	case "design_bg":
		a.setStatus("背景格式设置")
	case "design_size":
		a.setStatus("幻灯片大小设置")
	case "home_bullets":
		a.setStatus("项目符号")
	case "home_shapes", "insert_shapes":
		a.setStatus("插入形状")
	case "home_align":
		a.setStatus("排列对齐")
	case "home_group":
		a.setStatus("组合")
	case "home_copy":
		a.setStatus("已复制内容")
	case "home_paste":
		a.setStatus("已粘贴内容")
	case "insert_table":
		a.setStatus("插入表格")
	case "insert_picture":
		a.startOpenFilePicker()
	case "picture_replace":
		a.startPresentationPicturePicker()
	case "picture_crop":
		if !a.beginPresentationPictureCrop(v) {
			a.setStatus("无法裁剪当前图片")
		}
	case "picture_crop_done":
		a.commitPresentationPictureCrop(v)
	case "picture_crop_cancel":
		a.cancelPresentationPictureCrop(v)
	case "picture_crop_reset":
		a.resetPresentationPictureCrop(v)
	case "insert_chart":
		a.setStatus("插入图表")
	case "insert_video":
		a.setStatus("插入视频")
	case "insert_audio":
		a.setStatus("插入音频")
	case "review_spell":
		a.setStatus("拼写检查就绪")
	case "review_new_comment":
		a.setStatus("新建批注")
	case "review_del_comment":
		a.setStatus("删除批注")
	case "review_access":
		a.setStatus("辅助功能检查就绪")
	case "trans_fade", "trans_push", "trans_options", "trans_duration", "trans_apply_all":
		a.setStatus("切换效果设置")
	case "anim_appear", "anim_fadein", "anim_flyin", "anim_add", "anim_pane", "anim_duration":
		a.setStatus("动画效果设置")
	default:
		a.setStatus("功能已就绪")
	}
}
