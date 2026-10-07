package desktopui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

type spreadsheetFormatResult struct {
	state       *documentSessionState
	token       sessionOperationToken
	revision    uint64
	generation  uint64
	sheet       string
	window      *gridViewport
	err         error
	history     bool
	applied     bool
	undo        bool
	operation   string
	structure   *document.StructureEdit
	sort        *document.SortRequest
	filter      *ValueFilter
	clearFilter bool
}

func (a *App) applyHistory(undo bool) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	history, ok := state.book.(document.SpreadsheetHistory)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	go func() {
		unlock := lockSessionAdapterWrite(state)
		var applied bool
		var err error
		if undo {
			applied, err = history.Undo()
		} else {
			applied, err = history.Redo()
		}
		unlock()
		a.completeSessionOperation(state, token, err, applied)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, history: true, applied: applied, undo: undo}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) applyCellFormat(patch document.CellFormatPatch) {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet || state.formatRunning.Load() {
		return
	}
	formatter, ok := state.book.(document.SpreadsheetFormatter)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	selection := state.selection.Bounds()
	sheet, generation, window := state.sheet, a.documentGeneration, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := formatter.SetCellFormats(sheet, selection, patch)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) applyBatchEdits(edits []document.CellEdit, operation string) error {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return fmt.Errorf("表格正在处理其他操作")
	}
	editor, ok := state.book.(document.SpreadsheetBatchEditor)
	if !ok {
		return fmt.Errorf("当前表格不支持批量编辑")
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := editor.SetCells(sheet, edits)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
	return nil
}

func (a *App) applyStructure(edit document.StructureEdit) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetStructureEditor)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := editor.EditStructure(edit)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "结构操作", structure: &edit}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) applySort(request document.SortRequest) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	sorter, ok := state.book.(document.SpreadsheetSorter)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := sorter.SortRange(request)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "排序", sort: &request}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) toggleValueFilter() {
	state := a.documentSessionState
	if state == nil || state.formatRunning.Load() || state.book == nil {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
		return
	}
	if !state.selectionSet {
		return
	}
	bounds := state.selection.Bounds()
	pos := state.selection.Active
	reader, ok := state.book.(document.SpreadsheetRangeReader)
	if !ok || pos.Row+1 <= bounds.StartRow {
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	column := pos.Column + 1
	go func() {
		unlock := lockSessionAdapterRead(state)
		cells, err := reader.ReadRange(sheet, bounds)
		var filter *ValueFilter
		if err == nil {
			targetRow, targetCol := pos.Row+1-bounds.StartRow, column-bounds.StartColumn
			if targetRow < 0 || targetRow >= len(cells) || targetCol < 0 || targetCol >= len(cells[targetRow]) {
				err = fmt.Errorf("筛选列不在选择范围内")
			} else {
				value := cells[targetRow][targetCol].Value
				visible := filterMatchingRows(cells, bounds.StartRow, targetCol, value)
				filter = &ValueFilter{Sheet: sheet, Bounds: bounds, Column: column, Value: value, VisibleRows: visible}
				if grid, ok := state.book.(document.GridReader); ok {
					rows := bounds.EndRow - bounds.StartRow + 1
					if full, readErr := grid.Window(sheet, bounds.StartRow, bounds.StartColumn, rows, 1); readErr == nil {
						filter.RowHeightsPoints = full.RowHeightsPoints
					}
				}
			}
		}
		unlock()
		a.completeSessionOperation(state, token, err, false)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "筛选", filter: filter}:
		case <-a.shutdown:
		}
	}()
}

func filterMatchingRows(cells [][]document.Cell, firstRow, column int, value string) map[int]bool {
	visible := make(map[int]bool)
	for offset := 1; offset < len(cells); offset++ {
		if column >= 0 && column < len(cells[offset]) && cells[offset][column].Value == value {
			visible[firstRow+offset] = true
		}
	}
	return visible
}

