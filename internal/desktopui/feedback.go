package desktopui

import (
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/i18n"
)

type feedbackKind uint8

const (
	feedbackInfo feedbackKind = iota
	feedbackSuccess
	feedbackWarning
	feedbackError
)

type notificationPolicy uint8

const (
	notificationSuppressed notificationPolicy = iota
	notificationToast
)

type feedbackState struct {
	kind           feedbackKind
	messageRef     i18n.Message
	policy         notificationPolicy
	detail         string
	until          time.Time
	close          widget.Clickable
	actionLabel    string
	actionLabelRef i18n.Message
	action         widget.Clickable
	detailButton   widget.Clickable
}

func (s *feedbackState) showMessage(message i18n.Message, kind feedbackKind, policy notificationPolicy, detail string, now time.Time) {
	s.policy = policy
	s.kind = kind
	s.messageRef = message
	s.detail = detail
	s.actionLabel = ""
	s.actionLabelRef = i18n.Message{}
	duration := 4 * time.Second
	if kind == feedbackError || kind == feedbackWarning {
		duration = 8 * time.Second
	}
	s.until = now.Add(duration)
}

func (s *feedbackState) visible(now time.Time) bool {
	return s.policy == notificationToast && s.messageRef.ID != "" && now.Before(s.until)
}

func (s *feedbackState) dismiss() {
	s.messageRef = i18n.Message{}
	s.detail = ""
	s.actionLabel = ""
}

type feedbackStyle struct {
	title     string
	titleRef  i18n.Message
	icon      string
	fill, ink color.NRGBA
}

func feedbackAppearance(kind feedbackKind) feedbackStyle {
	switch kind {
	case feedbackSuccess:
		return feedbackStyle{titleRef: i18n.Message{ID: "common.feedback.title.success"}, icon: "circle-check", fill: color.NRGBA{R: 234, G: 247, B: 240, A: 255}, ink: color.NRGBA{R: 46, G: 139, B: 87, A: 255}}
	case feedbackWarning:
		return feedbackStyle{titleRef: i18n.Message{ID: "common.feedback.title.warning"}, icon: "triangle-alert", fill: color.NRGBA{R: 255, G: 245, B: 229, A: 255}, ink: color.NRGBA{R: 194, G: 122, B: 22, A: 255}}
	case feedbackError:
		return feedbackStyle{titleRef: i18n.Message{ID: "common.feedback.title.error"}, icon: "circle-x", fill: color.NRGBA{R: 253, G: 238, B: 238, A: 255}, ink: color.NRGBA{R: 199, G: 68, B: 68, A: 255}}
	default:
		return feedbackStyle{titleRef: i18n.Message{ID: "common.feedback.title.info"}, icon: "info", fill: color.NRGBA{R: 234, G: 242, B: 252, A: 255}, ink: color.NRGBA{R: 52, G: 120, B: 209, A: 255}}
	}
}

// Every format publishes operation statuses on the window thread. Observe them
// once here so transient feedback uses one component without changing document layout.
func (a *App) layoutFeedback(gtx layout.Context) layout.Dimensions {
	s := &a.feedback
	if s.actionLabel != "" && s.action.Clicked(gtx) {
		a.undoHomeRecentRemoval()
	}
	if s.detail != "" && s.detailButton.Clicked(gtx) {
		a.showFeedbackErrorDetails()
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
	if a.feedback.messageRef.ID == "spreadsheet.status.pdf_exported" || a.feedback.messageRef.ID == "markdown.pdf.export.status.exported_with_warnings" {
		style.titleRef = i18n.Message{ID: "common.feedback.title.export_complete"}
	}
	titleText := a.messageText(style.titleRef, style.title)
	messageText := a.text(a.feedback.messageRef)
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
							return widget.Image{Src: iconImageOp(icon), Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
						})
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							title := material.Label(a.theme, 13, titleText)
							title.Font.Weight = font.SemiBold
							title.Color = color.NRGBA{R: 39, G: 48, B: 58, A: 255}
							title.MaxLines = 1
							return title.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								label := material.Label(a.theme, 12, jsonTextPreview(messageText, 2048))
								label.Color = color.NRGBA{R: 115, G: 124, B: 134, A: 255}
								label.MaxLines = 2
								return label.Layout(gtx)
							})
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					actionLabel := a.messageText(a.feedback.actionLabelRef, a.feedback.actionLabel)
					if actionLabel == "" {
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
								return homeTextLabel(gtx, a.theme, actionLabel, 12, color.NRGBA{R: 52, G: 120, B: 209, A: 255}, true)
							})
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if a.feedback.detail == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						buttonWidth, buttonHeight := gtx.Dp(unit.Dp(52)), gtx.Dp(unit.Dp(28))
						gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(buttonWidth, buttonHeight), image.Pt(buttonWidth, buttonHeight)
						return a.feedback.detailButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, a.text(i18n.Message{ID: "common.feedback.details"}), 12, color.NRGBA{R: 52, G: 120, B: 209, A: 255}, true)
							})
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(20), gtx.Dp(24)))
						return a.feedback.close.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							jsonButtonSemantics(gtx, a.textID("word.accessibility.close_notice"), true)
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

func (a *App) setStatusMessage(message i18n.Message, kind feedbackKind, policy notificationPolicy) {
	a.statusRef = message
	a.feedback.showMessage(message, kind, policy, "", time.Now())
}

func (a *App) setStatusErrorMessage(message i18n.Message, source error, policy notificationPolicy) {
	detail := ""
	if source != nil {
		detail = source.Error()
	}
	a.setStatusDetailMessage(message, feedbackError, policy, detail)
}

func (a *App) setStatusDetailMessage(message i18n.Message, kind feedbackKind, policy notificationPolicy, detail string) {
	a.statusRef = message
	a.feedback.showMessage(message, kind, policy, detail, time.Now())
}

func (a *App) clearStatus() {
	a.statusRef = i18n.Message{}
	a.feedback.dismiss()
}

func (a *App) showFeedbackErrorDetails() {
	if a.feedback.detail == "" {
		return
	}
	a.modal.ShowMessage("common.error-details",
		i18n.Message{ID: "common.error_details.title"},
		i18n.Message{ID: "common.error_details.body", Args: i18n.Args{"Details": a.feedback.detail}},
		func(string) { a.modal.Hide() },
		modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}},
	)
}
