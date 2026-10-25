package desktopui

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"glance/internal/i18n"
)

const (
	dialogIDAgentAssistant      = "agent.assistant"
	dialogIDAgentModelSettings  = "agent.settings.model"
	dialogIDWordMacros          = "word.macros.security"
	dialogIDWordEditorReview    = "word.review.editor"
	dialogIDWordCompare         = "word.compare"
	dialogIDWordDocumentInfo    = "word.document-info"
	dialogIDWordProtection      = "word.document-protection"
	dialogIDWordPrintWarning    = "word.print.layout-warning"
	dialogIDWordPrintPrepared   = "word.print.prepared"
	dialogIDWordPDFWarning      = "word.export.layout-warning"
	dialogIDPDFExportPreview    = "pdf.export.preview"
	dialogIDOfficeSearchReplace = "office.search-replace"
)

type modalDialogActionSpec struct {
	ID, Label  string
	LabelRef   i18n.Message
	Style      modalDialogActionStyle
	WidthDp    float32
	TextSizeSp float32
}

type modalDialogAction struct {
	ID, Label  string
	LabelRef   i18n.Message
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
	DialogID                string
	TitleRef                i18n.Message
	MessageRef              i18n.Message
	Title                   string
	Message                 string
	Actions                 []modalDialogAction
	Input                   widget.Editor
	SecondInput             widget.Editor
	FormInputs              [5]widget.Editor
	FormLabels              [5]string
	FormHints               [5]string
	FormLabelRefs           [5]i18n.Message
	FormHintRefs            [5]i18n.Message
	FormInputCount          int
	InputHint               string
	SecondInputHint         string
	InputHintRef            i18n.Message
	SecondInputHintRef      i18n.Message
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
	SearchReplaceSummaryRef i18n.Message
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
	return modalDialogActionPrimary
}

func (d *modalDialog) Show(title, message string, onAction func(string), actions ...modalDialogActionSpec) {
	d.DialogID = ""
	d.TitleRef = i18n.Message{}
	d.MessageRef = i18n.Message{}
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
	d.SearchReplaceSummaryRef = i18n.Message{}
	d.HasConfirmStyle = false
	d.ConfirmStyle = modalDialogConfirmStyle{}
	d.FormInputCount = 0
	d.InputHint = ""
	d.SecondInputHint = ""
	d.InputHintRef = i18n.Message{}
	d.SecondInputHintRef = i18n.Message{}
	d.Input.SetText("")
	d.SecondInput.SetText("")
	for i := range d.FormInputs {
		d.FormInputs[i].SetText("")
		d.FormLabels[i] = ""
		d.FormHints[i] = ""
		d.FormLabelRefs[i] = i18n.Message{}
		d.FormHintRefs[i] = i18n.Message{}
	}
	d.OnAction = onAction
	d.Actions = make([]modalDialogAction, len(actions))
	for i, action := range actions {
		d.Actions[i] = modalDialogAction{
			ID:         action.ID,
			Label:      action.Label,
			LabelRef:   action.LabelRef,
			Style:      resolveDialogActionStyle(action),
			WidthDp:    action.WidthDp,
			TextSizeSp: action.TextSizeSp,
		}
	}
}

func (d *modalDialog) SetMessageRef(message i18n.Message) {
	d.Message = ""
	d.MessageRef = message
}

func (d *modalDialog) SetMessageText(message string) {
	d.MessageRef = i18n.Message{}
	d.Message = message
}

func (d *modalDialog) SetSearchReplaceSummaryRef(message i18n.Message) {
	d.SearchReplaceSummary = ""
	d.SearchReplaceSummaryRef = message
}

func (d *modalDialog) ShowMessage(dialogID string, title, body i18n.Message, onAction func(string), actions ...modalDialogActionSpec) {
	d.Show(title.ID, body.ID, onAction, actions...)
	d.DialogID = dialogID
	d.TitleRef = title
	d.MessageRef = body
	for i := range d.Actions {
		if d.Actions[i].LabelRef.ID != "" {
			d.Actions[i].Label = d.Actions[i].LabelRef.ID
		}
	}
}

func (d *modalDialog) ShowConfirmMessage(dialogID string, title, body i18n.Message, style modalDialogConfirmStyle, onAction func(string), actions ...modalDialogActionSpec) {
	d.ShowMessage(dialogID, title, body, onAction, actions...)
	d.HasConfirmStyle = true
	d.ConfirmStyle = style
}

