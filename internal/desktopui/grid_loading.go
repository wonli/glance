package desktopui

import (
	"fmt"

	"glance/internal/document"
)

func (a *App) requestGridWindow(v *gridViewport, reader document.GridReader, sheet string, r document.CellRange) {
	if r.StartRow <= 0 || r.StartColumn <= 0 {
		return
	}
	v.Loading = true
	v.RequestedRange = r
	v.RequestedSheet = sheet
	generation := a.documentGeneration
	rows, cols := r.EndRow-r.StartRow+1, r.EndColumn-r.StartColumn+1
	a.launchManagedBackground(func() {
		unlock := lockSessionAdapterRead(a.documentSessionState)
		w, err := reader.Window(sheet, r.StartRow, r.StartColumn, rows, cols)
		unlock()
		select {
		case a.gridLoadResults <- gridLoadResult{generation: generation, viewport: v, sheet: sheet, rangeOf: r, window: w, err: err}:
			a.window.Invalidate()
		case <-a.shutdown:
		}
	})
	a.setStatus(fmt.Sprintf("正在读取工作表「%s」的可见单元格…", sheet))
	a.window.Invalidate()
}

func (a *App) pollGridLoad(v *gridViewport) {
	for {
		select {
		case result := <-a.gridLoadResults:
			if result.viewport == nil || result.viewport.RequestedSheet != result.sheet || result.viewport.RequestedRange != result.rangeOf {
				continue
			}
			result.viewport.Loading = false
			if result.viewport != v {
				continue
			}
			if result.generation != a.documentGeneration || result.sheet != a.sheet {
				a.window.Invalidate()
				continue
			}
			if result.err != nil {
				v.WindowError = result.err
				v.FailedRange = result.rangeOf
				v.FailedSheet = result.sheet
				a.setStatus(gridWindowError(result.sheet, result.err))
				a.window.Invalidate()
				continue
			}
			_ = v.applyWindow(result.sheet, result.rangeOf, result.window)
			a.applyValueFilterWindow(a.documentSessionState, v)
			a.setStatus("就绪")
			a.window.Invalidate()
		default:
			return
		}
	}
}
