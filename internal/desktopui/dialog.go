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
}

type modalDialogAction struct {
	ID, Label string
	button    widget.Clickable
}

// modalDialog is a reusable, stateful modal surface for app-level prompts.
type modalDialog struct {
	Visible   bool
	Title     string
	Message   string
	Actions   []modalDialogAction
	Input     widget.Editor
	InputHint string
	HasInput  bool
	scrim     widget.Clickable
	OnAction  func(string)
}

func (d *modalDialog) Show(title, message string, onAction func(string), actions ...modalDialogActionSpec) {
	d.Title, d.Message, d.Visible = title, message, true
	d.HasInput = false
	d.InputHint = ""
	d.Input.SetText("")
	d.OnAction = onAction
	d.Actions = make([]modalDialogAction, len(actions))
	for i, action := range actions {
		d.Actions[i] = modalDialogAction{ID: action.ID, Label: action.Label}
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

func (d *modalDialog) Hide() { d.Visible = false }

func (a *App) showModalDialog(title, message string, onAction func(string), actions ...modalDialogActionSpec) {
	a.modal.Show(title, message, onAction, actions...)
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
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.modal.scrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Max
				paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 96}, clip.Rect{Max: size}.Op())
				return layout.Dimensions{Size: size}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
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
