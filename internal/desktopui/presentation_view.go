package desktopui

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
)

const (
	pptxCategoryHeightDP  = 34
	pptxRibbonHeightDP    = 68
	pptxSidebarWidthDP    = 248
	pptxNotesHeightDP     = 76
	pptxStatusBarHeightDP = 34
)

var (
	pptxAccentColor   = color.NRGBA{R: 99, G: 91, B: 255, A: 255}   // #635BFF (Pen spec)
	pptxAccentBg      = color.NRGBA{R: 238, G: 236, B: 255, A: 255} // #EEECFF
	pptxCanvasBg      = color.NRGBA{R: 233, G: 236, B: 241, A: 255} // #E9ECF1 (Pen spec)
	pptxSidebarBg     = color.NRGBA{R: 247, G: 248, B: 250, A: 255} // #F7F8FA (Pen spec)
	pptxBorderColor   = color.NRGBA{R: 225, G: 228, B: 232, A: 255} // #E1E4E8
	pptxTextDark      = color.NRGBA{R: 23, G: 35, B: 60, A: 255}    // #17233C
	pptxTextMuted     = color.NRGBA{R: 123, G: 131, B: 141, A: 255} // #7B838D
	pptxTextSecondary = color.NRGBA{R: 89, G: 97, B: 107, A: 255}   // #59616B
	pptxStatusBg      = color.NRGBA{R: 247, G: 248, B: 250, A: 255} // #F7F8FA
	pptxStatusRule    = color.NRGBA{R: 212, G: 216, B: 222, A: 255} // #D4D8DE
)

// pptxToolbarIcon resolves and renders an icon strictly from the PPTX icon family (assets/icons/pptx).
func pptxToolbarIcon(name string, tint color.NRGBA) (image.Image, bool) {
	return pptxToolbarIconAtSize(name, tint, 96)
}

func pptxToolbarIconAtSize(name string, tint color.NRGBA, size int) (image.Image, bool) {
	return svgIconTintAtSize("pptx", name, size, tint)
}

// PPTX ribbon actions configured per tab, strictly matching Pen design frame iRLmn.
var pptxHomeActions = []ribbonAction{
	{id: "home_paste", label: "粘贴", group: "剪贴板", icon: "clipboard-paste"},
	{id: "home_copy", label: "复制", group: "剪贴板", icon: "copy"},
	{id: "home_new_slide", label: "新建幻灯片", group: "幻灯片", icon: "gallery-horizontal-end"},
	{id: "home_layout", label: "版式", group: "幻灯片", icon: "panels-top-left"},
	{id: "home_font", label: "字体", group: "字体", icon: "type"},
	{id: "home_bold", label: "加粗", group: "字体", icon: "bold"},
	{id: "home_font_color", label: "字体颜色", group: "字体", icon: "baseline"},
	{id: "home_bullets", label: "项目符号", group: "段落", icon: "list"},
	{id: "home_align_left", label: "左对齐", group: "段落", icon: "format_align_left"},
	{id: "home_align_center", label: "居中", group: "段落", icon: "format_align_center"},
	{id: "home_shapes", label: "形状", group: "绘图", icon: "shapes"},
	{id: "home_fill", label: "填充颜色", group: "绘图", icon: "paint-bucket"},
	{id: "home_align", label: "对齐", group: "排列", icon: "align-start-vertical"},
	{id: "home_group", label: "组合", group: "排列", icon: "group"},
}

var pptxFileActions = []ribbonAction{
	{id: "file_new", label: "新建", group: "新建与打开", icon: "file-plus"},
	{id: "file_open", label: "打开", group: "新建与打开", icon: "folder-open"},
	{id: "file_save", label: "保存", group: "保存", icon: "save"},
	{id: "file_saveas", label: "另存为", group: "保存", icon: "save-all"},
	{id: "file_export", label: "导出 PDF", group: "输出", icon: "file-down"},
	{id: "file_print", label: "打印", group: "输出", icon: "printer"},
}

var pptxInsertActions = []ribbonAction{
	{id: "insert_table", label: "表格", group: "页面", icon: "table"},
	{id: "insert_picture", label: "图片", group: "图像", icon: "image"},
	{id: "insert_shapes", label: "形状", group: "插图", icon: "shapes"},
	{id: "insert_chart", label: "图表", group: "插图", icon: "chart-column"},
	{id: "insert_textbox", label: "文本框", group: "文本", icon: "text-cursor-input"},
	{id: "insert_video", label: "视频", group: "媒体", icon: "video"},
	{id: "insert_audio", label: "音频", group: "媒体", icon: "volume-2"},
}