func (a *App) clearValueFilter(state *documentSessionState) {
	filter := state.valueFilter
	if filter == nil {
		return
	}
	if state.virtualGrid != nil {
		for row, size := range filter.OriginalHeights {
			_ = state.virtualGrid.Rows.SetSize(row, size)
		}
		state.virtualGrid.invalidateWindow()
	}
	state.valueFilter = nil
	if state == a.documentSessionState && a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) applyValueFilterWindow(state *documentSessionState, v *gridViewport) {
	f := state.valueFilter
	if f == nil || v == nil || f.Sheet != v.WindowSheet {
		return
	}
	for row := maxInt(f.Bounds.StartRow, v.WindowRange.StartRow); row <= minInt(f.Bounds.EndRow, v.WindowRange.EndRow); row++ {
		index := row - 1
		if row == f.Bounds.StartRow || f.VisibleRows[row] {
			continue
		}
		_ = v.Rows.SetSize(index, 0)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

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

func (a *App) applySpreadsheetAutoSum() {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet || state.formatRunning.Load() {
		return
	}
	reader, hasReader := state.book.(document.SpreadsheetRangeReader)
	_, hasEditor := state.book.(document.SpreadsheetBatchEditor)
	if !hasReader || !hasEditor {
		a.setStatus("当前表格不支持自动求和")
		return
	}
	bounds := state.selection.Bounds()
	edits := computeAutoSumEdits(reader, state.sheet, bounds)
	if len(edits) == 0 {
		a.setStatus("未在选区中找到可求和的连续数值")
		return
	}
	if err := a.applyBatchEdits(edits, "求和"); err != nil {
		a.setStatus("求和失败：" + err.Error())
		return
	}
	a.navigateToCell(edits[0].Row, edits[0].Column)
	if len(edits) == 1 {
		a.setStatus(fmt.Sprintf("求和完成：%s = %s", xlsx.CellName(edits[0].Column, edits[0].Row), edits[0].Value))
	} else {
		a.setStatus(fmt.Sprintf("求和完成，已填入 %d 个单元格", len(edits)))
	}
}

func computeAutoSumEdits(reader document.SpreadsheetRangeReader, sheet string, bounds document.CellRange) []document.CellEdit {
	// Case 1: Single cell selected
	if bounds.StartRow == bounds.EndRow && bounds.StartColumn == bounds.EndColumn {
		row, col := bounds.StartRow, bounds.StartColumn
		if row > 1 {
			scanStartRow := maxInt(1, row-100)
			scanBounds := document.CellRange{StartRow: scanStartRow, StartColumn: col, EndRow: row - 1, EndColumn: col}
			cells, err := reader.ReadRange(sheet, scanBounds)
			if err == nil && len(cells) > 0 {
				var sum float64
				var count int
				for r := len(cells) - 1; r >= 0; r-- {
					v, ok := parseNumericValue(cells[r][0].Value)
					if !ok {
						break
					}
					sum += v
					count++
				}
				if count > 0 {
					return []document.CellEdit{{Row: row, Column: col, Value: formatSumNumber(sum)}}
				}
			}
		}
		if col > 1 {
			scanStartCol := maxInt(1, col-50)
			scanBounds := document.CellRange{StartRow: row, StartColumn: scanStartCol, EndRow: row, EndColumn: col - 1}
			cells, err := reader.ReadRange(sheet, scanBounds)
			if err == nil && len(cells) > 0 && len(cells[0]) > 0 {
				var sum float64
				var count int
				for c := len(cells[0]) - 1; c >= 0; c-- {
					v, ok := parseNumericValue(cells[0][c].Value)
					if !ok {
						break
					}
					sum += v
					count++
				}
				if count > 0 {
					return []document.CellEdit{{Row: row, Column: col, Value: formatSumNumber(sum)}}
				}
			}
		}
		return nil
	}

	// Case 2: Multi-cell selection
	cells, err := reader.ReadRange(sheet, bounds)
	if err != nil || len(cells) == 0 {
		return nil
	}

	// Subcase 2A: Multi-row selection
	if bounds.EndRow > bounds.StartRow {
		bottomRowEmpty := true
		lastRowIdx := len(cells) - 1
		for c := 0; c < len(cells[lastRowIdx]); c++ {
			if strings.TrimSpace(cells[lastRowIdx][c].Value) != "" {
				bottomRowEmpty = false
				break
			}
		}
		dataEndRowIdx := lastRowIdx
		targetRow := bounds.EndRow + 1
		if bottomRowEmpty {
			dataEndRowIdx = lastRowIdx - 1
			targetRow = bounds.EndRow
		}
		if dataEndRowIdx < 0 {
			return nil
		}

		var edits []document.CellEdit
		cols := bounds.EndColumn - bounds.StartColumn + 1
		for c := 0; c < cols; c++ {
			var colSum float64
			var count int
			for r := 0; r <= dataEndRowIdx; r++ {
				if c < len(cells[r]) {
					v, ok := parseNumericValue(cells[r][c].Value)
					if ok {
						colSum += v
						count++
					}
				}
			}
			if count > 0 {
				edits = append(edits, document.CellEdit{
					Row:    targetRow,
					Column: bounds.StartColumn + c,
					Value:  formatSumNumber(colSum),
				})
			}
		}
		return edits
	}

	// Subcase 2B: Single row, multi-column selection
	lastColEmpty := strings.TrimSpace(cells[0][len(cells[0])-1].Value) == ""
	dataEndColIdx := len(cells[0]) - 1
	targetCol := bounds.EndColumn + 1
	if lastColEmpty {
		dataEndColIdx = len(cells[0]) - 2
		targetCol = bounds.EndColumn
	}
	if dataEndColIdx < 0 {
		return nil
	}
	var rowSum float64
	var count int
	for c := 0; c <= dataEndColIdx; c++ {
		v, ok := parseNumericValue(cells[0][c].Value)
		if ok {
			rowSum += v
			count++
		}
	}
	if count > 0 {
		return []document.CellEdit{{
			Row:    bounds.StartRow,
			Column: targetCol,
			Value:  formatSumNumber(rowSum),
		}}
	}
	return nil
}

func parseNumericValue(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	s = strings.ReplaceAll(s, ",", "")
	if strings.HasSuffix(s, "%") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) {
			return v / 100.0, true
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func formatSumNumber(v float64) string {
	if math.Abs(v-math.Round(v)) < 1e-9 {
		return fmt.Sprintf("%.0f", math.Round(v))
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func (a *App) showSpreadsheetFindDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	searcher, ok := state.book.(document.SpreadsheetSearcher)
	if !ok {
		a.setStatus("当前表格不支持查找")
		return
	}
	a.modal.ShowInput("查找", "在当前工作表中查找内容", state.excelFindQuery, "查找内容", func(action string) {
		query := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if query == "" {
			a.setStatus("请输入要查找的内容")
			return
		}
		state.excelFindQuery = query
		matches, err := searcher.SearchSheet(state.sheet, query)
		if err != nil {
			a.setStatus("查找失败：" + err.Error())
			return
		}
		if len(matches) == 0 {
			a.setStatus("未找到：" + query)
			return
		}
		currentPos := state.selection.Active
		curRow, curCol := currentPos.Row+1, currentPos.Column+1
		target := matches[0]
		for _, m := range matches {
			if m.Row > curRow || (m.Row == curRow && m.Column > curCol) {
				target = m
				break
			}
		}
		a.navigateToCell(target.Row, target.Column)
		a.setStatus(fmt.Sprintf("已定位至 %s（共 %d 处匹配）", xlsx.CellName(target.Column, target.Row), len(matches)))
	}, modalDialogActionSpec{ID: "confirm", Label: "查找下一个"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func cellStyleToFormatPatch(style document.CellStyle) document.CellFormatPatch {
	bold := style.Bold
	italic := style.Italic
	underline := style.Underline
	wrapText := style.WrapText
	patch := document.CellFormatPatch{
		Bold:      &bold,
		Italic:    &italic,
		Underline: &underline,
		WrapText:  &wrapText,
	}
	if style.FontColor != "" {
		color := style.FontColor
		patch.FontColor = &color
	}
	if style.FillColor != "" {
		fill := style.FillColor
		patch.FillColor = &fill
	}
	if style.HorizontalAlignment != "" {
		align := style.HorizontalAlignment
		patch.HorizontalAlignment = &align
	}
	if style.NumberFormat != "" {
		numFmt := style.NumberFormat
		patch.NumberFormat = &numFmt
	}
	if style.FontSize > 0 {
		size := style.FontSize
		patch.FontSize = &size
	}
	if style.FontFamily != "" {
		family := style.FontFamily
		patch.FontFamily = &family
	}
	hasBorder := false
	for _, b := range style.Borders {
		if b.Style != 0 {
			hasBorder = true
			break
		}
	}
	if hasBorder {
		all := "all"
		patch.BorderPreset = &all
	}
	return patch
}

func (a *App) toggleSpreadsheetFormatPainter() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	if state.excelFormatPainterActive {
		a.cancelSpreadsheetFormatPainter()
		return
	}
	if !state.selectionSet {
		a.setStatus("请先选择要复制格式的单元格")
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	style := a.selectedCellStyle()
	patch := cellStyleToFormatPatch(style)
	state.excelFormatPainterActive = true
	state.excelFormatPainterPatch = &patch
	state.excelFormatPainterSourceCell = a.selectedCell
	a.setStatus("已启动格式刷；选择目标单元格区域应用，再点格式刷或按 Esc 取消")
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) cancelSpreadsheetFormatPainter() {
	state := a.documentSessionState
	if state == nil {
		return
	}
	state.excelFormatPainterActive = false
	state.excelFormatPainterPatch = nil
	state.excelFormatPainterSourceCell = ""
	a.setStatus("已取消格式刷")
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) applySpreadsheetFormatPainter() {
	state := a.documentSessionState
	if state == nil || !state.excelFormatPainterActive || state.excelFormatPainterPatch == nil || !state.selectionSet {
		return
	}
	patch := *state.excelFormatPainterPatch
	state.excelFormatPainterActive = false
	state.excelFormatPainterPatch = nil
	state.excelFormatPainterSourceCell = ""
	a.applyCellFormat(patch)
	a.setStatus("格式刷已应用至所选区域")
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) showSpreadsheetFontFamilyDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet {
		return
	}
	style := a.selectedCellStyle()
	currentFont := style.FontFamily
	if currentFont == "" {
		currentFont = "宋体"
	}
	a.modal.ShowInput("设置字体", "输入字体名称（如 宋体、Arial、Consolas、微软雅黑）", currentFont, "字体名称", func(action string) {
		value := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if value == "" {
			a.setStatus("字体名称不能为空")
			return
		}
		a.applyCellFormat(document.CellFormatPatch{FontFamily: &value})
	}, modalDialogActionSpec{ID: "confirm", Label: "应用"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) runSpreadsheetPageLayoutCommand(operation string, edit func(document.SpreadsheetPageLayoutEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	sheet, generation, window := state.sheet, a.documentGeneration, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetOrientationDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	currentOrient := "portrait"
	if layout, err := editor.GetSheetPageLayout(state.sheet); err == nil && layout.Orientation != "" {
		currentOrient = layout.Orientation
	}
	label := "纵向"
	if currentOrient == "landscape" {
		label = "横向"
	}
	message := fmt.Sprintf("当前工作表打印方向：%s\n请选择目标方向：", label)
	sheet := state.sheet
	a.modal.Show("纸张方向", message, func(action string) {
		a.modal.Hide()
		if action == "portrait" || action == "landscape" {
			a.runSpreadsheetPageLayoutCommand("修改纸张方向", func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPageOrientation(sheet, action)
			})
		}
	},
		modalDialogActionSpec{ID: "portrait", Label: "纵向"},
		modalDialogActionSpec{ID: "landscape", Label: "横向"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showSpreadsheetMarginsDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	top, right, bottom, left := 0.75, 0.7, 0.75, 0.7
	if layout, err := editor.GetSheetPageLayout(state.sheet); err == nil {
		if layout.MarginTop > 0 {
			top = layout.MarginTop
		}
		if layout.MarginRight > 0 {
			right = layout.MarginRight
		}
		if layout.MarginBottom > 0 {
			bottom = layout.MarginBottom
		}
		if layout.MarginLeft > 0 {
			left = layout.MarginLeft
		}
	}
	labels := []string{"上边距（英寸）", "右边距（英寸）", "下边距（英寸）", "左边距（英寸）"}
	hints := []string{"0.75", "0.7", "0.75", "0.7"}
	initial := []string{
		strconv.FormatFloat(top, 'f', -1, 64),
		strconv.FormatFloat(right, 'f', -1, 64),
		strconv.FormatFloat(bottom, 'f', -1, 64),
		strconv.FormatFloat(left, 'f', -1, 64),
	}
	sheet := state.sheet
	a.modal.ShowForm("页边距设置", "可选择快速预设或输入自定义页边距（英寸，1 英寸 = 2.54 厘米）：",
		labels, hints, initial,
		func(action string) {
			switch action {
			case "normal":
				a.modal.Hide()
				a.runSpreadsheetPageLayoutCommand("设置普通页边距", func(ed document.SpreadsheetPageLayoutEditor) error {
					return ed.SetSheetPageMargins(sheet, 0.75, 0.7, 0.75, 0.7)
				})
			case "wide":
				a.modal.Hide()
				a.runSpreadsheetPageLayoutCommand("设置宽页边距", func(ed document.SpreadsheetPageLayoutEditor) error {
					return ed.SetSheetPageMargins(sheet, 1.0, 1.0, 1.0, 1.0)
				})
			case "narrow":
				a.modal.Hide()
				a.runSpreadsheetPageLayoutCommand("设置窄页边距", func(ed document.SpreadsheetPageLayoutEditor) error {
					return ed.SetSheetPageMargins(sheet, 0.75, 0.25, 0.75, 0.25)
				})
			case "confirm":
				tVal, err1 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[0].Text()), 64)
				rVal, err2 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[1].Text()), 64)
				bVal, err3 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[2].Text()), 64)
				lVal, err4 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[3].Text()), 64)
				if err1 != nil || err2 != nil || err3 != nil || err4 != nil ||
					tVal < 0 || rVal < 0 || bVal < 0 || lVal < 0 ||
					tVal > 20 || rVal > 20 || bVal > 20 || lVal > 20 {
					a.setStatus("页边距必须是 0 到 20 英寸之间的数值")
					if a.window != nil {
						a.window.Invalidate()
					}
					return
				}
				a.modal.Hide()
				a.runSpreadsheetPageLayoutCommand("设置页边距", func(ed document.SpreadsheetPageLayoutEditor) error {
					return ed.SetSheetPageMargins(sheet, tVal, rVal, bVal, lVal)
				})
			case "cancel":
				a.modal.Hide()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "应用"},
		modalDialogActionSpec{ID: "normal", Label: "普通 (0.75/0.7\")"},
		modalDialogActionSpec{ID: "narrow", Label: "窄 (0.75/0.25\")"},
		modalDialogActionSpec{ID: "wide", Label: "宽 (1.0\")"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showSpreadsheetPageSizeDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	currentSize := 9
	if layout, err := editor.GetSheetPageLayout(state.sheet); err == nil && layout.PaperSize > 0 {
		currentSize = layout.PaperSize
	}
	sizeName := "A4 (210×297mm)"
	if currentSize == 1 {
		sizeName = "Letter (8.5×11in)"
	} else if currentSize == 8 {
		sizeName = "A3 (297×420mm)"
	}
	message := fmt.Sprintf("当前工作表纸张大小：%s\n请选择常用纸张规格：", sizeName)
	sheet := state.sheet
	a.modal.Show("纸张大小设置", message, func(action string) {
		a.modal.Hide()
		var paperSize int
		switch action {
		case "a4":
			paperSize = 9
		case "letter":
			paperSize = 1
		case "a3":
			paperSize = 8
		default:
			return
		}
		a.runSpreadsheetPageLayoutCommand("设置纸张大小", func(ed document.SpreadsheetPageLayoutEditor) error {
			return ed.SetSheetPageSize(sheet, paperSize)
		})
	},
		modalDialogActionSpec{ID: "a4", Label: "A4 (210×297mm)"},
		modalDialogActionSpec{ID: "letter", Label: "Letter (8.5×11in)"},
		modalDialogActionSpec{ID: "a3", Label: "A3 (297×420mm)"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showSpreadsheetPrintAreaDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	sheet := state.sheet
	layout, _ := editor.GetSheetPageLayout(sheet)
	var selRef string
	if state.selectionSet {
		bounds := state.selection.Bounds()
		selRef = fmt.Sprintf("$%s$%d:$%s$%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	}

	var message string
	if layout.PrintArea != "" {
		message = fmt.Sprintf("当前打印区域：%s\n\n您可将当前选区设为新的打印区域，或清除现有打印区域。", layout.PrintArea)
	} else {
		message = "当前未设置打印区域。\n\n您可将当前选区设为打印区域。"
	}

	actions := make([]modalDialogActionSpec, 0, 3)
	if selRef != "" {
		actions = append(actions, modalDialogActionSpec{ID: "set", Label: "设为打印区域 (" + selRef + ")"})
	}
	if layout.PrintArea != "" {
		actions = append(actions, modalDialogActionSpec{ID: "clear", Label: "清除打印区域"})
	}
	actions = append(actions, modalDialogActionSpec{ID: "cancel", Label: "取消"})

	a.modal.Show("打印区域设置", message, func(action string) {
		a.modal.Hide()
		switch action {
		case "set":
			if selRef == "" {
				return
			}
			a.runSpreadsheetPageLayoutCommand("设置打印区域", func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPrintArea(sheet, selRef)
			})
			a.setStatus("已设置打印区域：" + selRef)
		case "clear":
			a.runSpreadsheetPageLayoutCommand("清除打印区域", func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPrintArea(sheet, "")
			})
			a.setStatus("已清除打印区域")
		}
	}, actions...)
}

func (a *App) runSpreadsheetHyperlinkCommand(operation string, edit func(document.SpreadsheetHyperlinkEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetHyperlinkEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetLinkDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetHyperlinkEditor)
	if !ok {
		return
	}
	col := state.selection.Active.Column + 1
	row := state.selection.Active.Row + 1
	cellName := xlsx.CellName(col, row)
	sheet := state.sheet

	existingLink, hasLink, _ := editor.GetCellHyperlink(sheet, cellName)
	initialTarget := existingLink.Target
	initialTooltip := existingLink.Tooltip

	actions := []modalDialogActionSpec{
		{ID: "confirm", Label: "确定"},
	}
	if hasLink {
		actions = append(actions, modalDialogActionSpec{ID: "remove", Label: "删除链接"})
	}
	actions = append(actions, modalDialogActionSpec{ID: "cancel", Label: "取消"})

	a.modal.ShowDualInput("插入链接", fmt.Sprintf("为单元格 %s 设置超链接：", cellName),
		initialTarget, "链接地址 (例如 https://example.com 或 Sheet2!A1)",
		initialTooltip, "提示文本 (可选)",
		func(action string) {
			switch action {
			case "confirm":
				target := strings.TrimSpace(a.modal.Input.Text())
				tooltip := strings.TrimSpace(a.modal.SecondInput.Text())
				a.modal.Hide()
				if target == "" {
					a.setStatus("链接地址不能为空")
					return
				}
				a.runSpreadsheetHyperlinkCommand("插入链接", func(ed document.SpreadsheetHyperlinkEditor) error {
					return ed.SetCellHyperlink(sheet, cellName, target, tooltip)
				})
				a.setStatus(fmt.Sprintf("已为 %s 设置链接：%s", cellName, target))
			case "remove":
				a.modal.Hide()
				a.runSpreadsheetHyperlinkCommand("删除链接", func(ed document.SpreadsheetHyperlinkEditor) error {
					return ed.RemoveCellHyperlink(sheet, cellName)
				})
				a.setStatus(fmt.Sprintf("已删除 %s 的超链接", cellName))
			case "cancel":
				a.modal.Hide()
			}
		},
		actions...,
	)
}

func (a *App) showSpreadsheetPrintTitlesDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	sheet := state.sheet
	layout, _ := editor.GetSheetPageLayout(sheet)
	var selRowsRef string
	if state.selectionSet {
		bounds := state.selection.Bounds()
		selRowsRef = fmt.Sprintf("$%d:$%d", bounds.StartRow, bounds.EndRow)
	}

	var message string
	if layout.PrintTitles != "" {
		message = fmt.Sprintf("当前打印标题行：%s\n\n您可将当前选中行设为顶端标题行，或清除现有打印标题。", layout.PrintTitles)
	} else {
		message = "当前未设置打印标题行。\n\n在打印多页表格时，打印标题行会在每一页顶端重复打印。\n您可将当前选中行设为顶端标题行。"
	}

	actions := make([]modalDialogActionSpec, 0, 3)
	if selRowsRef != "" {
		actions = append(actions, modalDialogActionSpec{ID: "set", Label: "设为标题行 (" + selRowsRef + ")"})
	}
	if layout.PrintTitles != "" {
		actions = append(actions, modalDialogActionSpec{ID: "clear", Label: "清除标题行"})
	}
	actions = append(actions, modalDialogActionSpec{ID: "cancel", Label: "取消"})

	a.modal.Show("打印标题设置", message, func(action string) {
		a.modal.Hide()
		switch action {
		case "set":
			if selRowsRef == "" {
				return
			}
			a.runSpreadsheetPageLayoutCommand("设置打印标题行", func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPrintTitles(sheet, selRowsRef)
			})
			a.setStatus("已设置打印标题行：" + selRowsRef)
		case "clear":
			a.runSpreadsheetPageLayoutCommand("清除打印标题行", func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPrintTitles(sheet, "")
			})
			a.setStatus("已清除打印标题行")
		}
	}, actions...)
}

func (a *App) runSpreadsheetPanesCommand(operation string, edit func(document.SpreadsheetPanesEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPanesEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetFreezePanesDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPanesEditor)
	if !ok {
		return
	}
	sheet := state.sheet
	panes, _ := editor.GetSheetPanes(sheet)

	var activeCol, activeRow int
	if state.selectionSet {
		activeCol = state.selection.Active.Column
		activeRow = state.selection.Active.Row
	}

	actions := []modalDialogActionSpec{
		{ID: "freeze_row", Label: "冻结首行"},
		{ID: "freeze_col", Label: "冻结首列"},
	}
	if activeCol > 0 || activeRow > 0 {
		cellRef := xlsx.CellName(activeCol+1, activeRow+1)
		actions = append(actions, modalDialogActionSpec{ID: "freeze_sel", Label: "冻结至选区 (" + cellRef + ")"})
	}
	if panes.Frozen {
		actions = append(actions, modalDialogActionSpec{ID: "unfreeze", Label: "取消冻结"})
	}
	actions = append(actions, modalDialogActionSpec{ID: "cancel", Label: "取消"})

	var msg string
	if panes.Frozen {
		msg = fmt.Sprintf("当前工作表已冻结（行: %d，列: %d）。\n请选择调整或取消冻结：", panes.YSplit, panes.XSplit)
	} else {
		msg = "冻结工作表可以在滚动时保持特定行或列始终可见。\n请选择冻结方式："
	}

	a.modal.Show("冻结窗格", msg, func(action string) {
		a.modal.Hide()
		switch action {
		case "freeze_row":
			a.runSpreadsheetPanesCommand("冻结首行", func(ed document.SpreadsheetPanesEditor) error {
				return ed.SetSheetPanes(sheet, document.SpreadsheetPaneFreeze{Frozen: true, XSplit: 0, YSplit: 1})
			})
			a.setStatus("已冻结首行")
		case "freeze_col":
			a.runSpreadsheetPanesCommand("冻结首列", func(ed document.SpreadsheetPanesEditor) error {
				return ed.SetSheetPanes(sheet, document.SpreadsheetPaneFreeze{Frozen: true, XSplit: 1, YSplit: 0})
			})
			a.setStatus("已冻结首列")
		case "freeze_sel":
			a.runSpreadsheetPanesCommand("冻结选区", func(ed document.SpreadsheetPanesEditor) error {
				return ed.SetSheetPanes(sheet, document.SpreadsheetPaneFreeze{Frozen: true, XSplit: activeCol, YSplit: activeRow})
			})
			a.setStatus(fmt.Sprintf("已冻结至 %s", xlsx.CellName(activeCol+1, activeRow+1)))
		case "unfreeze":
			a.runSpreadsheetPanesCommand("取消冻结", func(ed document.SpreadsheetPanesEditor) error {
				return ed.SetSheetPanes(sheet, document.SpreadsheetPaneFreeze{Frozen: false})
			})
			a.setStatus("已取消冻结窗格")
		}
	}, actions...)
}

func (a *App) runSpreadsheetDuplicatesCommand(operation string, edit func(document.SpreadsheetDuplicatesEditor) (document.SpreadsheetDuplicatesResult, error)) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetDuplicatesEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		res, err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		if err == nil {
			a.setStatus(fmt.Sprintf("已删除 %d 个重复项，保留 %d 个唯一项", res.RemovedCount, res.RemainingCount))
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetRemoveDuplicatesDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	if !state.selectionSet {
		a.setStatus("请先选择包含数据的单元格区域")
		return
	}
	bounds := state.selection.Bounds()
	rowCount := bounds.EndRow - bounds.StartRow + 1
	if rowCount <= 1 {
		a.setStatus("选区至少需要 2 行才能执行删除重复项")
		return
	}
	sheet := state.sheet
	rangeRef := fmt.Sprintf("%s%d:%s%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	keyCol := state.selection.Active.Column + 1
	activeColName := columnName(keyCol)

	msg := fmt.Sprintf("将在选区 %s（共 %d 行）中查找并删除重复数据。\n默认按当前活动列（%s 列）排重，或按整行全部列排重。", rangeRef, rowCount, activeColName)

	a.modal.Show("删除重复项", msg, func(action string) {
		a.modal.Hide()
		switch action {
		case "col":
			a.runSpreadsheetDuplicatesCommand("按列删除重复项", func(ed document.SpreadsheetDuplicatesEditor) (document.SpreadsheetDuplicatesResult, error) {
				return ed.RemoveDuplicates(sheet, bounds, keyCol)
			})
		case "all":
			a.runSpreadsheetDuplicatesCommand("按整行删除重复项", func(ed document.SpreadsheetDuplicatesEditor) (document.SpreadsheetDuplicatesResult, error) {
				return ed.RemoveDuplicates(sheet, bounds, 0)
			})
		}
	},
		modalDialogActionSpec{ID: "col", Label: fmt.Sprintf("按 %s 列排重", activeColName)},
		modalDialogActionSpec{ID: "all", Label: "按全部列排重"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) runSpreadsheetTextToColumnsCommand(operation string, edit func(document.SpreadsheetTextToColumnsEditor) (document.SpreadsheetTextToColumnsResult, error)) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetTextToColumnsEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		res, err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		if err == nil {
			a.setStatus(fmt.Sprintf("已完成分列：处理 %d 行，最多拆分为 %d 列", res.RowsSplit, res.MaxCols))
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetTextToColumnsDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	if !state.selectionSet {
		a.setStatus("请先选择包含数据的单元格区域")
		return
	}
	bounds := state.selection.Bounds()
	sheet := state.sheet
	keyCol := state.selection.Active.Column + 1
	if keyCol < bounds.StartColumn || keyCol > bounds.EndColumn {
		keyCol = bounds.StartColumn
	}
	activeColName := columnName(keyCol)
	rangeRef := fmt.Sprintf("%s%d:%s%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	msg := fmt.Sprintf("将在选区 %s 中对 %s 列进行文本分列。\n选择预设分隔符或输入自定义字符向右拆分数据：", rangeRef, activeColName)

	splitWith := func(delim, label string) {
		a.runSpreadsheetTextToColumnsCommand("文本分列 ("+label+")", func(ed document.SpreadsheetTextToColumnsEditor) (document.SpreadsheetTextToColumnsResult, error) {
			return ed.SplitTextToColumns(sheet, bounds, keyCol, delim)
		})
	}

	a.modal.Show("文本分列", msg, func(action string) {
		a.modal.Hide()
		switch action {
		case "comma":
			splitWith(",", "逗号")
		case "semicolon":
			splitWith(";", "分号")
		case "space":
			splitWith(" ", "空格")
		case "tab":
			splitWith("\t", "制表符")
		case "custom":
			a.modal.ShowInput("自定义分列", "输入分隔符（支持单个字符、单词或 \\t 制表符）：", ",", "分隔符", func(act string) {
				delim := a.modal.Input.Text()
				a.modal.Hide()
				if act != "confirm" {
					return
				}
				if delim == "\\t" {
					delim = "\t"
				}
				if delim == "" {
					a.setStatus("分隔符不能为空")
					return
				}
				splitWith(delim, "自定义")
			}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
		}
	},
		modalDialogActionSpec{ID: "comma", Label: "逗号 (,)"},
		modalDialogActionSpec{ID: "semicolon", Label: "分号 (;)"},
		modalDialogActionSpec{ID: "space", Label: "空格 ( )"},
		modalDialogActionSpec{ID: "tab", Label: "制表符 (\\t)"},
		modalDialogActionSpec{ID: "custom", Label: "自定义..."},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) runSpreadsheetViewCommand(operation string, edit func(document.SpreadsheetViewsEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetViewsEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) setSpreadsheetViewMode(mode string) {
	state := a.documentSessionState
	if state == nil {
		return
	}
	label := "普通视图"
	if mode == "pageBreakPreview" {
		label = "分页预览"
	}
	state.spreadsheetViewMode = mode
	sheet := state.sheet
	a.runSpreadsheetViewCommand("切换"+label, func(editor document.SpreadsheetViewsEditor) error {
		return editor.SetSheetViewMode(sheet, mode)
	})
	a.setStatus("已切换为" + label)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) toggleSpreadsheetRuler() {
	state := a.documentSessionState
	if state == nil {
		return
	}
	state.spreadsheetRulerVisible = !state.spreadsheetRulerVisible
	show := state.spreadsheetRulerVisible
	sheet := state.sheet
	a.runSpreadsheetViewCommand("切换标尺", func(editor document.SpreadsheetViewsEditor) error {
		return editor.SetSheetRuler(sheet, show)
	})
	if show {
		a.setStatus("已显示标尺")
	} else {
		a.setStatus("已隐藏标尺")
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) runSpreadsheetProtectionCommand(operation string, edit func(document.SpreadsheetProtectionEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetProtectionEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetProtectSheetDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetProtectionEditor)
	if !ok {
		return
	}
	sheet := state.sheet
	isProtected, _ := editor.GetSheetProtection(sheet)

	if isProtected {
		a.modal.ShowInput("撤销保护工作表", "当前工作表已受保护。\n若设置了密码请输入密码（无密码直接确定）：", "", "密码（可选）", func(action string) {
			pwd := a.modal.Input.Text()
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			a.runSpreadsheetProtectionCommand("撤销保护工作表", func(ed document.SpreadsheetProtectionEditor) error {
				return ed.UnprotectSheet(sheet, pwd)
			})
			a.setStatus("已撤销工作表保护")
		},
			modalDialogActionSpec{ID: "confirm", Label: "撤销保护"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	} else {
		a.modal.ShowInput("保护工作表", "保护工作表可防止其他用户更改工作表中的数据。\n可设置保护密码（可选，留空无密码）：", "", "密码（可选）", func(action string) {
			pwd := a.modal.Input.Text()
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			a.runSpreadsheetProtectionCommand("保护工作表", func(ed document.SpreadsheetProtectionEditor) error {
				return ed.ProtectSheet(sheet, pwd)
			})
			a.setStatus("已保护工作表")
		},
			modalDialogActionSpec{ID: "confirm", Label: "保护工作表"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	}
}

func (a *App) runSpreadsheetWorkbookProtectionCommand(operation string, edit func(document.SpreadsheetWorkbookProtectionEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetWorkbookProtectionEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetProtectWorkbookDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetWorkbookProtectionEditor)
	if !ok {
		return
	}
	isProtected, _ := editor.GetWorkbookProtection()

	if isProtected {
		a.modal.ShowInput("撤销保护工作簿", "当前工作簿已受保护。\n若设置了密码请输入密码（无密码直接确定）：", "", "密码（可选）", func(action string) {
			pwd := a.modal.Input.Text()
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			a.runSpreadsheetWorkbookProtectionCommand("撤销保护工作簿", func(ed document.SpreadsheetWorkbookProtectionEditor) error {
				return ed.UnprotectWorkbook(pwd)
			})
			a.setStatus("已撤销工作簿保护")
		},
			modalDialogActionSpec{ID: "confirm", Label: "撤销保护"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	} else {
		a.modal.ShowInput("保护工作簿", "保护工作簿结构可防止其他用户查看隐藏的工作表、添加、移动、删除或重命名工作表。\n可设置保护密码（可选，留空无密码）：", "", "密码（可选）", func(action string) {
			pwd := a.modal.Input.Text()
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			a.runSpreadsheetWorkbookProtectionCommand("保护工作簿", func(ed document.SpreadsheetWorkbookProtectionEditor) error {
				return ed.ProtectWorkbook(pwd)
			})
			a.setStatus("已保护工作簿")
		},
			modalDialogActionSpec{ID: "confirm", Label: "保护工作簿"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	}
}

func (a *App) runSpreadsheetTableCommand(operation string, edit func(document.SpreadsheetTableEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetTableEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		if err == nil {
			a.setStatus("已创建表格")
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetInsertTableDialog(styleName string) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	if !state.selectionSet {
		a.setStatus("请先选择创建表格的单元格区域")
		return
	}
	bounds := state.selection.Bounds()
	if bounds.EndRow <= bounds.StartRow {
		a.setStatus("表格选区至少需要 2 行（包含标题行与数据行）")
		return
	}
	sheet := state.sheet
	rangeRef := fmt.Sprintf("%s%d:%s%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	rowCount := bounds.EndRow - bounds.StartRow + 1
	colCount := bounds.EndColumn - bounds.StartColumn + 1
	msg := fmt.Sprintf("将在选区 %s（共 %d 行 %d 列）创建格式化表格。\n表格将自动启用筛选与行列交替颜色。", rangeRef, rowCount, colCount)

	createTable := func(style string) {
		a.runSpreadsheetTableCommand("创建表格", func(ed document.SpreadsheetTableEditor) error {
			return ed.AddTable(document.SpreadsheetTableSpec{
				Sheet:     sheet,
				Range:     bounds,
				StyleName: style,
			})
		})
	}

	if styleName != "" {
		createTable(styleName)
		return
	}

	a.modal.Show("创建表格", msg, func(action string) {
		a.modal.Hide()
		switch action {
		case "medium":
			createTable("TableStyleMedium2")
		case "light":
			createTable("TableStyleLight1")
		case "dark":
			createTable("TableStyleDark1")
		}
	},
		modalDialogActionSpec{ID: "medium", Label: "中等风格 (推荐)"},
		modalDialogActionSpec{ID: "light", Label: "浅色风格"},
		modalDialogActionSpec{ID: "dark", Label: "深色风格"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) runSpreadsheetFormulaCommand(operation string, edit func(document.SpreadsheetFormulaEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetFormulaEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		if err == nil {
			a.setStatus("已设置公式")
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetInsertFunctionDialog(category string) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	_, ok := state.book.(document.SpreadsheetFormulaEditor)
	if !ok {
		a.setStatus("当前工作簿不支持插入函数")
		return
	}
	activeCol := state.selection.Active.Column + 1
	activeRow := state.selection.Active.Row + 1
	cellName := fmt.Sprintf("%s%d", columnName(activeCol), activeRow)

	defaultRef := "A1:A10"
	if state.selectionSet {
		b := state.selection.Bounds()
		if b.StartColumn == b.EndColumn && b.StartRow == b.EndRow {
			if activeRow > 1 {
				defaultRef = fmt.Sprintf("%s1:%s%d", columnName(activeCol), columnName(activeCol), activeRow-1)
			}
		} else {
			defaultRef = fmt.Sprintf("%s%d:%s%d", columnName(b.StartColumn), b.StartRow, columnName(b.EndColumn), b.EndRow)
		}
	}

	title := "插入函数"
	defaultFormula := fmt.Sprintf("=SUM(%s)", defaultRef)

	switch category {
	case "recent":
		title = "最近使用的函数"
		defaultFormula = fmt.Sprintf("=AVERAGE(%s)", defaultRef)
	case "logical":
		title = "逻辑函数"
		defaultFormula = fmt.Sprintf("=IF(%s1>0, 1, 0)", columnName(activeCol))
	case "text":
		title = "文本函数"
		defaultFormula = fmt.Sprintf("=CONCATENATE(%s1, \" \", %s2)", columnName(activeCol), columnName(activeCol))
	case "datetime":
		title = "日期时间函数"
		defaultFormula = "=TODAY()"
	}

	prompt := fmt.Sprintf("为单元格 %s 设置公式表达式：", cellName)
	a.modal.ShowInput(title, prompt, defaultFormula, "输入公式（例如 =SUM(A1:A10)）", func(action string) {
		formula := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" || formula == "" {
			return
		}
		a.runSpreadsheetFormulaCommand("插入公式", func(ed document.SpreadsheetFormulaEditor) error {
			return ed.SetCellFormula(state.sheet, cellName, formula)
		})
	}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) showSpreadsheetNameManagerDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	ed, ok := state.book.(document.SpreadsheetDefinedNameEditor)
	if !ok {
		a.setStatus("当前工作簿不支持名称管理器")
		return
	}
	existing, _ := ed.GetDefinedNames()
	b := state.selection.Bounds()
	defaultRef := fmt.Sprintf("'%s'!$%s$%d:$%s$%d", state.sheet, columnName(b.StartColumn), b.StartRow, columnName(b.EndColumn), b.EndRow)
	defaultName := fmt.Sprintf("区域%d", len(existing)+1)

	msg := fmt.Sprintf("当前已有 %d 个定义名称。\n请输入新名称并确认引用位置：", len(existing))
	a.modal.ShowInput("名称管理器", msg, defaultName, "名称", func(action string) {
		name := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" || name == "" {
			return
		}
		err := ed.SetDefinedName(document.SpreadsheetDefinedName{
			Name: name,
			Ref:  defaultRef,
		})
		if err != nil {
			a.setStatus("创建定义名称失败：" + err.Error())
			return
		}
		a.setStatus(fmt.Sprintf("已创建定义名称：“%s” -> %s", name, defaultRef))
		if a.window != nil {
			a.window.Invalidate()
		}
	}, modalDialogActionSpec{ID: "confirm", Label: "新建名称"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) applySpreadsheetFlashFill() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetFlashFillEditor)
	if !ok {
		a.setStatus("当前工作簿不支持快速填充")
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	bounds := state.selection.Bounds()
	go func() {
		unlock := lockSessionAdapterWrite(state)
		res, err := editor.FlashFill(sheet, bounds)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		if err == nil {
			a.setStatus(fmt.Sprintf("快速填充完成，已自动填充 %d 个单元格", res.RowsFilled))
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "快速填充"}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetDataValidationDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetDataValidationEditor)
	if !ok {
		a.setStatus("当前工作簿不支持数据验证")
		return
	}
	bounds := state.selection.Bounds()
	prompt := fmt.Sprintf("为选区 %s%d:%s%d 设置下拉列表选项（用逗号分隔，留空则清除验证）：",
		columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	a.modal.ShowInput("数据验证", prompt, "男,女", "例如: 是,否 或 优秀,良好,及格", func(action string) {
		input := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return
		}
		go func() {
			unlock := lockSessionAdapterWrite(state)
			var err error
			if input == "" {
				err = editor.DeleteDataValidation(sheet, bounds)
			} else {
				normalized := strings.ReplaceAll(input, "，", ",")
				parts := strings.Split(normalized, ",")
				var items []string
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						items = append(items, p)
					}
				}
				if len(items) > 0 {
					err = editor.AddDataValidation(document.SpreadsheetDataValidationSpec{
						Sheet:    sheet,
						Range:    bounds,
						Type:     document.DataValidationTypeList,
						DropList: items,
					})
				} else {
					err = editor.DeleteDataValidation(sheet, bounds)
				}
			}
			unlock()
			a.completeSessionOperation(state, token, err, true)
			if err == nil {
				if input == "" {
					a.setStatus("已清除数据验证")
				} else {
					a.setStatus("已设置数据验证下拉列表")
				}
			}
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "数据验证"}:
			case <-a.shutdown:
			}
		}()
	}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) showSpreadsheetNewCommentDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetCommentEditor)
	if !ok {
		a.setStatus("当前工作簿不支持批注")
		return
	}
	activeCol := state.selection.Active.Column + 1
	activeRow := state.selection.Active.Row + 1
	cellName := xlsx.CellName(activeCol, activeRow)
	if cellName == "" {
		cellName = fmt.Sprintf("%s%d", columnName(activeCol), activeRow)
	}

	prompt := fmt.Sprintf("为单元格 %s 添加批注：", cellName)
	a.modal.ShowInput("新建批注", prompt, "", "输入批注内容", func(action string) {
		text := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" || text == "" {
			return
		}
		generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return
		}
		go func() {
			unlock := lockSessionAdapterWrite(state)
			err := editor.AddComment(sheet, document.SpreadsheetComment{
				Author: "用户",
				Cell:   cellName,
				Text:   text,
			})
			unlock()
			a.completeSessionOperation(state, token, err, true)
			if err == nil {
				a.setStatus(fmt.Sprintf("已为单元格 %s 添加批注", cellName))
			}
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "新建批注"}:
			case <-a.shutdown:
			}
		}()
	}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) deleteSpreadsheetComment() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetCommentEditor)
	if !ok {
		a.setStatus("当前工作簿不支持批注")
		return
	}
	activeCol := state.selection.Active.Column + 1
	activeRow := state.selection.Active.Row + 1
	cellName := xlsx.CellName(activeCol, activeRow)
	if cellName == "" {
		cellName = fmt.Sprintf("%s%d", columnName(activeCol), activeRow)
	}

	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := editor.DeleteComment(sheet, cellName)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		if err == nil {
			a.setStatus(fmt.Sprintf("已删除单元格 %s 的批注", cellName))
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "删除批注"}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetConditionalFormatDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetConditionalFormatEditor)
	if !ok {
		a.setStatus("当前工作簿不支持条件格式")
		return
	}
	bounds := state.selection.Bounds()
	prompt := fmt.Sprintf("为选区 %s%d:%s%d 选择条件格式规则：",
		columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	a.modal.ShowInput("条件格式", prompt, "data_bar", "规则: data_bar(数据条), 3_color(色阶), duplicate(重复值), above_avg(高于平均值), clear(清除)", func(action string) {
		a.modal.Hide()
		var ruleType document.SpreadsheetConditionalFormatType
		clear := false
		switch action {
		case "confirm":
			val := strings.TrimSpace(a.modal.Input.Text())
			switch val {
			case "3_color", "color", "scale":
				ruleType = document.ConditionalFormat3ColorScale
			case "duplicate", "dup":
				ruleType = document.ConditionalFormatDuplicate
			case "above_avg", "avg":
				ruleType = document.ConditionalFormatAboveAvg
			case "clear", "none", "":
				clear = true
			default:
				ruleType = document.ConditionalFormatDataBar
			}
		case "data_bar":
			ruleType = document.ConditionalFormatDataBar
		case "3_color":
			ruleType = document.ConditionalFormat3ColorScale
		case "duplicate":
			ruleType = document.ConditionalFormatDuplicate
		case "above_avg":
			ruleType = document.ConditionalFormatAboveAvg
		case "clear":
			clear = true
		default:
			return
		}

		generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return
		}
		go func() {
			unlock := lockSessionAdapterWrite(state)
			var err error
			if clear {
				err = editor.ClearConditionalFormat(sheet, bounds)
			} else {
				err = editor.SetConditionalFormat(document.SpreadsheetConditionalFormatSpec{
					Sheet: sheet,
					Range: bounds,
					Type:  ruleType,
				})
			}
			unlock()
			a.completeSessionOperation(state, token, err, true)
			if err == nil {
				if clear {
					a.setStatus("已清除条件格式")
				} else {
					a.setStatus("已应用条件格式")
				}
			}
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "条件格式"}:
			case <-a.shutdown:
			}
		}()
	},
		modalDialogActionSpec{ID: "data_bar", Label: "渐变数据条"},
		modalDialogActionSpec{ID: "3_color", Label: "三色色阶"},
		modalDialogActionSpec{ID: "duplicate", Label: "高亮重复值"},
		modalDialogActionSpec{ID: "above_avg", Label: "高于平均值"},
		modalDialogActionSpec{ID: "clear", Label: "清除规则"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}








