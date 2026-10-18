package xlsx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

var _ document.SpreadsheetMergeCellsEditor = (*Workbook)(nil)

func (w *Workbook) MergeCellsChanged(sheet string, bounds document.CellRange, confirmationToken string) (document.SpreadsheetMergeResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return document.SpreadsheetMergeResult{}, err
	}
	if err := validateRange(sheet, bounds, w.file.GetSheetList()); err != nil {
		return document.SpreadsheetMergeResult{}, err
	}
	bounds = normalizeMergeBounds(bounds)
	rows, columns := bounds.EndRow-bounds.StartRow+1, bounds.EndColumn-bounds.StartColumn+1
	if rows > maxClipboardCells/columns {
		return document.SpreadsheetMergeResult{}, fmt.Errorf("合并范围超过 %d 个单元格", maxClipboardCells)
	}

	merges, err := w.file.GetMergeCells(sheet)
	if err != nil {
		return document.SpreadsheetMergeResult{}, fmt.Errorf("读取已合并单元格: %w", err)
	}
	var unmergeRange *document.CellRange
	for _, merge := range merges {
		mergeBounds, err := formulaReferenceBounds(merge.GetStartAxis() + ":" + merge.GetEndAxis())
		if err != nil {
			return document.SpreadsheetMergeResult{}, fmt.Errorf("识别已合并范围: %w", err)
		}
		if !rangesIntersect(bounds, mergeBounds) {
			continue
		}
		selectsExistingMerge := mergeBounds == bounds || rows == 1 && columns == 1 &&
			bounds.StartRow >= mergeBounds.StartRow && bounds.StartRow <= mergeBounds.EndRow &&
			bounds.StartColumn >= mergeBounds.StartColumn && bounds.StartColumn <= mergeBounds.EndColumn
		if !selectsExistingMerge || unmergeRange != nil {
			return document.SpreadsheetMergeResult{}, fmt.Errorf("所选范围与已合并单元格相交；请先单独取消合并")
		}
		copy := mergeBounds
		unmergeRange = &copy
	}
	if unmergeRange != nil {
		return w.unmergeCellsLocked(sheet, *unmergeRange)
	}
	if rows*columns < 2 {
		return document.SpreadsheetMergeResult{}, fmt.Errorf("请至少选择两个单元格")
	}
	if err := w.rejectMergeInTablesLocked(sheet, bounds); err != nil {
		return document.SpreadsheetMergeResult{}, err
	}
	groups, err := w.clipboardFormulaGroupsLocked(sheet)
	if err != nil {
		return document.SpreadsheetMergeResult{}, fmt.Errorf("检查公式范围: %w", err)
	}
	for _, group := range groups {
		if rangesIntersect(group.bounds, bounds) {
			return document.SpreadsheetMergeResult{}, fmt.Errorf("无法合并包含 %s 公式范围 %s 的区域", group.kind, group.ref)
		}
	}

	discarded, token, err := w.mergeDiscardSummaryLocked(sheet, bounds)
	if err != nil {
		return document.SpreadsheetMergeResult{}, err
	}
	if discarded > 0 && confirmationToken != token {
		return document.SpreadsheetMergeResult{
			ConfirmationRequired: true,
			DiscardedCellCount:   discarded,
			ConfirmationToken:    token,
		}, nil
	}
	return w.mergeCellsLocked(sheet, bounds)
}

func normalizeMergeBounds(bounds document.CellRange) document.CellRange {
	if bounds.StartRow > bounds.EndRow {
		bounds.StartRow, bounds.EndRow = bounds.EndRow, bounds.StartRow
	}
	if bounds.StartColumn > bounds.EndColumn {
		bounds.StartColumn, bounds.EndColumn = bounds.EndColumn, bounds.StartColumn
	}
	return bounds
}

func (w *Workbook) mergeDiscardSummaryLocked(sheet string, bounds document.CellRange) (int, string, error) {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%s\x00%d:%d:%d:%d\x00", sheet, bounds.StartRow, bounds.StartColumn, bounds.EndRow, bounds.EndColumn)
	discarded := 0
	for row := bounds.StartRow; row <= bounds.EndRow; row++ {
		for column := bounds.StartColumn; column <= bounds.EndColumn; column++ {
			if row == bounds.StartRow && column == bounds.StartColumn {
				continue
			}
			ref, err := excelize.CoordinatesToCellName(column, row)
			if err != nil {
				return 0, "", err
			}
			formula, err := w.file.GetCellFormula(sheet, ref)
			if err != nil {
				return 0, "", fmt.Errorf("读取 %s 公式: %w", ref, err)
			}
			value, err := w.file.GetCellValue(sheet, ref, excelize.Options{RawCellValue: true})
			if err != nil {
				return 0, "", fmt.Errorf("读取 %s 内容: %w", ref, err)
			}
			if formula == "" && value == "" {
				continue
			}
			discarded++
			_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%s\x00", ref, formula, value)
		}
	}
	if discarded == 0 {
		return 0, "", nil
	}
	return discarded, hex.EncodeToString(hash.Sum(nil)), nil
}

