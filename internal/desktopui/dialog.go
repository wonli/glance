package desktopui

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

type modalDialogActionSpec struct {
	ID, Label  string
	Style      modalDialogActionStyle
	WidthDp    float32
	TextSizeSp float32
}

type modalDialogAction struct {
	ID, Label  string
	Style      modalDialogActionStyle
	WidthDp    float32
	TextSizeSp float32
	button     widget.Clickable
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
	AgentHistoryDelete      bool
	Visible                 bool
	Title                   string
	Message                 string
	Actions                 []modalDialogAction
	Input                   widget.Editor
	SecondInput             widget.Editor
	FormInputs              [5]widget.Editor
	FormLabels              [5]string
	FormHints               [5]string
	FormInputCount          int
	InputHint               string
	SecondInputHint         string
	HasInput                bool
	HasSecondInput          bool
	HasForm                 bool
	HasConfirmStyle         bool
	ConfirmStyle            modalDialogConfirmStyle
	CardWidthDp             int
	CustomLayout            func(gtx layout.Context) layout.Dimensions
	CustomActionsInLayout   bool
	SearchReplace           bool
	FocusInputRequested     bool
	SearchReplaceCompact    bool
	SearchReplaceHasReplace bool
	SearchReplaceReplaceTab bool
	SearchReplaceScope      string
	SearchReplaceSummary    string
	searchReplaceFindTab    widget.Clickable
	searchReplaceReplaceTab widget.Clickable
	searchReplaceClose      widget.Clickable
	scrim                   widget.Clickable
	OnAction                func(string)
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
	d.CardWidthDp = 0
	d.AgentHistoryDelete = false
	d.CustomLayout = nil
	d.CustomActionsInLayout = false
	d.SearchReplace = false
	d.FocusInputRequested = false
	d.SearchReplaceCompact = false
	d.SearchReplaceHasReplace = false
	d.SearchReplaceReplaceTab = false
	d.SearchReplaceScope = ""
	d.SearchReplaceSummary = ""
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
			ID:         action.ID,
			Label:      action.Label,
			Style:      resolveDialogActionStyle(action),
			WidthDp:    action.WidthDp,
			TextSizeSp: action.TextSizeSp,
		}
	}
}

func (d *modalDialog) ShowConfirm(title, message string, style modalDialogConfirmStyle, onAction func(string), actions ...modalDialogActionSpec) {
	d.Show(title, message, onAction, actions...)
	d.HasConfirmStyle = true
	d.ConfirmStyle = style
}

