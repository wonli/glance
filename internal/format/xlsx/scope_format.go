package xlsx

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

func wholeRowRange(b document.CellRange) bool {
	return b.StartColumn == 1 && b.EndColumn == maxXLSXColumns
}
func wholeColumnRange(b document.CellRange) bool { return b.StartRow == 1 && b.EndRow == maxXLSXRows }

// setScopedFormatsLocked uses row and column defaults, never iterating the
// worksheet's logical cell rectangle. Cells are visited only if already present
// or needed to represent an intersection of distinct inherited defaults.
func (w *Workbook) setScopedFormatsLocked(sheet string, b document.CellRange, patch document.CellFormatPatch) (applied bool, operationErr error) {
	if patch.BorderPreset != nil && *patch.BorderPreset == "outside" {
		return false, fmt.Errorf("outside borders require a bounded cell selection")
	}
	before, err := w.snapshotLocked()
	if err != nil {
		return false, err
	}
	// NewStyle changes the live style table before worksheet validation has
	// finished. A failed or no-op patch must leave the complete package intact.
	defer func() {
		if !applied {
			if err := w.restoreSnapshotLocked(before); err != nil {
				operationErr = errors.Join(operationErr, fmt.Errorf("restore failed scoped format: %w", err))
			}
		}
	}()
	files, target, err := scopePackage(before, sheet)
	if err != nil {
		return false, err
	}
	root, err := parseScopeXML(files[target])
	if err != nil {
		return false, err
	}
	ws := root.child("worksheet")
	data := ws.child("sheetData")
	if data == nil {
		data = newScopeChild(ws, "sheetData")
		ws.parts = append(ws.parts, scopePart{node: data})
	}
	cols := ws.child("cols")
	oldCols := make([]int, maxXLSXColumns+1)
	colNodes := make([]*scopeNode, maxXLSXColumns+1)
	if cols != nil {
		for _, c := range cols.children("col") {
			for i := max(1, c.integer("min")); i <= min(maxXLSXColumns, c.integer("max")); i++ {
				oldCols[i] = c.integer("style")
				colNodes[i] = c
			}
		}
	}
	rows := map[int]*scopeNode{}
	oldRows := map[int]int{}
	cells := map[int]map[int]*scopeNode{}
	for _, r := range data.children("row") {
		row := r.integer("r")
		rows[row] = r
		oldRows[row] = r.integer("s")
		cells[row] = map[int]*scopeNode{}
		for _, c := range r.children("c") {
			col, _, e := excelize.CellNameToCoordinates(c.attrs["r"])
			if e == nil {
				cells[row][col] = c
			}
		}
	}
	styledColumns := []int{}
	for col := 1; col <= maxXLSXColumns; col++ {
		if oldCols[col] != 0 {
			styledColumns = append(styledColumns, col)
		}
	}
	// A nonzero identity style suppresses inherited row/column defaults.
	// Style zero means "inherit" to Excelize and cannot clear an intersection.
	resetID := 0
	if patch.ClearFormat {
		base, e := w.file.GetStyle(0)
		if e != nil {
			return false, e
		}
		reset := cloneExcelizeStyle(base)
		reset.Alignment = &excelize.Alignment{Horizontal: "general"}
		resetID, e = w.file.NewStyle(&reset)
		if e != nil {
			return false, e
		}
	}
	cache := map[int]int{}
	changed := false
	update := func(id int) (int, error) {
		if v, ok := cache[id]; ok {
			return v, nil
		}
		if patch.ClearFormat {
			if id == 0 {
				return 0, nil
			}
			cache[id] = resetID
			return resetID, nil
		}
		base, e := w.file.GetStyle(id)
		if e != nil {
			return 0, e
		}
		updated := cloneExcelizeStyle(base)
		applyCellFormatPatch(&updated, patch, b.StartRow, b.StartColumn, b)
		if reflect.DeepEqual(*base, updated) {
			cache[id] = id
			return id, nil
		}
		v, e := w.file.NewStyle(&updated)
		cache[id] = v
		return v, e
	}
	setStyle := func(n *scopeNode, attr string, id int) {
		if n.integer(attr) != id {
			n.set(attr, strconv.Itoa(id))
			changed = true
		}
	}
	all := wholeRowRange(b) && wholeColumnRange(b)
	// Preserve the original effective style before any default is updated.
	effective := func(row, col int) int {
		if c := cells[row][col]; c != nil && c.integer("s") != 0 {
			return c.integer("s")
		}
		if oldRows[row] != 0 {
			return oldRows[row]
		}
		return oldCols[col]
	}
	ensureRow := func(row int) *scopeNode {
		r := rows[row]
		if r == nil {
			r = newScopeChild(data, "row")
			r.set("r", strconv.Itoa(row))
			rows[row] = r
			cells[row] = map[int]*scopeNode{}
			insertScopeChild(data, r, func(n *scopeNode) int { return n.integer("r") })
		}
		return r
	}
	intersections := 0
	ensureCell := func(row, col, id int) error {
		if cells[row][col] != nil {
			return nil
		}
		intersections++
		if intersections > 1_000_000 {
			return fmt.Errorf("format default intersections exceed 1000000 cells")
		}
		r := ensureRow(row)
		c := newScopeChild(r, "c")
		c.set("r", CellName(col, row))
		c.set("s", strconv.Itoa(id))
		insertScopeChild(r, c, func(n *scopeNode) int { return cellColumn(n.attrs["r"]) })
		cells[row][col] = c
		changed = true
		return nil
	}
	// Patch existing explicit cells independently to retain their other properties.
	for row, cs := range cells {
		if row < b.StartRow || row > b.EndRow {
			continue
		}
		for col, c := range cs {
			if col < b.StartColumn || col > b.EndColumn {
				continue
			}
			id, e := update(effective(row, col))
			if patch.ClearFormat && (oldRows[row] != 0 || oldCols[col] != 0) {
				id = resetID
			}
			if e != nil {
				return false, e
			}
			setStyle(c, "s", id)
		}
	}
	if wholeRowRange(b) {
		for row := b.StartRow; row <= b.EndRow && !all; row++ {
			id, e := update(oldRows[row])
			if e != nil {
				return false, e
			}
			if patch.ClearFormat && len(styledColumns) > 0 {
				id = resetID
			}
			r := rows[row]
			if id != oldRows[row] {
				r = ensureRow(row)
				setStyle(r, "s", id)
				r.set("customFormat", "1")
			}
			if oldRows[row] == 0 {
				for _, col := range styledColumns {
					v, e := update(oldCols[col])
					if e != nil {
						return false, e
					}
					if v != id {
						if e = ensureCell(row, col, v); e != nil {
							return false, e
						}
					}
				}
			}
		}
	}
	if wholeColumnRange(b) {
		updatedCols := append([]int(nil), oldCols...)
		for col := b.StartColumn; col <= b.EndColumn; col++ {
			id, e := update(oldCols[col])
			if e != nil {
				return false, e
			}
			if id != oldCols[col] {
				changed = true
			}
			updatedCols[col] = id
		}
		for row, id := range oldRows {
			if id == 0 {
				continue
			}
			v, e := update(id)
			if e != nil {
				return false, e
			}
			if all {
				setStyle(rows[row], "s", v)
			} else if v != id {
				for col := b.StartColumn; col <= b.EndColumn; col++ {
					if e = ensureCell(row, col, v); e != nil {
						return false, e
					}
				}
			}
		}
		// Rebuild only column declarations, retaining every geometry attribute.
		if cols == nil {
			cols = newScopeChild(ws, "cols")
			idx := 0
			for i, p := range ws.parts {
				if p.node == data {
					idx = i
					break
				}
			}
			ws.parts = append(ws.parts, nilPart())
			copy(ws.parts[idx+1:], ws.parts[idx:])
			ws.parts[idx] = scopePart{node: cols}
		}
		cols.parts = nil
		for col := 1; col <= maxXLSXColumns; {
			end := col
			for end < maxXLSXColumns && updatedCols[end+1] == updatedCols[col] && colNodes[end+1] == colNodes[col] {
				end++
			}
			if colNodes[col] != nil || updatedCols[col] != 0 {
				var n *scopeNode
				if colNodes[col] != nil {
					copyNode := *colNodes[col]
					copyNode.attrs = map[string]string{}
					for k, v := range colNodes[col].attrs {
						copyNode.attrs[k] = v
					}
					n = &copyNode
				} else {
					n = newScopeChild(cols, "col")
				}
				n.set("min", strconv.Itoa(col))
				n.set("max", strconv.Itoa(end))
				n.set("style", strconv.Itoa(updatedCols[col]))
				cols.parts = append(cols.parts, scopePart{node: n})
			}
			col = end + 1
		}
	}
	if !changed {
		return false, nil
	}
	sheetXML := root.bytes()
	afterStyles, err := w.snapshotLocked()
	if err != nil {
		return false, err
	}
	files, _, err = scopePackage(afterStyles, sheet)
	if err != nil {
		return false, err
	}
	files[target] = sheetXML
	updated, err := replaceScopePackage(afterStyles, files)
	if err != nil {
		return false, err
	}
	if bytes.Equal(before, updated) {
		return false, nil
	}
	if err = w.restoreSnapshotLocked(updated); err != nil {
		return false, err
	}
	if err = w.recordHistoryLocked(before); err != nil {
		w.restoreSnapshotLocked(before)
		return false, err
	}
	w.dirty = true
	return true, nil
}
func nilPart() scopePart { return scopePart{} }
