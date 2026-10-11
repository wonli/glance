package desktopui

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

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
	if v.presenterMode {
		return a.layoutPresentationPresenter(gtx, v)
	}
	darkBg := color.NRGBA{R: 24, G: 26, B: 30, A: 255}
	paint.FillShape(gtx.Ops, darkBg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	// Handle controls
	if v.exitShowBtn.Clicked(gtx) {
		if _, err := a.applyPresentationViewIntent("presentation.view.set", json.RawMessage(`{"operation":"set_mode","mode":"normal"}`)); err != nil {
			a.setStatus("退出放映失败：" + err.Error())
		}
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
			shift := op.Offset(image.Pt(0, gtx.Constraints.Max.Y/2-gtx.Dp(unit.Dp(40)))).Push(gtx.Ops)
			defer shift.Pop()

			bg := color.NRGBA{R: 40, G: 44, B: 52, A: 230}
			shape := clip.UniformRRect(image.Rectangle{Max: image.Pt(gtx.Dp(unit.Dp(420)), gtx.Dp(unit.Dp(44)))}, gtx.Dp(unit.Dp(22)))
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
						if v.rehearsalElapsed == 0 && !v.rehearsalRunning() {
							return layout.Dimensions{}
						}
						label := "计时 " + formatPresentationRehearsalDuration(v.rehearsalDuration(time.Now()))
						if !v.rehearsalRunning() {
							label += " 已停止"
						}
						lbl := material.Label(a.theme, unit.Sp(12), label)
						lbl.Color = color.NRGBA{R: 255, G: 211, B: 125, A: 255}
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