func (d *modalDialog) ShowCustom(title, message string, cardWidthDp int, customLayout func(gtx layout.Context) layout.Dimensions, onAction func(string), actions ...modalDialogActionSpec) {
	d.Show(title, message, onAction, actions...)
	d.CardWidthDp = cardWidthDp
	d.CustomLayout = customLayout
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

func (d *modalDialog) cancelFromEscape() {
	actionID := ""
	if d.SearchReplace {
		actionID = "close"
	} else {
		for _, action := range d.Actions {
			if action.ID == "cancel" {
				actionID = action.ID
				break
			}
		}
		if actionID == "" {
			for _, action := range d.Actions {
				if action.ID == "close" {
					actionID = action.ID
					break
				}
			}
		}
	}
	if d.OnAction != nil && actionID != "" {
		title := d.Title
		d.OnAction(actionID)
		if d.Visible && d.Title == title {
			d.Hide()
		}
		return
	}
	d.Hide()
}

func (d *modalDialog) hasActiveComposition() bool {
	if d.HasInput && editorHasActiveComposition(&d.Input) {
		return true
	}
	if d.HasSecondInput && editorHasActiveComposition(&d.SecondInput) {
		return true
	}
	for i := 0; i < d.FormInputCount; i++ {
		if editorHasActiveComposition(&d.FormInputs[i]) {
			return true
		}
	}
	return false
}

// handleOverlayEscape routes Escape to the visually topmost transient surface.
func (a *App) handleOverlayEscape(gtx layout.Context) bool {
	if a == nil {
		return false
	}
	confirmation := a.activeAgentConfirmation()
	state := a.documentSessionState
	hasRibbonPopup := state != nil && (state.officeRibbonMenu.kind != officeRibbonMenuClosed || state.toolbarPicker != pickerNone)
	if confirmation == nil && !a.modal.Visible && !a.wordSymbolPickerVisible && !a.wordVersionHistoryVisible && !a.showNewMenu && !a.fontPicker.open && !hasRibbonPopup {
		return false
	}
	if confirmation == nil && a.modal.Visible && a.modal.hasActiveComposition() {
		return false
	}
	if confirmation == nil && !a.modal.Visible && !a.wordSymbolPickerVisible && !a.wordVersionHistoryVisible && !a.showNewMenu && a.fontPicker.open {
		return false
	}
	event, ok := gtx.Event(key.Filter{Name: key.NameEscape})
	if !ok {
		return false
	}
	pressed, ok := event.(key.Event)
	if !ok || pressed.State != key.Press {
		return true
	}
	switch {
	case confirmation != nil:
		a.resolveAgentConfirmation(confirmation, false)
	case a.modal.Visible:
		a.modal.cancelFromEscape()
	case a.wordSymbolPickerVisible:
		a.closeWordSymbolPicker()
	case a.wordVersionHistoryVisible:
		a.closeWordVersionHistory()
	case a.showNewMenu:
		a.showNewMenu = false
	case state != nil && state.officeRibbonMenu.kind != officeRibbonMenuClosed:
		a.closeOfficeRibbonMenu()
	case state != nil && state.toolbarPicker != pickerNone:
		state.toolbarPicker = pickerNone
	}
	return true
}

func (a *App) showConfirmDialog(title, message string, style modalDialogConfirmStyle, onAction func(string), actions ...modalDialogActionSpec) {
	a.modal.ShowConfirm(title, message, style, onAction, actions...)
}

func (a *App) layoutModalDialog(gtx layout.Context) layout.Dimensions {
	if !a.modal.Visible {
		return layout.Dimensions{}
	}
	if a.modal.FocusInputRequested && a.modal.HasInput {
		gtx.Execute(key.FocusCmd{Tag: &a.modal.Input})
		a.modal.FocusInputRequested = false
	}
	for a.modal.scrim.Clicked(gtx) {
	}
	if !a.modal.CustomActionsInLayout && !a.modal.SearchReplace {
		for i := range a.modal.Actions {
			action := &a.modal.Actions[i]
			if action.button.Clicked(gtx) {
				if a.modal.OnAction != nil {
					a.modal.OnAction(action.ID)
				}
				if !a.modal.Visible {
					return layout.Dimensions{}
				}
				break
			}
		}
	}
	if a.modal.HasInput {
		for ev, ok := a.modal.Input.Update(gtx); ok; ev, ok = a.modal.Input.Update(gtx) {
			if _, submit := ev.(widget.SubmitEvent); submit && a.modal.OnAction != nil {
				if a.modal.SearchReplace {
					a.modal.OnAction("find-next")
				} else {
					a.modal.OnAction("confirm")
				}
				if !a.modal.Visible {
					return layout.Dimensions{}
				}
			}
		}
	}
	if a.modal.HasSecondInput {
		for ev, ok := a.modal.SecondInput.Update(gtx); ok; ev, ok = a.modal.SecondInput.Update(gtx) {
			if _, submit := ev.(widget.SubmitEvent); submit && a.modal.OnAction != nil {
				if a.modal.SearchReplace {
					a.modal.OnAction("find-next")
				} else {
					a.modal.OnAction("confirm")
				}
				if !a.modal.Visible {
					return layout.Dimensions{}
				}
			}
		}
	}
	if a.modal.HasForm {
		for index := 0; index < a.modal.FormInputCount; index++ {
			for ev, ok := a.modal.FormInputs[index].Update(gtx); ok; ev, ok = a.modal.FormInputs[index].Update(gtx) {
				if _, submit := ev.(widget.SubmitEvent); submit && a.modal.OnAction != nil {
					a.modal.OnAction("confirm")
					if !a.modal.Visible {
						return layout.Dimensions{}
					}
				}
			}
		}
	}
	if a.modal.SearchReplace {
		return a.layoutOfficeSearchReplaceDialog(gtx)
	}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.layoutModalScrim(gtx)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return a.layoutConfirmDialogCard(gtx)
		}),
	)
}

func (a *App) layoutModalScrim(gtx layout.Context) layout.Dimensions {
	return a.modal.scrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Max
		scrimColor := color.NRGBA{R: 0, G: 0, B: 0, A: 96}
		paint.FillShape(gtx.Ops, scrimColor, clip.Rect{Max: size}.Op())
		return layout.Dimensions{Size: image.Pt(size.X, size.Y)}
	})
}
