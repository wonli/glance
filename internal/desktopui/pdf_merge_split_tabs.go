package desktopui

import "glance/internal/workspace"

func (a *App) openPDFMergeSplitToolTab() {
	if a.proofreadingModule {
		return
	}
	if !a.pdfMergeSplitToolPage {
		a.pdfMergeSplitReturnTabID = a.workspaceSelectedTabID()
		if a.pdfMergeSplitReturnTabID == "" || a.pdfMergeSplitReturnTabID == workspacePDFMergeSplitTabID {
			a.pdfMergeSplitReturnTabID = a.activeTabID
		}
		if a.pdfMergeSplitReturnTabID == "" {
			a.pdfMergeSplitReturnTabID = workspace.RecentTabID
		}
		a.pdfMergeSplitToolPage = true
	}
	a.activatePDFMergeSplitToolTab()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) pdfMergeSplitToolTabActive() bool {
	return !a.proofreadingModule && a.pdfMergeSplitToolPage && a.pdfMergeSplitTabActive
}

// closePDFMergeSplitToolTab removes the ephemeral workbench tab. Closing an
// inactive tab leaves the user's current workspace tab untouched.
func (a *App) closePDFMergeSplitToolTab(backToTools bool) {
	if !a.pdfMergeSplitToolPage {
		return
	}
	wasActive := a.pdfMergeSplitTabActive
	returnTabID := a.pdfMergeSplitReturnTabID
	a.pdfMergeSplitTabActive = false
	a.pdfMergeSplitToolPage = false
	a.cancelPDFMergeSplitJobForExit()

	generation := a.pdfMergeSplitTool.generation
	jobResults := a.pdfMergeSplitTool.jobResults
	a.pdfMergeSplitTool = newPDFMergeSplitToolState()
	a.pdfMergeSplitTool.generation = generation
	a.pdfMergeSplitTool.jobResults = jobResults
	a.pdfMergeSplitReturnTabID = ""
	a.workspaceTabsRevealedActiveID = ""
	a.workspaceTabsRevealedIndex = -1
	if !wasActive {
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if backToTools {
		a.activateRecent()
		a.homeSection = homeSectionTools
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if returnTabID != "" && returnTabID != workspace.RecentTabID {
		if _, exists := a.tabMeta[returnTabID]; exists || a.proofreadingTab(returnTabID) != nil {
			a.activateWorkspaceTab(returnTabID)
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
	}
	a.activateRecent()
	if a.window != nil {
		a.window.Invalidate()
	}
}