var pptxDesignActions = []ribbonAction{
	{id: "design_theme", label: "主题", group: "主题", icon: "palette"},
	{id: "design_colors", label: "配色", group: "主题", icon: "paint-bucket"},
	{id: "design_bg", label: "背景格式", group: "背景", icon: "settings-2"},
	{id: "design_size", label: "幻灯片大小", group: "页面设置", icon: "ratio"},
}

var pptxTransitionsActions = []ribbonAction{
	{id: "trans_fade", label: "淡化", group: "切换效果", icon: "blend"},
	{id: "trans_push", label: "推进", group: "切换效果", icon: "move-right"},
	{id: "trans_options", label: "效果选项", group: "效果设置", icon: "sliders-horizontal"},
	{id: "trans_duration", label: "持续时间", group: "效果设置", icon: "timer"},
	{id: "trans_apply_all", label: "全部应用", group: "换片方式", icon: "copy-check"},
}

var pptxAnimationsActions = []ribbonAction{
	{id: "anim_appear", label: "出现", group: "动画效果", icon: "circle-plus"},
	{id: "anim_fadein", label: "淡入", group: "动画效果", icon: "blend"},
	{id: "anim_flyin", label: "飞入", group: "动画效果", icon: "move-up"},
	{id: "anim_add", label: "添加动画", group: "动画管理", icon: "plus"},
	{id: "anim_pane", label: "动画窗格", group: "动画管理", icon: "panel-right"},
	{id: "anim_duration", label: "持续时间", group: "计时", icon: "timer"},
}

var pptxSlideShowActions = []ribbonAction{
	{id: "show_from_start", label: "从头开始", group: "开始放映", icon: "presentation"},
	{id: "show_from_current", label: "从当前页", group: "开始放映", icon: "play"},
	{id: "show_rehearse", label: "排练计时", group: "放映设置", icon: "timer"},
	{id: "show_presenter", label: "演讲者视图", group: "演示工具", icon: "panels-top-left"},
}

var pptxReviewActions = []ribbonAction{
	{id: "review_spell", label: "拼写检查", group: "校对", icon: "spell-check"},
	{id: "review_new_comment", label: "新建批注", group: "批注", icon: "message-square-plus"},
	{id: "review_del_comment", label: "删除批注", group: "批注", icon: "message-square-x"},
	{id: "review_access", label: "辅助功能", group: "检查", icon: "accessibility"},
}

var pptxViewActions = []ribbonAction{
	{id: "view_normal", label: "普通视图", group: "演示视图", icon: "panels-top-left"},
	{id: "view_sorter", label: "幻灯片浏览", group: "演示视图", icon: "layout-grid"},
	{id: "view_notes", label: "备注页", group: "演示视图", icon: "sticky-note"},
	{id: "view_ruler", label: "标尺", group: "显示", icon: "ruler"},
	{id: "view_fit", label: "适应窗口", group: "缩放", icon: "maximize"},
}

func pptxActionsForTab(tab string) []ribbonAction {
	switch tab {
	case "file":
		return pptxFileActions
	case "insert":
		return pptxInsertActions
	case "design":
		return pptxDesignActions
	case "transitions":
		return pptxTransitionsActions
	case "animations":
		return pptxAnimationsActions
	case "slideshow":
		return pptxSlideShowActions
	case "review":
		return pptxReviewActions
	case "view":
		return pptxViewActions
	default:
		return pptxHomeActions
	}
}

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

	tabs := []struct {
		id    string
		label string
	}{
		{"file", "文件"},
		{"home", "开始"},
		{"insert", "插入"},
		{"design", "设计"},
		{"transitions", "切换"},
		{"animations", "动画"},
		{"slideshow", "幻灯片放映"},
		{"review", "审阅"},
		{"view", "视图"},
	}

	return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var children []layout.FlexChild
		for _, tab := range tabs {
			t := tab
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btn := v.ribbonBtn("tab_" + t.id)
				if btn.Clicked(gtx) {
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
								Min: image.Pt(0, dims.Size.Y + gtx.Dp(unit.Dp(4))),
								Max: image.Pt(dims.Size.X, dims.Size.Y + gtx.Dp(unit.Dp(4)) + barH),
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

	actions := pptxActionsForTab(v.activeRibbonTab)
	iconSize := toolbarIconSize

	paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
	paint.FillShape(gtx.Ops, pptxBorderColor, clip.Rect{Min: image.Pt(0, height-1), Max: gtx.Constraints.Max}.Op())

	return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return v.ribbonActionList.Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
			action := actions[index]
			button := v.ribbonBtn(action.id)
			if button.Clicked(gtx) {
				a.handlePresentationRibbonAction(v, action.id)
			}

			active := a.isPresentationActionActive(v, action.id)
			enabled := true
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

func (a *App) isPresentationActionActive(v *presentationViewState, actionID string) bool {
	switch actionID {
	case "home_notes", "view_notes":
		return v.notesExpanded
	case "view_fit":
		return v.fitMode
	case "design_size":
		return false
	case "view_normal":
		return !v.slideshowMode
	default:
		return false
	}
}

func (a *App) handlePresentationRibbonAction(v *presentationViewState, actionID string) {
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
	case "home_bold":
		a.setStatus("字体样式：加粗 (B)")
	case "home_font":
		a.setStatus("字体选择")
	case "home_font_color":
		a.setStatus("字体颜色")
	case "home_bullets":
		a.setStatus("项目符号")
	case "home_align_left":
		a.setStatus("段落对齐：左对齐")
	case "home_align_center":
		a.setStatus("段落对齐：居中")
	case "home_shapes", "insert_shapes":
		a.setStatus("插入形状")
	case "home_fill":
		a.setStatus("填充颜色")
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
		// Sidebar Header: "幻灯片", "+ 新建"
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
									lbl := material.Label(a.theme, unit.Sp(13), "幻灯片")
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
															lbl := material.Label(a.theme, unit.Sp(12), "新建")
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
		v.activeSlide = index
		v.selectedShape = 0
		v.editingShape = 0
		v.notesEditor.SetText(slide.Notes)
		v.lastNotesSlide = index
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
						lbl := material.Label(a.theme, unit.Sp(11), "演讲者备注")
						lbl.Color = pptxTextMuted
						lbl.Font.Weight = font.Medium
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if v.notesToggleBtn.Clicked(gtx) {
							v.notesExpanded = false
						}
						return v.notesToggleBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(a.theme, unit.Sp(11), "收起 ▾")
							lbl.Color = pptxTextMuted
							return lbl.Layout(gtx)
						})
					}),
				)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				ed := material.Editor(a.theme, &v.notesEditor, "单击此处添加演讲者备注…")
				ed.TextSize = unit.Sp(12)
				ed.Color = pptxTextDark
				return ed.Layout(gtx)
			}),
		)
	})
}