func (d *modalDialog) ShowCustomMessage(dialogID string, title, body i18n.Message, cardWidthDp int, customLayout func(gtx layout.Context) layout.Dimensions, onAction func(string), actions ...modalDialogActionSpec) {
	d.ShowMessage(dialogID, title, body, onAction, actions...)
	d.CardWidthDp = cardWidthDp
	d.CustomLayout = customLayout
}

func (d *modalDialog) ShowFormMessage(dialogID string, title, body i18n.Message, labels, hints []i18n.Message, initial []string, onAction func(string), actions ...modalDialogActionSpec) {
	d.ShowMessage(dialogID, title, body, onAction, actions...)
	count := min(len(labels), len(d.FormInputs))
	d.HasForm = count > 0
	d.FormInputCount = count
	for i := 0; i < count; i++ {
		d.FormLabelRefs[i] = labels[i]
		if i < len(hints) {
			d.FormHintRefs[i] = hints[i]
		}
		if i < len(initial) {
			d.FormInputs[i].SetText(initial[i])
		}
		d.FormInputs[i].SingleLine = true
		d.FormInputs[i].Submit = true
	}
}

func (d *modalDialog) ShowInputMessage(dialogID string, title, body i18n.Message, initial string, hint i18n.Message, onAction func(string), actions ...modalDialogActionSpec) {
	d.ShowMessage(dialogID, title, body, onAction, actions...)
	d.HasInput = true
	d.InputHintRef = hint
	d.Input.SingleLine = true
	d.Input.Submit = true
	d.Input.SetText(initial)
}

func (d *modalDialog) ShowDualInputMessage(dialogID string, title, body, firstHint, secondHint i18n.Message, firstInitial, secondInitial string, onAction func(string), actions ...modalDialogActionSpec) {
	d.ShowInputMessage(dialogID, title, body, firstInitial, firstHint, onAction, actions...)
	d.HasSecondInput = true
	d.SecondInputHintRef = secondHint
	d.SecondInput.SingleLine = true
	d.SecondInput.Submit = true
	d.SecondInput.SetText(secondInitial)
}

// String-based helpers remain for interaction-test fixtures. Production dialogs use message references.
func (d *modalDialog) ShowCustom(title, message string, cardWidthDp int, customLayout func(gtx layout.Context) layout.Dimensions, onAction func(string), actions ...modalDialogActionSpec) {
	d.Show(title, message, onAction, actions...)
	d.CardWidthDp = cardWidthDp
	d.CustomLayout = customLayout
}

