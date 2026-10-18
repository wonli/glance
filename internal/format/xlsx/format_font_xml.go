package xlsx

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

type rawXLSXFontStyles struct {
	file       *excelize.File
	root       *scopeNode
	fonts      *scopeNode
	cellXfs    *scopeNode
	fontIDs    map[string]int
	cellXFIDs  map[string]int
	styleCache map[int]int
}

func newRawXLSXFontStyles(file *excelize.File, data []byte) (*rawXLSXFontStyles, error) {
	root, err := parseScopeXML(data)
	if err != nil {
		return nil, fmt.Errorf("parse workbook styles: %w", err)
	}
	styleSheet := root.child("styleSheet")
	if styleSheet == nil {
		return nil, fmt.Errorf("workbook styles are missing styleSheet")
	}
	fonts, cellXfs := styleSheet.child("fonts"), styleSheet.child("cellXfs")
	if fonts == nil || cellXfs == nil {
		return nil, fmt.Errorf("workbook styles are missing fonts or cellXfs")
	}
	editor := &rawXLSXFontStyles{
		file:       file,
		root:       root,
		fonts:      fonts,
		cellXfs:    cellXfs,
		fontIDs:    make(map[string]int),
		cellXFIDs:  make(map[string]int),
		styleCache: make(map[int]int),
	}
	for id, font := range fonts.children("font") {
		editor.fontIDs[string(font.bytes())] = id
	}
	for id, xf := range cellXfs.children("xf") {
		editor.cellXFIDs[string(xf.bytes())] = id
	}
	return editor, nil
}

func (s *rawXLSXFontStyles) bytes() []byte { return s.root.bytes() }

func (s *rawXLSXFontStyles) update(styleID int, patch document.CellFormatPatch) (int, error) {
	if id, ok := s.styleCache[styleID]; ok {
		return id, nil
	}
	base, err := s.file.GetStyle(styleID)
	if err != nil {
		return 0, err
	}
	updated := cloneExcelizeStyle(base)
	applyCellFormatPatch(&updated, patch, 1, 1, document.CellRange{})
	if reflect.DeepEqual(base.Font, updated.Font) {
		s.styleCache[styleID] = styleID
		return styleID, nil
	}

	xfs := s.cellXfs.children("xf")
	if styleID < 0 || styleID >= len(xfs) {
		return 0, fmt.Errorf("cell style %d is outside the style table", styleID)
	}
	baseXF := xfs[styleID]
	fontID := baseXF.integer("fontId")
	fonts := s.fonts.children("font")
	if fontID < 0 || fontID >= len(fonts) {
		return 0, fmt.Errorf("cell style %d references missing font %d", styleID, fontID)
	}
	font := cloneScopeNode(fonts[fontID])
	if err := applyRawFontPatch(font, patch, updated.Font); err != nil {
		return 0, err
	}
	fontKey := string(font.bytes())
	updatedFontID, ok := s.fontIDs[fontKey]
	if !ok {
		updatedFontID = len(fonts)
		s.fonts.set("count", strconv.Itoa(updatedFontID+1))
		appendScopeChild(s.fonts, font)
		s.fontIDs[fontKey] = updatedFontID
	}

	updatedXF := cloneScopeNode(baseXF)
	updatedXF.set("fontId", strconv.Itoa(updatedFontID))
	updatedXF.set("applyFont", "1")
	xfKey := string(updatedXF.bytes())
	updatedStyleID, ok := s.cellXFIDs[xfKey]
	if !ok {
		if len(xfs) >= excelize.MaxCellStyles {
			return 0, excelize.ErrCellStyles
		}
		updatedStyleID = len(xfs)
		s.cellXfs.set("count", strconv.Itoa(updatedStyleID+1))
		appendScopeChild(s.cellXfs, updatedXF)
		s.cellXFIDs[xfKey] = updatedStyleID
	}
	s.styleCache[styleID] = updatedStyleID
	return updatedStyleID, nil
}

func applyRawFontPatch(font *scopeNode, patch document.CellFormatPatch, updated *excelize.Font) error {
	if font == nil || updated == nil {
		return fmt.Errorf("cell font is unavailable")
	}
	if patch.Bold != nil {
		setRawFontFlag(font, "b", updated.Bold)
	}
	if patch.Italic != nil {
		setRawFontFlag(font, "i", updated.Italic)
	}
	if patch.Underline != nil {
		if updated.Underline == "" {
			if node := font.child("u"); node != nil {
				node.remove = true
			}
		} else {
			node := ensureRawFontChild(font, "u")
			node.set("val", updated.Underline)
		}
	}
	if patch.FontFamily != nil {
		node := ensureRawFontChild(font, "name")
		node.set("val", updated.Family)
	}
	if patch.FontSize != nil || patch.FontSizeStep != nil {
		node := ensureRawFontChild(font, "sz")
		node.set("val", strconv.FormatFloat(updated.Size, 'f', -1, 64))
	}
	if patch.FontColor != nil {
		node := font.child("color")
		if *patch.FontColor == "" {
			if node != nil {
				node.remove = true
			}
		} else {
			color := normalizeRGBColor(*patch.FontColor)
			if color == "" {
				return fmt.Errorf("font color must be a six-digit RGB color")
			}
			if node == nil {
				node = ensureRawFontChild(font, "color")
			}
			for _, attr := range []string{"rgb", "indexed", "theme", "tint", "auto"} {
				node.removeAttr(attr)
			}
			node.set("rgb", "FF"+strings.TrimPrefix(color, "#"))
		}
	}
	return nil
}