// layoutPresentationStatusBar renders the 34dp status bar matching Pen node C2TtCZ.
func (a *App) layoutPresentationStatusBar(gtx layout.Context, v *presentationViewState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(pptxStatusBarHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height

	paint.FillShape(gtx.Ops, pptxStatusBg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	paint.FillShape(gtx.Ops, pptxStatusRule, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())

	// Handle status bar actions
	if v.zoomInBtn.Clicked(gtx) {
		v.fitMode = false
		v.zoom = min(2.5, v.zoom+0.1)
	}
	if v.zoomOutBtn.Clicked(gtx) {
		v.fitMode = false
		v.zoom = max(0.4, v.zoom-0.1)
	}
	if v.zoomFitBtn.Clicked(gtx) {
		v.fitMode = true
		v.zoom = 1.0
	}
	if v.slideshowBtn.Clicked(gtx) {
		v.slideshowMode = true
	}

	totalSlides := v.snapshot.SlideCount
	if totalSlides == 0 {
		totalSlides = 1
	}

	return layout.Stack{Alignment: layout.W}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
					// Left status items: "幻灯片 X / Y", "中文（中国）", "辅助功能：正常"
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(unit.Dp(16))}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								txt := fmt.Sprintf("幻灯片 %d / %d", v.activeSlide+1, totalSlides)
								lbl := material.Label(a.theme, unit.Sp(11), txt)
								lbl.Color = pptxTextMuted
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(a.theme, unit.Sp(11), "中文（中国）")
								lbl.Color = pptxTextMuted
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(a.theme, unit.Sp(11), "辅助功能：正常")
								lbl.Color = pptxTextMuted
								return lbl.Layout(gtx)
							}),
						)
					}),
					// Right status items: 备注, 放映, 缩放控件
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(unit.Dp(12))}.Layout(gtx,
							// 备注切换按钮
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := v.ribbonBtn("status_notes_toggle")
								if btn.Clicked(gtx) {
									v.notesExpanded = !v.notesExpanded
								}
								return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									label := "备注"
									lbl := material.Label(a.theme, unit.Sp(11), label)
									if v.notesExpanded {
										lbl.Color = pptxAccentColor
										lbl.Font.Weight = font.SemiBold
									} else {
										lbl.Color = pptxTextMuted
									}
									return lbl.Layout(gtx)
								})
							}),
							// 放映按钮
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return v.slideshowBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(a.theme, unit.Sp(11), "幻灯片放映")
									lbl.Color = pptxAccentColor
									lbl.Font.Weight = font.Medium
									return lbl.Layout(gtx)
								})
							}),
							// 缩放减 "-"
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return v.zoomOutBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(a.theme, unit.Sp(13), "−")
									lbl.Color = pptxTextMuted
									return lbl.Layout(gtx)
								})
							}),
							// 缩放百分比
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								zoomText := fmt.Sprintf("%.0f%%", v.zoom*100)
								lbl := material.Label(a.theme, unit.Sp(11), zoomText)
								lbl.Color = pptxTextMuted
								return lbl.Layout(gtx)
							}),
							// 缩放加 "+"
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return v.zoomInBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(a.theme, unit.Sp(13), "+")
									lbl.Color = pptxTextMuted
									return lbl.Layout(gtx)
								})
							}),
							// 适应窗口
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return v.zoomFitBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(a.theme, unit.Sp(11), "适应")
									if v.fitMode {
										lbl.Color = pptxAccentColor
									} else {
										lbl.Color = pptxTextMuted
									}
									return lbl.Layout(gtx)
								})
							}),
						)
					}),
				)
			})
		}),
	)
}

