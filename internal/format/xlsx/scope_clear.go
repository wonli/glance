package xlsx

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

var _ document.SpreadsheetRangeContentEditor = (*Workbook)(nil)

func (w *Workbook) ClearCellRange(sheet string, bounds document.CellRange) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	if err := validateRange(sheet, bounds, w.file.GetSheetList()); err != nil {
		return false, err
	}
	before, err := w.snapshotLocked()
	if err != nil {
		return false, err
	}
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
		return false, nil
	}
	if err := w.prepareScopedFormulaClear(sheet, data, bounds); err != nil {
		return false, err
	}
	changed := false
	clearedCells := map[string]bool{}
	for _, row := range data.children("row") {
		r := row.integer("r")
		if r < bounds.StartRow || r > bounds.EndRow {
			continue
		}
		for _, cell := range row.children("c") {
			col := cellColumn(cell.attrs["r"])
			if col < bounds.StartColumn || col > bounds.EndColumn {
				continue
			}
			cleared := false
			for _, name := range []string{"v", "f", "is"} {
				for _, n := range cell.children(name) {
					n.remove = true
					cleared = true
				}
			}
			if cleared {
				changed = true
				clearedCells[cell.attrs["r"]] = true
				cell.set("t", "n")
			}
		}
	}
	if !changed {
		return false, nil
	}
	if err := clearScopeCalculationChain(files, sheet, clearedCells); err != nil {
		return false, err
	}
	files[target] = root.bytes()
	updated, err := replaceScopePackage(before, files)
	if err != nil {
		return false, err
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

// Formula groups may cross a header selection. Validate arrays before touching
// any content, and detach surviving shared formulas before deleting their anchor.
func (w *Workbook) prepareScopedFormulaClear(sheet string, data *scopeNode, bounds document.CellRange) error {
	selected := func(cell *scopeNode) bool {
		col, row, err := excelize.CellNameToCoordinates(cell.attrs["r"])
		return err == nil && row >= bounds.StartRow && row <= bounds.EndRow && col >= bounds.StartColumn && col <= bounds.EndColumn
	}
	anchors := map[string]bool{}
	for _, row := range data.children("row") {
		for _, cell := range row.children("c") {
			f := cell.child("f")
			if f == nil {
				continue
			}
			if f.attrs["t"] == "shared" && f.attrs["ref"] != "" && selected(cell) {
				anchors[f.attrs["si"]] = true
			}
			if f.attrs["t"] != "array" && f.attrs["t"] != "dataTable" {
				continue
			}
			ref := strings.Split(strings.ReplaceAll(f.attrs["ref"], "$", ""), ":")
			if len(ref) == 1 {
				ref = append(ref, ref[0])
			}
			if len(ref) != 2 {
				return fmt.Errorf("cannot safely clear formula group at %s", cell.attrs["r"])
			}
			c1, r1, e1 := excelize.CellNameToCoordinates(ref[0])
			c2, r2, e2 := excelize.CellNameToCoordinates(ref[1])
			if e1 != nil || e2 != nil || c1 > c2 || r1 > r2 {
				return fmt.Errorf("cannot safely clear formula group at %s", cell.attrs["r"])
			}
			intersects := r1 <= bounds.EndRow && r2 >= bounds.StartRow && c1 <= bounds.EndColumn && c2 >= bounds.StartColumn
			contained := r1 >= bounds.StartRow && r2 <= bounds.EndRow && c1 >= bounds.StartColumn && c2 <= bounds.EndColumn
			if intersects && !contained {
				return fmt.Errorf("clear must include the entire %s formula range %s", f.attrs["t"], f.attrs["ref"])
			}
		}
	}
	if len(anchors) == 0 {
		return nil
	}
	for _, row := range data.children("row") {
		for _, cell := range row.children("c") {
			f := cell.child("f")
			if f == nil || f.attrs["t"] != "shared" || !anchors[f.attrs["si"]] || selected(cell) {
				continue
			}
			formula, err := w.file.GetCellFormula(sheet, cell.attrs["r"])
			if err != nil || formula == "" {
				return fmt.Errorf("cannot resolve shared formula at %s", cell.attrs["r"])
			}
			// Keep the original namespace prefix and any unrelated attributes.
			f.open = sharedFormulaAttrs.ReplaceAllString(f.open, "")
			if strings.HasSuffix(f.open, "/>") {
				f.open = strings.TrimSuffix(f.open, "/>") + ">"
			}
			name := f.open[1:strings.IndexAny(f.open, " \t\r\n>")]
			f.close = "</" + name + ">"
			var escaped strings.Builder
			if err := xml.EscapeText(&escaped, []byte(formula)); err != nil {
				return err
			}
			f.parts = []scopePart{{raw: escaped.String()}}
		}
	}
	return nil
}

var sharedFormulaAttrs = regexp.MustCompile(`\s(?:t|si|ref)\s*=\s*(?:"[^"]*"|'[^']*')`)

func cellColumn(ref string) int {
	col := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		col = col*26 + int(r-'A') + 1
	}
	return col
}
