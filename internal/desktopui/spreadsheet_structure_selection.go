package desktopui

import "glance/internal/document"

func spreadsheetStructureEditForSelection(kind document.StructureEditKind, sheet string, selection CellSelection) document.StructureEdit {
	index, count := selection.Active.Row+1, 1
	switch kind {
	case document.StructureInsertRows, document.StructureDeleteRows:
		if selection.Scope == selectionRows {
			bounds := selection.Bounds()
			index, count = bounds.StartRow, bounds.EndRow-bounds.StartRow+1
		}
	case document.StructureInsertColumns, document.StructureDeleteColumns:
		index = selection.Active.Column + 1
		if selection.Scope == selectionColumns {
			bounds := selection.Bounds()
			index, count = bounds.StartColumn, bounds.EndColumn-bounds.StartColumn+1
		}
	}
	return document.StructureEdit{Kind: kind, Sheet: sheet, Index: index, Count: count}
}

func spreadsheetStructureEditForToolbarAction(slot int, sheet string, selection CellSelection) (document.StructureEdit, bool) {
	if slot < 12 || slot > 15 {
		return document.StructureEdit{}, false
	}
	kind := [...]document.StructureEditKind{
		document.StructureInsertRows,
		document.StructureDeleteRows,
		document.StructureInsertColumns,
		document.StructureDeleteColumns,
	}[slot-12]
	return spreadsheetStructureEditForSelection(kind, sheet, selection), true
}