// layoutPresentationSlideshow renders the full presentation mode overlay.
func (a *App) layoutPresentationSlideshow(gtx layout.Context, v *presentationViewState) layout.Dimensions {
	darkBg := color.NRGBA{R: 24, G: 26, B: 30, A: 255}
	paint.FillShape(gtx.Ops, darkBg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	// Handle controls
	if v.exitShowBtn.Clicked(gtx) {
		v.slideshowMode = false
	}
	if v.prevSlideBtn.Clicked(gtx) {
		if v.activeSlide > 0 {
			v.activeSlide--
		}
	}
	if v.nextSlideBtn.Clicked(gtx) {
		if v.activeSlide < v.snapshot.SlideCount-1 {
			v.activeSlide++
		}
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		// Slide canvas centered
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return a.layoutPresentationCanvas(gtx, v)
		}),
		// Bottom floating slideshow controls
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			shift := op.Offset(image.Pt(0, gtx.Constraints.Max.Y/2 - gtx.Dp(unit.Dp(40)))).Push(gtx.Ops)
			defer shift.Pop()

			bg := color.NRGBA{R: 40, G: 44, B: 52, A: 230}
			shape := clip.UniformRRect(image.Rectangle{Max: image.Pt(gtx.Dp(unit.Dp(320)), gtx.Dp(unit.Dp(44)))}, gtx.Dp(unit.Dp(22)))
			paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))

			return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16), Top: unit.Dp(10), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(unit.Dp(16))}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return v.prevSlideBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(a.theme, unit.Sp(12), "◀ 上一页")
							lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
							return lbl.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						txt := fmt.Sprintf("%d / %d", v.activeSlide+1, v.snapshot.SlideCount)
						lbl := material.Label(a.theme, unit.Sp(12), txt)
						lbl.Color = color.NRGBA{R: 200, G: 205, B: 215, A: 255}
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return v.nextSlideBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(a.theme, unit.Sp(12), "下一页 ▶")
							lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
							return lbl.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return v.exitShowBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(a.theme, unit.Sp(12), "退出 (Esc)")
							lbl.Color = color.NRGBA{R: 255, G: 120, B: 120, A: 255}
							return lbl.Layout(gtx)
						})
					}),
				)
			})
		}),
	)
}

func (a *App) handlePresentationAddSlide(v *presentationViewState) {
	if a.pptxDoc == nil {
		return
	}
	if err := a.pptxDoc.AddSlide(); err != nil {
		a.setStatus("新建幻灯片失败: " + err.Error())
		return
	}
	v.snapshot = a.pptxDoc.Snapshot()
	v.activeSlide = v.snapshot.SlideCount - 1
	v.selectedShape = 0
	v.editingShape = 0
	if cur := v.currentSlide(); cur != nil {
		v.notesEditor.SetText(cur.Notes)
	}
	a.setStatus("已新建幻灯片")
}

func (a *App) handlePresentationDeleteSlide(v *presentationViewState) {
	if a.pptxDoc == nil {
		return
	}
	if v.snapshot.SlideCount <= 1 {
		a.setStatus("无法删除演示文稿中的最后一页幻灯片")
		return
	}
	if err := a.pptxDoc.DeleteSlide(v.activeSlide); err != nil {
		a.setStatus("删除幻灯片失败: " + err.Error())
		return
	}
	v.snapshot = a.pptxDoc.Snapshot()
	if v.activeSlide >= v.snapshot.SlideCount {
		v.activeSlide = v.snapshot.SlideCount - 1
	}
	v.selectedShape = 0
	v.editingShape = 0
	if cur := v.currentSlide(); cur != nil {
		v.notesEditor.SetText(cur.Notes)
	}
	a.setStatus("已删除幻灯片")
}

func (a *App) handlePresentationAddTextBox(v *presentationViewState) {
	if a.pptxDoc == nil {
		return
	}
	a.setStatus("点击幻灯片画布上的文本框可直接编辑内容")
}
