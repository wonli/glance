package desktopui

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func (a *App) layoutPresentationPresenter(gtx layout.Context, v *presentationViewState) layout.Dimensions {
	background := color.NRGBA{R: 24, G: 26, B: 30, A: 255}
	panel := color.NRGBA{R: 35, G: 38, B: 44, A: 255}
	secondary := color.NRGBA{R: 177, G: 184, B: 197, A: 255}
	paint.FillShape(gtx.Ops, background, clip.Rect{Max: gtx.Constraints.Max}.Op())

	if v.exitShowBtn.Clicked(gtx) {
		if _, err := a.applyPresentationViewIntent("presentation.view.set", json.RawMessage(`{"operation":"set_mode","mode":"normal"}`)); err != nil {
			a.setStatus("退出演讲者视图失败：" + err.Error())
		}
	}
	if v.prevSlideBtn.Clicked(gtx) && v.activeSlide > 0 {
		v.activeSlide--
	}
	if v.nextSlideBtn.Clicked(gtx) && v.activeSlide < v.snapshot.SlideCount-1 {
		v.activeSlide++
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(0.72, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(18), Top: unit.Dp(18), Right: unit.Dp(12), Bottom: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPresentationCanvas(gtx, v)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					width := gtx.Dp(unit.Dp(1))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 65, G: 69, B: 77, A: 255}, clip.Rect{Max: image.Pt(width, gtx.Constraints.Max.Y)}.Op())
					return layout.Dimensions{Size: image.Pt(width, gtx.Constraints.Max.Y)}
				}),
				layout.Flexed(0.28, func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, panel, clip.Rect{Max: gtx.Constraints.Max}.Op())
					return layout.Inset{Left: unit.Dp(18), Top: unit.Dp(18), Right: unit.Dp(18), Bottom: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(a.theme, unit.Sp(18), "演讲者视图")
										lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
										return lbl.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										elapsed := "计时未启动"
										if v.rehearsalRunning() || v.rehearsalElapsed > 0 {
											elapsed = formatPresentationRehearsalDuration(v.rehearsalDuration(time.Now()))
										}
										lbl := material.Label(a.theme, unit.Sp(15), elapsed)
										lbl.Color = color.NRGBA{R: 255, G: 211, B: 125, A: 255}
										return lbl.Layout(gtx)
									}),
								)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(20), Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := material.Label(a.theme, unit.Sp(13), "演讲者备注")
											lbl.Color = secondary
											return lbl.Layout(gtx)
										}),
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											slide := v.currentSlide()
											notes := "此页尚无演讲者备注。"
											if slide != nil && strings.TrimSpace(slide.Notes) != "" {
												notes = slide.Notes
											}
											lbl := material.Label(a.theme, unit.Sp(15), notes)
											lbl.Color = color.NRGBA{R: 242, G: 244, B: 248, A: 255}
											return lbl.Layout(gtx)
										}),
									)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return a.layoutPresentationNextSlideSummary(gtx, v, secondary)
							}),
						)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			height := gtx.Dp(unit.Dp(58))
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
			return layout.Inset{Left: unit.Dp(18), Right: unit.Dp(18), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(a.theme, unit.Sp(12), presentationCurrentPageLabel(v))
						lbl.Color = secondary
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(unit.Dp(18))}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return v.prevSlideBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(a.theme, unit.Sp(13), "◀ 上一页")
									lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
									return lbl.Layout(gtx)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return v.nextSlideBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(a.theme, unit.Sp(13), "下一页 ▶")
									lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
									return lbl.Layout(gtx)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return v.exitShowBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(a.theme, unit.Sp(13), "退出 (Esc)")
									lbl.Color = color.NRGBA{R: 255, G: 150, B: 150, A: 255}
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

func (a *App) layoutPresentationNextSlideSummary(gtx layout.Context, v *presentationViewState, secondary color.NRGBA) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	if v.activeSlide+1 >= len(v.snapshot.Slides) {
		lbl := material.Label(a.theme, unit.Sp(13), "已是最后一页")
		lbl.Color = secondary
		return lbl.Layout(gtx)
	}
	next := v.snapshot.Slides[v.activeSlide+1]
	text := strings.TrimSpace(next.Title)
	if text == "" {
		for _, element := range next.Elements {
			if candidate := strings.TrimSpace(element.PlainText()); candidate != "" {
				text = candidate
				break
			}
		}
	}
	if len([]rune(text)) > 160 {
		text = string([]rune(text)[:160]) + "…"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(a.theme, unit.Sp(13), "下一页 · "+presentationCurrentPageLabel(v, 1))
			lbl.Color = secondary
			return lbl.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			if text == "" {
				text = "无可读取的标题或文本摘要。"
			}
			lbl := material.Label(a.theme, unit.Sp(14), text)
			lbl.Color = color.NRGBA{R: 238, G: 240, B: 244, A: 255}
			return lbl.Layout(gtx)
		}),
	)
}

func presentationCurrentPageLabel(v *presentationViewState, offset ...int) string {
	page := v.activeSlide + 1
	if len(offset) > 0 {
		page += offset[0]
	}
	return fmt.Sprintf("%d / %d", page, max(1, v.snapshot.SlideCount))
}
