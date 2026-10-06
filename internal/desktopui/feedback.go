package desktopui

import (
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type feedbackKind uint8

const (
	feedbackInfo feedbackKind = iota
	feedbackSuccess
	feedbackWarning
	feedbackError
)

type feedbackState struct {
	initialized bool
	lastStatus  string
	pending     bool
	kind        feedbackKind
	message     string
	until       time.Time
	close       widget.Clickable
}

func feedbackSeverity(message string) feedbackKind {
	for _, word := range []string{"失败", "错误", "无效", "不能为空", "超过"} {
		if strings.Contains(message, word) {
			return feedbackError
		}
	}
	for _, word := range []string{"只读", "不可用", "暂未实现", "请稍", "请重试", "已取消", "需确认", "暂不", "无法", "不支持"} {
		if strings.Contains(message, word) {
			return feedbackWarning
		}
	}
	if strings.HasPrefix(message, "已") || strings.Contains(message, "格式正确") {
		return feedbackSuccess
	}
	return feedbackInfo
}

func (s *feedbackState) observe(status string, now time.Time) {
	if !s.initialized {
		s.initialized = true
		s.lastStatus = status
		return
	}
	if status == s.lastStatus && !s.pending {
		return
	}
	s.pending = false
	s.lastStatus = status
	if strings.TrimSpace(status) == "" {
		return
	}
	s.kind = feedbackSeverity(status)
	s.message = status
	s.until = now.Add(4 * time.Second)
	if s.kind == feedbackError || s.kind == feedbackWarning {
		s.until = now.Add(8 * time.Second)
	}
}
func (s *feedbackState) visible(now time.Time) bool { return s.message != "" && now.Before(s.until) }
func (s *feedbackState) dismiss()                   { s.message = "" }

type feedbackStyle struct {
	title, icon string
	fill, ink   color.NRGBA
}

func feedbackAppearance(kind feedbackKind) feedbackStyle {
	switch kind {
	case feedbackSuccess:
		return feedbackStyle{"操作完成", "circle-check", color.NRGBA{R: 234, G: 247, B: 240, A: 255}, color.NRGBA{R: 46, G: 139, B: 87, A: 255}}
	case feedbackWarning:
		return feedbackStyle{"需要确认", "triangle-alert", color.NRGBA{R: 255, G: 245, B: 229, A: 255}, color.NRGBA{R: 194, G: 122, B: 22, A: 255}}
	case feedbackError:
		return feedbackStyle{"操作失败", "circle-x", color.NRGBA{R: 253, G: 238, B: 238, A: 255}, color.NRGBA{R: 199, G: 68, B: 68, A: 255}}
	default:
		return feedbackStyle{"提示", "info", color.NRGBA{R: 234, G: 242, B: 252, A: 255}, color.NRGBA{R: 52, G: 120, B: 209, A: 255}}
	}
}

// Every format publishes operation statuses on the window thread. Observe them
// once here so transient feedback uses one component without changing document layout.
func (a *App) layoutFeedback(gtx layout.Context) layout.Dimensions {
	s := &a.feedback
	s.observe(a.status, gtx.Now)
	if s.close.Clicked(gtx) {
		s.dismiss()
	}
	if !s.visible(gtx.Now) || a.modal.Visible {
		return layout.Dimensions{}
	}
	gtx.Execute(op.InvalidateCmd{At: s.until})
	return layout.NE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 160, Right: 16, Left: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			width := min(gtx.Dp(390), gtx.Constraints.Max.X)
			gtx.Constraints = layout.Exact(image.Pt(width, gtx.Dp(88)))
			return a.layoutFeedbackCard(gtx)
		})
	})
}

func (a *App) layoutFeedbackCard(gtx layout.Context) layout.Dimensions {
	style := feedbackAppearance(a.feedback.kind)
	if strings.HasPrefix(a.feedback.message, "PDF 已导出，") {
		style.title = "导出完成"
	}
	bounds := image.Rectangle{Max: gtx.Constraints.Max}
	for _, shadow := range []struct{ offset, alpha int }{{6, 5}, {4, 9}} {
		paint.FillShape(gtx.Ops, color.NRGBA{R: 31, G: 41, B: 55, A: uint8(shadow.alpha)}, clip.UniformRRect(bounds.Add(image.Pt(0, gtx.Dp(unit.Dp(shadow.offset)))), gtx.Dp(10)).Op(gtx.Ops))
	}
	return homeRoundedPanel(gtx, contentSurface, color.NRGBA{R: 220, G: 225, B: 231, A: 255}, 10, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(34), gtx.Dp(34)))
						paint.FillShape(gtx.Ops, style.fill, clip.Ellipse(image.Rectangle{Max: gtx.Constraints.Max}).Op(gtx.Ops))
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							icon, _ := svgIconTint("feedback", style.icon, style.ink)
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(18), gtx.Dp(18)))
							return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
						})
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							title := material.Label(a.theme, 13, style.title)
							title.Font.Weight = font.SemiBold
							title.Color = color.NRGBA{R: 39, G: 48, B: 58, A: 255}
							title.MaxLines = 1
							return title.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								label := material.Label(a.theme, 12, jsonTextPreview(a.feedback.message, 2048))
								label.Color = color.NRGBA{R: 115, G: 124, B: 134, A: 255}
								label.MaxLines = 2
								return label.Layout(gtx)
							})
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(20), gtx.Dp(24)))
						return a.feedback.close.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							jsonButtonSemantics(gtx, "关闭通知", true)
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return jsonLabel(gtx, a, "×", 16, homeMuted, false, false)
							})
						})
					})
				}),
			)
		})
	})
}

// setStatus publishes an operation event, including repeated results with the
// same text. Frame observation consumes it once without restarting the timer.
func (a *App) setStatus(message string) {
	a.status = message
	a.feedback.initialized = true
	a.feedback.pending = true
}