func setRawFontFlag(font *scopeNode, name string, enabled bool) {
	node := font.child(name)
	if !enabled && node == nil {
		return
	}
	if node == nil {
		node = ensureRawFontChild(font, name)
	}
	if enabled {
		node.set("val", "1")
	} else {
		node.set("val", "0")
	}
}

func ensureRawFontChild(font *scopeNode, name string) *scopeNode {
	if node := font.child(name); node != nil {
		return node
	}
	node := newScopeChild(font, name)
	ranks := map[string]int{
		"name": 0, "charset": 1, "family": 2, "b": 3, "i": 4, "strike": 5,
		"outline": 6, "shadow": 7, "condense": 8, "extend": 9, "color": 10,
		"sz": 11, "u": 12, "vertAlign": 13, "scheme": 14,
	}
	targetRank, known := ranks[name]
	index := len(font.parts)
	if known {
		for i, part := range font.parts {
			if part.node == nil {
				continue
			}
			if rank, childKnown := ranks[part.node.name]; childKnown && rank > targetRank {
				index = i
				break
			}
		}
	}
	font.parts = append(font.parts, scopePart{})
	copy(font.parts[index+1:], font.parts[index:])
	font.parts[index] = scopePart{node: node}
	return node
}

func fontOnlyFormatPatch(patch document.CellFormatPatch) bool {
	hasFontChange := patch.Bold != nil || patch.Italic != nil || patch.Underline != nil || patch.FontColor != nil || patch.FontSizeStep != nil || patch.FontSize != nil || patch.FontFamily != nil
	return hasFontChange && !patch.ClearFormat && patch.FillColor == nil && patch.HorizontalAlignment == nil && patch.VerticalAlignment == nil && patch.NumberFormat == nil && patch.DecimalPlacesDelta == nil && patch.BorderPreset == nil && patch.WrapText == nil
}

func (w *Workbook) setCellFontFormatsLocked(sheet string, bounds document.CellRange, patch document.CellFormatPatch) (bool, error) {
	before, err := w.snapshotLocked()
	if err != nil {
		return false, err
	}
	files, target, err := scopePackage(before, sheet)
	if err != nil {
		return false, err
	}
	styles, err := newRawXLSXFontStyles(w.file, files["xl/styles.xml"])
	if err != nil {
		return false, err
	}
	root, err := parseScopeXML(files[target])
	if err != nil {
		return false, err
	}
	ws := root.child("worksheet")
	if ws == nil {
		return false, fmt.Errorf("worksheet %q is missing worksheet root", sheet)
	}
	data := ws.child("sheetData")
	if data == nil {
		data = newScopeChild(ws, "sheetData")
		ws.parts = append(ws.parts, scopePart{node: data})
	}
	rows := map[int]*scopeNode{}
	cells := map[int]map[int]*scopeNode{}
	for _, rowNode := range data.children("row") {
		row := rowNode.integer("r")
		rows[row] = rowNode
		cells[row] = map[int]*scopeNode{}
		for _, cellNode := range rowNode.children("c") {
			column, _, coordinateErr := excelize.CellNameToCoordinates(cellNode.attrs["r"])
			if coordinateErr == nil {
				cells[row][column] = cellNode
			}
		}
	}
	ensureRow := func(row int) *scopeNode {
		node := rows[row]
		if node == nil {
			node = newScopeChild(data, "row")
			node.set("r", strconv.Itoa(row))
			rows[row] = node
			cells[row] = map[int]*scopeNode{}
			insertScopeChild(data, node, func(n *scopeNode) int { return n.integer("r") })
		}
		return node
	}
	changed := false
	for row := bounds.StartRow; row <= bounds.EndRow; row++ {
		for column := bounds.StartColumn; column <= bounds.EndColumn; column++ {
			cell := CellName(column, row)
			beforeID, styleErr := w.file.GetCellStyle(sheet, cell)
			if styleErr != nil {
				return false, fmt.Errorf("read style for %s: %w", cell, styleErr)
			}
			afterID, updateErr := styles.update(beforeID, patch)
			if updateErr != nil {
				return false, fmt.Errorf("create style for %s: %w", cell, updateErr)
			}
			if afterID == beforeID {
				continue
			}
			cellNode := cells[row][column]
			if cellNode == nil {
				rowNode := ensureRow(row)
				cellNode = newScopeChild(rowNode, "c")
				cellNode.set("r", cell)
				insertScopeChild(rowNode, cellNode, func(n *scopeNode) int { return cellColumn(n.attrs["r"]) })
				cells[row][column] = cellNode
			}
			cellNode.set("s", strconv.Itoa(afterID))
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	files[target] = root.bytes()
	files["xl/styles.xml"] = styles.bytes()
	updated, err := replaceScopePackage(before, files)
	if err != nil {
		return false, err
	}
	if err = w.restoreSnapshotLocked(updated); err != nil {
		return false, err
	}
	if err = w.recordHistoryWithAfterLocked(before, updated); err != nil {
		_ = w.restoreSnapshotLocked(before)
		return false, err
	}
	w.dirty = true
	return true, nil
}
