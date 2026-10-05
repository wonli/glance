package desktopui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type modalDialogActionSpec struct {
	ID, Label string
	Style     modalDialogActionStyle
}

type modalDialogAction struct {
	ID, Label string
	Style     modalDialogActionStyle
	button    widget.Clickable
}

type modalDialogActionStyle uint8

const (
	modalDialogActionDefault modalDialogActionStyle = iota
	modalDialogActionPrimary
	modalDialogActionSecondary
	modalDialogActionDanger
)

type modalDialogConfirmStyle struct {
	Icon           string
	IconFamily     string
	IconBackground color.NRGBA
	IconForeground color.NRGBA
}

// modalDialog is a reusable, stateful modal surface for app-level prompts.
type modalDialog struct {
	Visible         bool
	Title           string
	Message         string
	Actions         []modalDialogAction
	Input           widget.Editor
	SecondInput     widget.Editor
	FormInputs      [5]widget.Editor
	FormLabels      [5]string
	FormHints       [5]string
	FormInputCount  int
	InputHint       string
	SecondInputHint string
	HasInput        bool
	HasSecondInput  bool
	HasForm         bool
	HasConfirmStyle bool
	ConfirmStyle    modalDialogConfirmStyle
	scrim           widget.Clickable
	OnAction        func(string)
}

func (d *modalDialog) Show(title, message string, onAction func(string), actions ...modalDialogActionSpec) {
	d.Title, d.Message, d.Visible = title, message, true
	d.HasInput = false
	d.HasSecondInput = false
	d.HasForm = false
	d.HasConfirmStyle = false
	d.ConfirmStyle = modalDialogConfirmStyle{}
	d.FormInputCount = 0
	d.InputHint = ""
	d.SecondInputHint = ""
	d.Input.SetText("")
	d.SecondInput.SetText("")
	for i := range d.FormInputs {
		d.FormInputs[i].SetText("")
		d.FormLabels[i] = ""
		d.FormHints[i] = ""
	}
	d.OnAction = onAction
	d.Actions = make([]modalDialogAction, len(actions))
	for i, action := range actions {
		d.Actions[i] = modalDialogAction{ID: action.ID, Label: action.Label, Style: action.Style}
	}
}

func (d *modalDialog) ShowConfirm(title, message string, style modalDialogConfirmStyle, onAction func(string), actions ...modalDialogActionSpec) {
	d.Show(title, message, onAction, actions...)
	d.HasConfirmStyle = true
	d.ConfirmStyle = style
}

func (d *modalDialog) ShowForm(title, message string, labels, hints, initial []string, onAction func(string), actions ...modalDialogActionSpec) {
	d.Show(title, message, onAction, actions...)
	count := len(labels)
	if count > len(d.FormInputs) {
		count = len(d.FormInputs)
	}
	d.HasForm = count > 0
	d.FormInputCount = count
	for i := 0; i < count; i++ {
		d.FormLabels[i] = labels[i]
		if i < len(hints) {
			d.FormHints[i] = hints[i]
		}
		if i < len(initial) {
			d.FormInputs[i].SetText(initial[i])
		}
		d.FormInputs[i].SingleLine = true
		d.FormInputs[i].Submit = true
	}
}

func (d *modalDialog) ShowInput(title, message, initial, hint string, onAction func(string), actions ...modalDialogActionSpec) {
	d.Show(title, message, onAction, actions...)
	d.HasInput = true
	d.InputHint = hint
	d.Input.SingleLine = true
	d.Input.Submit = true
	d.Input.SetText(initial)
}

func (d *modalDialog) ShowDualInput(title, message, initial, hint, secondInitial, secondHint string, onAction func(string), actions ...modalDialogActionSpec) {
	d.ShowInput(title, message, initial, hint, onAction, actions...)
	d.HasSecondInput = true
	d.SecondInputHint = secondHint
	d.SecondInput.SingleLine = true
	d.SecondInput.Submit = true
	d.SecondInput.SetText(secondInitial)
}

func (d *modalDialog) Hide() { d.Visible = false }

