package desktopui

import (
	"fmt"
	"glance/internal/i18n"

	"glance/internal/document"
)

func (a *App) requestGridWindow(v *gridViewport, reader document.GridReader, sheet string, windows gridPaneRanges) {
	if v == nil || windows.empty() {
		return
	}
	v.Loading = true
	v.RequestSequence++
	v.RequestedID = v.RequestSequence
	requestID := v.RequestedID
	v.RequestedWindows = windows
	v.RequestedRange = windows.Body
	v.RequestedSheet = sheet
	generation := a.documentGeneration
	state := a.documentSessionState
	read := func(r document.CellRange) (document.SheetWindow, error) {
		if r.StartRow <= 0 || r.StartColumn <= 0 {
			return document.SheetWindow{}, nil
		}
		return reader.Window(sheet, r.StartRow, r.StartColumn, r.EndRow-r.StartRow+1, r.EndColumn-r.StartColumn+1)
	}
	if !a.launchManagedBackground(func() {
		unlock := lockSessionAdapterRead(state)
		body, err := read(windows.Body)
		var top, left, corner document.SheetWindow
		if err == nil {
			top, err = read(windows.Top)
		}
		if err == nil {
			left, err = read(windows.Left)
		}
		if err == nil {
			corner, err = read(windows.Corner)
		}
		panes := body.Panes
		if windows.Body.StartRow <= 0 {
			switch {
			case windows.Top.StartRow > 0:
				panes = top.Panes
			case windows.Left.StartRow > 0:
				panes = left.Panes
			case windows.Corner.StartRow > 0:
				panes = corner.Panes
			}
		}
		unlock()
		select {
		case a.gridLoadResults <- gridLoadResult{generation: generation, requestID: requestID, viewport: v, sheet: sheet, rangeOf: windows.Body, windows: windows, window: body, top: top, left: left, corner: corner, panes: panes, err: err}:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	}) {
		v.Loading = false
		v.WindowError = fmt.Errorf("worksheet window load was cancelled")
		v.FailedWindows = windows
		v.FailedSheet = sheet
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.grid.window.load_failed", Args: i18n.Args{"Sheet": sheet}}, v.WindowError, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	a.setStatusMessage(i18n.Message{ID: "spreadsheet.grid.window.loading", Args: i18n.Args{"Sheet": sheet}}, feedbackInfo, notificationSuppressed)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) pollGridLoad(v *gridViewport) {
	for {
		select {
		case result := <-a.gridLoadResults:
			if result.viewport == nil {
				continue
			}
			if result.viewport.RequestedID != result.requestID || result.viewport.RequestedSheet != result.sheet || result.viewport.RequestedWindows != result.windows {
				continue
			}
			result.viewport.Loading = false
			if result.viewport != v {
				continue
			}
			if result.generation != a.documentGeneration || result.sheet != a.sheet {
				if a.window != nil {
					a.window.Invalidate()
				}
				continue
			}
			if result.err != nil {
				v.WindowError = result.err
				v.FailedRange = result.rangeOf
				v.FailedSheet = result.sheet
				v.FailedWindows = result.windows
				a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.grid.window.load_failed", Args: i18n.Args{"Sheet": result.sheet}}, result.err, notificationToast)
				if a.window != nil {
					a.window.Invalidate()
				}
				continue
			}
			if err := v.applyLoadResult(result); err != nil {
				v.WindowError = err
				v.FailedWindows = result.windows
				v.FailedSheet = result.sheet
				a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.grid.window.load_failed", Args: i18n.Args{"Sheet": result.sheet}}, err, notificationToast)
				if a.window != nil {
					a.window.Invalidate()
				}
				continue
			}
			a.applyValueFilterWindow(a.documentSessionState, v)
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.ready_b796f2"}, feedbackInfo, notificationSuppressed)
			if a.window != nil {
				a.window.Invalidate()
			}
		default:
			return
		}
	}
}

func (a *App) drainGridLoadResults() {
	if a == nil || a.gridLoadResults == nil {
		return
	}
	for {
		select {
		case <-a.gridLoadResults:
		default:
			return
		}
	}
}