func (w *Workbook) rejectMergeInTablesLocked(sheet string, bounds document.CellRange) error {
	tables, err := w.file.GetTables(sheet)
	if err != nil {
		return fmt.Errorf("读取表格范围: %w", err)
	}
	for _, table := range tables {
		tableBounds, err := formulaReferenceBounds(table.Range)
		if err != nil {
			return fmt.Errorf("识别表格 %q 范围: %w", table.Name, err)
		}
		if rangesIntersect(tableBounds, bounds) {
			return fmt.Errorf("无法合并 Excel 表格 %q 内的单元格", table.Name)
		}
	}
	return nil
}

func (w *Workbook) mergeCellsLocked(sheet string, bounds document.CellRange) (document.SpreadsheetMergeResult, error) {
	anchor, err := excelize.CoordinatesToCellName(bounds.StartColumn, bounds.StartRow)
	if err != nil {
		return document.SpreadsheetMergeResult{}, err
	}
	end, err := excelize.CoordinatesToCellName(bounds.EndColumn, bounds.EndRow)
	if err != nil {
		return document.SpreadsheetMergeResult{}, err
	}
	styleID, err := w.file.GetCellStyle(sheet, anchor)
	if err != nil {
		return document.SpreadsheetMergeResult{}, fmt.Errorf("读取 %s 样式: %w", anchor, err)
	}
	baseStyle, err := w.file.GetStyle(styleID)
	if err != nil {
		return document.SpreadsheetMergeResult{}, fmt.Errorf("读取 %s 样式 %d: %w", anchor, styleID, err)
	}
	centeredStyle := cloneExcelizeStyle(baseStyle)
	if centeredStyle.Alignment == nil {
		centeredStyle.Alignment = &excelize.Alignment{}
	}
	centeredStyle.Alignment.Horizontal = "center"

	before, err := w.snapshotLocked()
	if err != nil {
		return document.SpreadsheetMergeResult{}, fmt.Errorf("保存合并前工作簿状态: %w", err)
	}
	newStyleID, err := w.file.NewStyle(&centeredStyle)
	if err != nil {
		return document.SpreadsheetMergeResult{}, w.restoreMergeSnapshotErrorLocked(before, fmt.Errorf("创建合并后的单元格样式: %w", err))
	}
	if err := w.file.MergeCell(sheet, anchor, end); err != nil {
		return document.SpreadsheetMergeResult{}, w.restoreMergeSnapshotErrorLocked(before, fmt.Errorf("合并单元格: %w", err))
	}
	if err := w.file.SetCellStyle(sheet, anchor, anchor, newStyleID); err != nil {
		return document.SpreadsheetMergeResult{}, w.restoreMergeSnapshotErrorLocked(before, fmt.Errorf("居中合并单元格: %w", err))
	}
	if err := w.recordHistoryLocked(before); err != nil {
		return document.SpreadsheetMergeResult{}, w.restoreMergeSnapshotErrorLocked(before, fmt.Errorf("记录合并历史: %w", err))
	}
	w.dirty = true
	return document.SpreadsheetMergeResult{Changed: true}, nil
}

func (w *Workbook) unmergeCellsLocked(sheet string, bounds document.CellRange) (document.SpreadsheetMergeResult, error) {
	start, err := excelize.CoordinatesToCellName(bounds.StartColumn, bounds.StartRow)
	if err != nil {
		return document.SpreadsheetMergeResult{}, err
	}
	end, err := excelize.CoordinatesToCellName(bounds.EndColumn, bounds.EndRow)
	if err != nil {
		return document.SpreadsheetMergeResult{}, err
	}
	before, err := w.snapshotLocked()
	if err != nil {
		return document.SpreadsheetMergeResult{}, fmt.Errorf("保存取消合并前工作簿状态: %w", err)
	}
	if err := w.file.UnmergeCell(sheet, start, end); err != nil {
		return document.SpreadsheetMergeResult{}, w.restoreMergeSnapshotErrorLocked(before, fmt.Errorf("取消合并单元格: %w", err))
	}
	if err := w.recordHistoryLocked(before); err != nil {
		return document.SpreadsheetMergeResult{}, w.restoreMergeSnapshotErrorLocked(before, fmt.Errorf("记录取消合并历史: %w", err))
	}
	w.dirty = true
	return document.SpreadsheetMergeResult{Changed: true, Unmerged: true}, nil
}

func (w *Workbook) restoreMergeSnapshotErrorLocked(snapshot []byte, operationErr error) error {
	restored, err := excelize.OpenReader(bytes.NewReader(snapshot))
	if err != nil {
		return fmt.Errorf("%v; 且无法恢复合并前工作簿: %w", operationErr, err)
	}
	previous := w.file
	w.file = restored
	w.pictureIndexes = nil
	w.invalidateStyleCacheLocked()
	if previous != nil {
		_ = previous.Close()
	}
	return operationErr
}