func (a *App) showConfirmDialog(title, message string, style modalDialogConfirmStyle, onAction func(string), actions ...modalDialogActionSpec) {
	a.modal.ShowConfirm(title, message, style, onAction, actions...)
}

func (a *App) layoutModalDialog(gtx layout.Context) layout.Dimensions {
	if !a.modal.Visible {
		return layout.Dimensions{}
	}
	if a.modal.HasInput {
		for ev, ok := a.modal.Input.Update(gtx); ok; ev, ok = a.modal.Input.Update(gtx) {
			if _, submit := ev.(widget.SubmitEvent); submit && a.modal.OnAction != nil {
				a.modal.OnAction("confirm")
			}
		}
	}
	if a.modal.HasSecondInput {
		for ev, ok := a.modal.SecondInput.Update(gtx); ok; ev, ok = a.modal.SecondInput.Update(gtx) {
			if _, submit := ev.(widget.SubmitEvent); submit && a.modal.OnAction != nil {
				a.modal.OnAction("confirm")
			}
		}
	}
	if a.modal.HasForm {
		for index := 0; index < a.modal.FormInputCount; index++ {
			for ev, ok := a.modal.FormInputs[index].Update(gtx); ok; ev, ok = a.modal.FormInputs[index].Update(gtx) {
				if _, submit := ev.(widget.SubmitEvent); submit && a.modal.OnAction != nil {
					a.modal.OnAction("confirm")
				}
			}
		}
	}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.modal.scrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Max
				paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 96}, clip.Rect{Max: size}.Op())
				return layout.Dimensions{Size: size}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if a.modal.HasConfirmStyle {
				return a.layoutConfirmDialogCard(gtx)
			}
			maxWidth := gtx.Dp(unit.Dp(440))
			if gtx.Constraints.Max.X < maxWidth {
				maxWidth = gtx.Constraints.Max.X
			}
			gtx.Constraints.Min.X = maxWidth
			gtx.Constraints.Min.Y = 0
			gtx.Constraints.Max.X = maxWidth
			child := op.Record(gtx.Ops)
			dims := layout.Inset{Top: unit.Dp(24), Bottom: unit.Dp(18), Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{
					layout.Rigid(material.H6(a.theme, a.modal.Title).Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(24)}.Layout(gtx, material.Body1(a.theme, a.modal.Message).Layout)
					}),
				}
				if a.modal.HasInput {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layoutFormulaEditorField(gtx, a.theme, &a.modal.Input, a.modal.InputHint, 0)
						})
					}))
				}
				if a.modal.HasSecondInput {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layoutFormulaEditorField(gtx, a.theme, &a.modal.SecondInput, a.modal.SecondInputHint, 0)
						})
					}))
				}
				if a.modal.HasForm {
					for index := 0; index < a.modal.FormInputCount; index++ {
						fieldIndex := index
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										label := material.Caption(a.theme, a.modal.FormLabels[fieldIndex])
										label.Color = color.NRGBA{R: 83, G: 91, B: 102, A: 255}
										return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, label.Layout)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layoutFormulaEditorField(gtx, a.theme, &a.modal.FormInputs[fieldIndex], a.modal.FormHints[fieldIndex], 0)
									}),
								)
							})
						}))
					}
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					buttons := make([]layout.FlexChild, 0, len(a.modal.Actions))
					for i := range a.modal.Actions {
						action := &a.modal.Actions[i]
						buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if action.button.Clicked(gtx) && a.modal.OnAction != nil {
								a.modal.OnAction(action.ID)
							}
							return material.Button(a.theme, &action.button, action.Label).Layout(gtx)
						}))
					}
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx, buttons...)
				}))
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
			call := child.Stop()
			radius := gtx.Dp(unit.Dp(10))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.RRect{Rect: image.Rect(0, 0, dims.Size.X, dims.Size.Y), NW: radius, NE: radius, SW: radius, SE: radius}.Op(gtx.Ops))
			call.Add(gtx.Ops)
			return dims
		}),
	)
}
