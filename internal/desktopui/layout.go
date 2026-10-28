package desktopui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/workspace"
)

var (
	pageBackground           = color.NRGBA{R: 247, G: 248, B: 250, A: 255}
	chromeSurface            = color.NRGBA{R: 241, G: 243, B: 245, A: 255}
	workspaceTabStripSurface = color.NRGBA{R: 238, G: 241, B: 244, A: 255}
	tabSurface               = color.NRGBA{R: 238, G: 241, B: 244, A: 255}
	contentSurface           = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	gridHeaderFill           = color.NRGBA{R: 242, G: 244, B: 246, A: 255}
	gridLine                 = color.NRGBA{R: 216, G: 221, B: 227, A: 255}
	selectedFill             = color.NRGBA{R: 231, G: 240, B: 251, A: 255}
	mutedText                = color.NRGBA{R: 104, G: 113, B: 123, A: 255}
)

func (a *App) layout(gtx layout.Context) layout.Dimensions {
	a.ensureDesktopRuntime()
	a.processPDFMergeSplitToolJobResults()
	if a.pdfMergeSplitToolPage && a.activeTabID != workspace.RecentTabID {
		a.pdfMergeSplitToolPage = false
		a.cancelPDFMergeSplitJobForExit()
	}
	a.processProofreadingWorkbench()
	a.processProofreadingTabs(gtx)
	for _, tab := range a.proofreadingTabs {
		tab.editor.processProofreadingEditorEvents(gtx)
	}
	a.processProofreadingNavigation(gtx)
	if a.proofreadingWorkbench.active {
		if a.proofreading != nil {
			a.processProofreadingResults()
			a.acceptProofreadingConfirmation()
		}
		a.handleOverlayEscape(gtx)
		return a.layoutRootWithOverlays(gtx, a.layoutProofreadingWorkbench)
	}
	if a.proofreading != nil {
		a.handleOverlayEscape(gtx)
		a.processAsyncResults()
		a.processProofreadingResults()
		gtx.Constraints.Min = gtx.Constraints.Max
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions { return fill(gtx, pageBackground) },
			func(gtx layout.Context) layout.Dimensions {
				return a.layoutRootWithOverlays(gtx, a.layoutProofreadingWorkspace)
			},
		)
	}
	a.handleOverlayEscape(gtx)
	if a.selectedProofreadingTabID == "" && a.markdownDoc != nil && !a.modal.Visible {
		a.updateMarkdownInput(gtx)
	}
	a.processAsyncResults()
	a.processFontPickerEvents(gtx)
	a.reconcileWorkspaceTabs()
	syncWindowViewFromSharedSession(gtx, a.documentSessionState)
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return fill(gtx, pageBackground) },
		func(gtx layout.Context) layout.Dimensions {
			return a.layoutRootWithOverlays(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
					layout.Rigid(a.layoutWorkspaceTabs),
					layout.Flexed(1, a.layoutWorkspaceBody),
				)
			})
		},
	)
}

func (a *App) layoutRootWithOverlays(gtx layout.Context, content layout.Widget) layout.Dimensions {
	if tab := a.proofreadingTab(a.selectedProofreadingTabID); tab != nil && tab.editor != nil {
		editor := tab.editor
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(content),
			layout.Stacked(editor.layoutWordSymbolPicker),
			layout.Expanded(editor.layoutFeedback),
			layout.Expanded(editor.layoutModalDialog),
			layout.Expanded(editor.layoutAgentConfirmationModal),
			layout.Stacked(a.layoutWordSymbolPicker),
			layout.Expanded(a.layoutFeedback),
			layout.Expanded(a.layoutModalDialog),
			layout.Expanded(a.layoutAgentConfirmationModal),
		)
	}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(content),
		layout.Stacked(a.layoutWordSymbolPicker),
		layout.Expanded(a.layoutFeedback),
		layout.Expanded(a.layoutModalDialog),
		layout.Expanded(a.layoutAgentConfirmationModal),
	)
}

// layoutWorkspaceBody keeps the document as the only content directly below
// the tabs. Transient actions float over it and do not change its geometry.
func (a *App) layoutWorkspaceBody(gtx layout.Context) layout.Dimensions {
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
				layout.Flexed(1, a.layoutDocument),
				layout.Rigid(a.layoutEditor),
			)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !a.showNewMenu {
				return layout.Dimensions{}
			}
			return a.layoutCreateMenuPopover(gtx)
		}),
		layout.Stacked(a.layoutWordVersionHistory),
	)
}

func (a *App) visibleStatus() string {
	if a.workspaceWriteErrorRef.ID != "" {
		return a.text(a.workspaceWriteErrorRef)
	}
	return a.statusText()
}

func (a *App) layoutDocument(gtx layout.Context) layout.Dimensions {
	if tab := a.proofreadingTab(a.selectedProofreadingTabID); tab != nil {
		return a.layoutProofreadingTab(gtx, tab)
	}
	if a.proofreadingModule && a.activeTabID == "recent" {
		return a.layoutProofreadingWorkbench(gtx)
	}
	if a.activeTabID == "recent" || (a.book == nil && a.word == nil && a.pptxDoc == nil && a.jsonDoc == nil && a.markdownDoc == nil && a.pdfDoc == nil) {
		return a.layoutRecent(gtx)
	}
	switch {
	case a.book != nil:
		return a.layoutWorkbook(gtx)
	case a.word != nil:
		return a.layoutWord(gtx)
	case a.pptxDoc != nil:
		return a.layoutPresentationDocument(gtx)
	case a.pdfDoc != nil:
		return a.layoutPDFDocument(gtx)
	case a.markdownDoc != nil:
		return a.layoutMarkdownDocument(gtx)
	case a.jsonDoc != nil:
		return a.layoutJSON(gtx)
	default:
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return material.H6(a.theme, "Glance Office").Layout(gtx)
		})
	}
}

func (a *App) layoutWorkbook(gtx layout.Context) layout.Dimensions {
	a.handleOfficeShortcuts(gtx)
	if _, ok := a.book.(document.GridReader); !ok {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return material.H6(a.theme, a.textID("workspace.grid.read_capability_unavailable")).Layout(gtx)
		})
	}
	a.handleSpreadsheetToolbarEvents(gtx)
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(a.layoutSpreadsheetToolbar),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return a.layoutAgentSidebarWorkspace(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(a.layoutGridControls),
							layout.Flexed(1, a.layoutVirtualGrid),
							layout.Rigid(a.layoutSheetTabs),
						)
					})
				}),
				layout.Rigid(a.layoutSpreadsheetStatusBar),
			)
		}),
		layout.Stacked(a.layoutSpreadsheetToolbarPickerOverlay),
		layout.Stacked(a.layoutOfficeRibbonDropdown),
	)
}

func (a *App) layoutWord(gtx layout.Context) layout.Dimensions {
	a.handleOfficeShortcuts(gtx)
	a.handleWordToolbarEvents(gtx)
	a.handleWordClipboardEvents(gtx)
	dimensions := a.layoutWordFlow(gtx)
	a.processWordFormatPainterTargetIntent()
	return dimensions
}

func (a *App) layoutEditor(gtx layout.Context) layout.Dimensions {
	if a.jsonDoc != nil || a.pptxDoc != nil || a.book != nil {
		return layout.Dimensions{}
	}
	if a.word != nil {
		return layout.Dimensions{}
	}
	return layout.Dimensions{}
}
