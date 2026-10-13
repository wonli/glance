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
	actionLabel string
	action      widget.Clickable
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
	if strings.HasPrefix(message, "已") || strings.Contains(message, "格式正确") || message == "路径已复制" || message == "文件名已复制" {
		return feedbackSuccess
	}
	return feedbackInfo
}

// isSideEffectStatus reports whether a status message indicates an operation
// that produces observable side effects (such as document mutation, saving,
// exporting, printing, or actionable error/warning reports). Pure read,
// navigation, zoom, selection, cancellation, and dialog preparation statuses
// return false to avoid noisy, unnecessary toast prompts.
func isSideEffectStatus(status string) bool {
	s := strings.TrimSpace(status)
	if s == "" {
		return false
	}

	// 1. Explicit pure / read-only / navigation / transient operations without side effects:
	// Cancellations
	if strings.Contains(s, "已取消") || strings.Contains(s, "已丢弃") {
		return false
	}
	// Zoom
	if strings.Contains(s, "缩放") {
		return false
	}
	// Selection & caret navigation
	if strings.Contains(s, "全选") || strings.Contains(s, "已定位至") {
		return false
	}
	// View modes & layout toggles
	if strings.Contains(s, "并排查看") || strings.Contains(s, "连续阅读") ||
		strings.Contains(s, "大纲视图") || strings.Contains(s, "页面视图") ||
		strings.Contains(s, "阅读视图") || strings.Contains(s, "只读预览") {
		return false
	}
	// File-menu copies explicitly promise a confirmation toast.
	if s == "路径已复制" || s == "文件名已复制" {
		return true
	}
	// Clipboard copy (read-only)
	if strings.Contains(s, "已复制") {
		return false
	}
	// Dialog & picker preparation
	if strings.HasPrefix(s, "选择要") || strings.HasPrefix(s, "选择用于") ||
		strings.HasPrefix(s, "选择另存为") || strings.HasPrefix(s, "选择 PDF") ||
		strings.HasPrefix(s, "选择共享") || strings.Contains(s, "选择要打开") ||
		strings.Contains(s, "选择要浏览") || strings.Contains(s, "选择保存位置") ||
		strings.Contains(s, "等待确认") {
		return false
	}
	// Ongoing transitions
	if strings.HasPrefix(s, "正在打开") || strings.HasPrefix(s, "正在读取") ||
		strings.HasPrefix(s, "正在生成") || strings.HasPrefix(s, "正在准备") ||
		strings.HasPrefix(s, "正在为") || strings.HasPrefix(s, "正在按") ||
		strings.HasPrefix(s, "正在提交") || strings.HasPrefix(s, "正在编辑") ||
		strings.HasPrefix(s, "正在处理") {
		return false
	}
	// Idle / recent files / preview
	if s == "最近打开的本地文档" || s == "最近文件操作" || s == "没有活动文档" ||
		s == "打开文件开始预览" || s == "就绪" || strings.Contains(s, "虚拟网格就绪") ||
		strings.HasPrefix(s, "已打开 ") || strings.Contains(s, "已读入") ||
		strings.Contains(s, "到最近打开") {
		return false
	}
	// Inspection & tools arming
	if strings.HasPrefix(s, "已启动格式刷") || strings.Contains(s, "字数统计") ||
		strings.Contains(s, "拼写检查完成") || strings.Contains(s, "翻译查询完成") {
		return false
	}

	// 2. Errors and actionable warnings:
	for _, word := range []string{"失败", "错误", "无效", "不能为空", "超过"} {
		if strings.Contains(s, word) {
			return true
		}
	}
	for _, word := range []string{"只读", "不可用", "暂未实现", "无法", "不支持", "需确认", "请重试", "请先"} {
		if strings.Contains(s, word) {
			return true
		}
	}

	// 3. Operations with side effects (saving, exports, mutations, format patches, etc.):
	// Persistence and external I/O
	if strings.Contains(s, "保存") || strings.Contains(s, "另存为") ||
		strings.Contains(s, "导出") || strings.Contains(s, "打印") ||
		strings.Contains(s, "恢复历史版本") {
		return true
	}

	// Content insertions
	if strings.Contains(s, "插入") {
		return true
	}

	// Content deletions / cleanups
	if strings.Contains(s, "删除") || strings.Contains(s, "清除") ||
		strings.Contains(s, "移除") || strings.Contains(s, "清空") {
		return true
	}

	// Mutations, styling, property changes
	if strings.Contains(s, "设置") || strings.Contains(s, "应用") ||
		strings.Contains(s, "更新") || strings.Contains(s, "修改") ||
		strings.Contains(s, "调整") || strings.Contains(s, "替换") ||
		strings.Contains(s, "填入") || strings.Contains(s, "语言") ||
		strings.Contains(s, "页眉") || strings.Contains(s, "页脚") ||
		strings.Contains(s, "页码") {
		return true
	}

	// Table, spreadsheet & data mutations
	if strings.Contains(s, "排序") || strings.Contains(s, "筛选") ||
		strings.Contains(s, "快速填充") || strings.Contains(s, "分列") ||
		strings.Contains(s, "条件格式") || strings.Contains(s, "数据验证") ||
		strings.Contains(s, "定义名称") || strings.Contains(s, "重新计算") ||
		strings.Contains(s, "重命名") || strings.Contains(s, "新建工作表") ||
		strings.Contains(s, "透视表") || strings.Contains(s, "求和完成") ||
		strings.Contains(s, "冻结") || strings.Contains(s, "解冻") {
		return true
	}

	// Review, track revisions, protection, macro security
	if strings.Contains(s, "批注") || strings.Contains(s, "修订") ||
		strings.Contains(s, "保护") || strings.Contains(s, "限制编辑") ||
		strings.Contains(s, "编辑限制") || strings.Contains(s, "宏安全策略") {
		return true
	}

	// Document tools, formatting, validation
	if strings.Contains(s, "格式化") || strings.Contains(s, "压缩") ||
		strings.Contains(s, "格式正确") || strings.Contains(s, "设计器") {
		return true
	}

	// Undo / Redo / Cut
	if strings.Contains(s, "撤销") || strings.Contains(s, "重做") ||
		strings.Contains(s, "剪切") {
		return true
	}

	return false
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
	if !isSideEffectStatus(status) {
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
func (s *feedbackState) dismiss()                   { s.message = ""; s.actionLabel = "" }

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
	if s.actionLabel != "" && s.action.Clicked(gtx) {
		a.undoHomeRecentRemoval()
	}
	if s.close.Clicked(gtx) {
		s.dismiss()
	}
	if !s.visible(gtx.Now) && s.actionLabel != "" {
		s.actionLabel = ""
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
					if a.feedback.actionLabel == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						buttonWidth, buttonHeight := gtx.Dp(unit.Dp(48)), gtx.Dp(unit.Dp(28))
						gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(buttonWidth, buttonHeight), image.Pt(buttonWidth, buttonHeight)
						return a.feedback.action.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							bg := color.NRGBA{A: 0}
							if a.feedback.action.Hovered() {
								bg = homeHoverFill
							}
							shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(6)))
							paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, a.feedback.actionLabel, 12, color.NRGBA{R: 52, G: 120, B: 209, A: 255}, true)
							})
						})
					})
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
	a.feedback.actionLabel = ""
}
