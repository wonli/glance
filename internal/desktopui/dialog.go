package desktopui

import (
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
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

func resolveDialogActionStyle(action modalDialogActionSpec) modalDialogActionStyle {
	if action.Style != modalDialogActionDefault {
		return action.Style
	}
	switch strings.ToLower(action.ID) {
	case "cancel", "close":
		return modalDialogActionSecondary
	case "delete", "discard":
		return modalDialogActionDanger
	}
	switch action.Label {
	case "取消", "关闭":
		return modalDialogActionSecondary
	case "删除", "不保存":
		return modalDialogActionDanger
	default:
		return modalDialogActionPrimary
	}
}

func (d *modalDialog) Show(title, message string, onAction func(string), actions ...modalDialogActionSpec) {
	d.Title, d.Message, d.Visible = title, message, true
	d.HasInput = false
	d.HasSecondInput = false
	d.HasForm = false
	if style, ok := defaultConfirmStyleForTitle(title); ok {
		d.HasConfirmStyle = true
		d.ConfirmStyle = style
	} else {
		d.HasConfirmStyle = false
		d.ConfirmStyle = modalDialogConfirmStyle{}
	}
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
		d.Actions[i] = modalDialogAction{
			ID:    action.ID,
			Label: action.Label,
			Style: resolveDialogActionStyle(action),
		}
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
			return a.layoutConfirmDialogCard(gtx)
		}),
	)
}