func (d *modalDialog) ShowForm(title, message string, labels, hints, initial []string, onAction func(string), actions ...modalDialogActionSpec) {
	d.Show(title, message, onAction, actions...)
	count := min(len(labels), len(d.FormInputs))
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

func (d *modalDialog) submitAction(replacementField bool) string {
	if !d.SearchReplace {
		return "confirm"
	}
	if replacementField && d.SearchReplaceHasReplace && d.SearchReplaceReplaceTab {
		return "replace"
	}
	return "find-next"
}

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
		dialogID := d.DialogID
		d.OnAction(actionID)
		if d.Visible && d.DialogID == dialogID {
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
	localAccess := a.activeAgentLocalAccessConsent()
	confirmation := a.activeAgentConfirmation()
	state := a.documentSessionState
	hasRibbonPopup := state != nil && (state.officeRibbonMenu.kind != officeRibbonMenuClosed || state.toolbarPicker != pickerNone)
	hasWordColorPicker := state != nil && (state.wordToolbarColorPicker || state.wordToolbarHighlightPicker)
	hasAgentHistorySurface := a.aiHistoryDrawerOpen || a.agentUI.conversationHistoryVisible
	hasProofreadingHistory := a.proofreadingWorkbench != nil && a.proofreadingWorkbench.historyOpen
	hasProofreadingOCRLink := a.proofreadingOCRLinkValid()
	hasPresentationStylePopover := state != nil && state.pptxView != nil && state.pptxView.stylePopover != ""
	if localAccess == nil && confirmation == nil && !a.modal.Visible && !a.wordSymbolPickerVisible && !a.wordVersionHistoryVisible && !a.showNewMenu && !a.fontPicker.open && !hasRibbonPopup && !hasWordColorPicker && !hasAgentHistorySurface && !hasProofreadingHistory && !hasProofreadingOCRLink && !hasPresentationStylePopover {
		return false
	}
	if localAccess == nil && confirmation == nil && a.modal.Visible && a.modal.hasActiveComposition() {
		return false
	}
	if localAccess == nil && confirmation == nil && !a.modal.Visible && hasAgentHistorySurface && editorHasActiveComposition(&a.agentUI.historySearch) {
		return false
	}
	if localAccess == nil && confirmation == nil && !a.modal.Visible && !a.wordSymbolPickerVisible && !a.wordVersionHistoryVisible && !a.showNewMenu && a.fontPicker.open {
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
	case localAccess != nil:
		a.resolveAgentLocalAccessConsent(localAccess, false)
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
	case a.aiHistoryDrawerOpen:
		a.aiHistoryDrawerOpen = false
		gtx.Execute(key.FocusCmd{Tag: &a.aiDrawerOpenBtn})
	case hasProofreadingHistory:
		a.proofreadingWorkbench.historyRequest++
		a.proofreadingWorkbench.historyOpen = false
		gtx.Execute(key.FocusCmd{Tag: a.proofreadingWorkbench.button("workspace-history")})
	case hasProofreadingOCRLink:
		a.clearProofreadingOCRLink(true)
	case a.agentUI.conversationHistoryVisible:
		a.agentUI.conversationHistoryVisible = false
		gtx.Execute(key.FocusCmd{Tag: &a.agentUI.conversationTitleButton})
	case state != nil && state.officeRibbonMenu.kind != officeRibbonMenuClosed:
		anchor := a.officeRibbonMenuAnchorButton(state)
		a.closeOfficeRibbonMenu()
		if anchor != nil {
			gtx.Execute(key.FocusCmd{Tag: anchor})
		}
	case state != nil && state.toolbarPicker != pickerNone:
		state.toolbarPicker = pickerNone
	case hasPresentationStylePopover:
		state.pptxView.stylePopover = ""
		state.pptxView.stylePopoverShapeID = 0
		gtx.Execute(key.FocusCmd{Tag: &state.pptxView.canvasBlankClick})
	case hasWordColorPicker:
		state.wordToolbarColorPicker = false
		state.wordToolbarHighlightPicker = false
	}
	return true
}

func (a *App) showConfirmMessage(dialogID string, title, message i18n.Message, style modalDialogConfirmStyle, onAction func(string), actions ...modalDialogActionSpec) {
	a.modal.ShowConfirmMessage(dialogID, title, message, style, onAction, actions...)
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
					gtx.Execute(op.InvalidateCmd{})
					return layout.Dimensions{}
				}
				break
			}
		}
	}
	if a.modal.SearchReplace && a.modal.HasInput && gtx.Source.Focused(&a.modal.Input) {
		if event, ok := gtx.Event(
			key.Filter{Focus: &a.modal.Input, Name: key.NameReturn, Required: key.ModShift},
			key.Filter{Focus: &a.modal.Input, Name: key.NameEnter, Required: key.ModShift},
		); ok {
			if pressed, ok := event.(key.Event); ok && pressed.State == key.Press && a.modal.OnAction != nil {
				a.modal.OnAction("find-previous")
				if !a.modal.Visible {
					gtx.Execute(op.InvalidateCmd{})
					return layout.Dimensions{}
				}
			}
		}
	}
	if a.modal.HasInput {
		for ev, ok := a.modal.Input.Update(gtx); ok; ev, ok = a.modal.Input.Update(gtx) {
			if _, submit := ev.(widget.SubmitEvent); submit && a.modal.OnAction != nil {
				a.modal.OnAction(a.modal.submitAction(false))
				if !a.modal.Visible {
					gtx.Execute(op.InvalidateCmd{})
					return layout.Dimensions{}
				}
			}
		}
	}
	if a.modal.HasSecondInput {
		for ev, ok := a.modal.SecondInput.Update(gtx); ok; ev, ok = a.modal.SecondInput.Update(gtx) {
			if _, submit := ev.(widget.SubmitEvent); submit && a.modal.OnAction != nil {
				a.modal.OnAction(a.modal.submitAction(true))
				if !a.modal.Visible {
					gtx.Execute(op.InvalidateCmd{})
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
						gtx.Execute(op.InvalidateCmd{})
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
