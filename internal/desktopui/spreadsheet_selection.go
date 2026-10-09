package desktopui

import (
	"fmt"
	"glance/internal/document"
)

func (a *App) clearSelectionAsync() {
	state := a.documentSessionState
	if state == nil || !state.selectionSet {
		return
	}
	b := state.selection.Bounds()
	count := (b.EndRow - b.StartRow + 1) * (b.EndColumn - b.StartColumn + 1)
	if count > maxClipboardCellsUI {
		a.setStatus(fmt.Sprintf("剪切选区超过 %d 个单元格", maxClipboardCellsUI))
		return
	}
	edits := make([]document.CellEdit, 0, count)
	for r := b.StartRow; r <= b.EndRow; r++ {
		for c := b.StartColumn; c <= b.EndColumn; c++ {
			edits = append(edits, document.CellEdit{Row: r, Column: c, Value: ""})
		}
	}
	if err := a.applyBatchEdits(edits, "剪切"); err != nil {
		a.setStatus("剪切失败：" + err.Error())
	}
}
