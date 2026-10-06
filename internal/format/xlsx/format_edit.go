package xlsx

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

type cellStyleChange struct {
	cell string
	from int
	to   int
}

func (w *Workbook) SetCellFormats(sheet string, bounds document.CellRange, patch document.CellFormatPatch) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if err := validateCellFormatPatch(patch); err != nil {
		return err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if bounds.StartRow < 1 || bounds.StartColumn < 1 || bounds.EndRow < bounds.StartRow || bounds.EndColumn < bounds.StartColumn || bounds.EndRow > maxXLSXRows || bounds.EndColumn > maxXLSXColumns {
		return errors.New("cell format range is outside XLSX worksheet limits")
	}
	if (bounds.EndRow-bounds.StartRow+1)*(bounds.EndColumn-bounds.StartColumn+1) > maxClipboardCells {
		return fmt.Errorf("format range exceeds %d cells", maxClipboardCells)
	}

	styleCache := make(map[int]*excelize.Style)
	styleIDs := make(map[string]int)
	changes := make([]cellStyleChange, 0)
	for row := bounds.StartRow; row <= bounds.EndRow; row++ {
		for column := bounds.StartColumn; column <= bounds.EndColumn; column++ {
			cell := CellName(column, row)
			before, err := w.file.GetCellStyle(sheet, cell)
			if err != nil {
				return fmt.Errorf("read style for %s: %w", cell, err)
			}
			base, ok := styleCache[before]
			if !ok {
				base, err = w.file.GetStyle(before)
				if err != nil {
					return fmt.Errorf("read style %d: %w", before, err)
				}
				styleCache[before] = base
			}
			updated := cloneExcelizeStyle(base)
			if patch.ClearFormat {
				if before == 0 {
					continue
				}
				changes = append(changes, cellStyleChange{cell: cell, from: before, to: 0})
				continue
			}
			applyCellFormatPatch(&updated, patch, row, column, bounds)
			if reflect.DeepEqual(*base, updated) {
				continue
			}
			keyBytes, err := json.Marshal(updated)
			if err != nil {
				return fmt.Errorf("encode style for %s: %w", cell, err)
			}
			key := string(keyBytes)
			after, ok := styleIDs[key]
			if !ok {
				after, err = w.file.NewStyle(&updated)
				if err != nil {
					return fmt.Errorf("create style for %s: %w", cell, err)
				}
				styleIDs[key] = after
			}
			changes = append(changes, cellStyleChange{cell: cell, from: before, to: after})
		}
	}
	if len(changes) == 0 {
		return nil
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return fmt.Errorf("capture format history: %w", err)
	}
	for index, change := range changes {
		if err := w.file.SetCellStyle(sheet, change.cell, change.cell, change.to); err != nil {
			for rollback := index - 1; rollback >= 0; rollback-- {
				previous := changes[rollback]
				_ = w.file.SetCellStyle(sheet, previous.cell, previous.cell, previous.from)
			}
			return fmt.Errorf("set style for %s: %w", change.cell, err)
		}
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		for _, previous := range changes {
			_ = w.file.SetCellStyle(sheet, previous.cell, previous.cell, previous.from)
		}
		return fmt.Errorf("record format history: %w", err)
	}
	w.dirty = true
	return nil
}

func validateCellFormatPatch(patch document.CellFormatPatch) error {
	if patch.ClearFormat {
		if patch.Bold != nil || patch.Italic != nil || patch.Underline != nil || patch.FontColor != nil || patch.FillColor != nil || patch.HorizontalAlignment != nil || patch.NumberFormat != nil || patch.BorderPreset != nil || patch.WrapText != nil || patch.FontSize != nil || patch.FontFamily != nil {
			return errors.New("clear format cannot be combined with other format changes")
		}
		return nil
	}
	if patch.FontSize != nil && (*patch.FontSize <= 0 || *patch.FontSize > 409) {
		return fmt.Errorf("invalid font size %v (must be between 1 and 409)", *patch.FontSize)
	}
	if patch.FontFamily != nil && strings.TrimSpace(*patch.FontFamily) == "" {
		return errors.New("font family cannot be empty")
	}
	if patch.FontColor != nil && *patch.FontColor != "" && normalizeRGBColor(*patch.FontColor) == "" {
		return errors.New("font color must be a six-digit RGB color")
	}
	if patch.FillColor != nil && *patch.FillColor != "" && normalizeRGBColor(*patch.FillColor) == "" {
		return errors.New("fill color must be a six-digit RGB color")
	}
	if patch.HorizontalAlignment != nil {
		switch strings.ToLower(*patch.HorizontalAlignment) {
		case "general", "left", "center", "right":
		default:
			return fmt.Errorf("unsupported horizontal alignment %q", *patch.HorizontalAlignment)
		}
	}
	if patch.NumberFormat != nil {
		switch *patch.NumberFormat {
		case "General", "0", "0.00", "0%", "0.00%", "yyyy-mm-dd":
		default:
			return fmt.Errorf("unsupported number format %q", *patch.NumberFormat)
		}
	}
	if patch.BorderPreset != nil {
		switch *patch.BorderPreset {
		case "all", "outside", "bottom", "none":
		default:
			return fmt.Errorf("unsupported border preset %q", *patch.BorderPreset)
		}
	}
	return nil
}

func cloneExcelizeStyle(style *excelize.Style) excelize.Style {
	result := *style
	if style.Font != nil {
		font := *style.Font
		result.Font = &font
	}
	result.Fill.Color = append([]string(nil), style.Fill.Color...)
	if style.Alignment != nil {
		alignment := *style.Alignment
		result.Alignment = &alignment
	}
	result.Border = append([]excelize.Border(nil), style.Border...)
	if style.CustomNumFmt != nil {
		format := *style.CustomNumFmt
		result.CustomNumFmt = &format
	}
	if style.DecimalPlaces != nil {
		places := *style.DecimalPlaces
		result.DecimalPlaces = &places
	}
	if style.Protection != nil {
		protection := *style.Protection
		result.Protection = &protection
	}
	return result
}

func applyCellFormatPatch(style *excelize.Style, patch document.CellFormatPatch, row, column int, bounds document.CellRange) {
	if patch.Bold != nil || patch.Italic != nil || patch.Underline != nil || patch.FontColor != nil || patch.FontSize != nil || patch.FontFamily != nil {
		if style.Font == nil {
			style.Font = &excelize.Font{}
		}
		if patch.Bold != nil {
			style.Font.Bold = *patch.Bold
		}
		if patch.Italic != nil {
			style.Font.Italic = *patch.Italic
		}
		if patch.Underline != nil {
			if *patch.Underline {
				style.Font.Underline = "single"
			} else {
				style.Font.Underline = ""
			}
		}
		if patch.FontColor != nil {
			style.Font.Color = excelizeRGBColor(*patch.FontColor)
		}
		if patch.FontSize != nil && *patch.FontSize > 0 {
			style.Font.Size = *patch.FontSize
		}
		if patch.FontFamily != nil && *patch.FontFamily != "" {
			style.Font.Family = *patch.FontFamily
		}
	}
	if patch.FillColor != nil {
		if *patch.FillColor == "" {
			style.Fill = excelize.Fill{}
		} else {
			style.Fill = excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{excelizeRGBColor(*patch.FillColor)}}
		}
	}
	if patch.HorizontalAlignment != nil {
		if style.Alignment == nil {
			style.Alignment = &excelize.Alignment{}
		}
		if strings.EqualFold(*patch.HorizontalAlignment, "general") {
			style.Alignment.Horizontal = ""
		} else {
			style.Alignment.Horizontal = strings.ToLower(*patch.HorizontalAlignment)
		}
	}
	if patch.WrapText != nil {
		if style.Alignment == nil {
			style.Alignment = &excelize.Alignment{}
		}
		style.Alignment.WrapText = *patch.WrapText
	}
	if patch.NumberFormat != nil {
		style.CustomNumFmt = nil
		style.DecimalPlaces = nil
		switch *patch.NumberFormat {
		case "General":
			style.NumFmt = 0
		case "0":
			style.NumFmt = 1
		case "0.00":
			style.NumFmt = 2
		case "0%":
			style.NumFmt = 9
		case "0.00%":
			style.NumFmt = 10
		case "yyyy-mm-dd":
			format := *patch.NumberFormat
			style.CustomNumFmt = &format
			style.NumFmt = 0
		}
	}
	if patch.BorderPreset != nil {
		applyBorderPreset(style, *patch.BorderPreset, row, column, bounds)
	}
}

func excelizeRGBColor(value string) string {
	if value == "" {
		return ""
	}
	return strings.TrimPrefix(normalizeRGBColor(value), "#")
}

func applyBorderPreset(style *excelize.Style, preset string, row, column int, bounds document.CellRange) {
	if preset == "none" {
		style.Border = nil
		return
	}
	set := map[string]bool{}
	switch preset {
	case "all":
		set = map[string]bool{"top": true, "right": true, "bottom": true, "left": true}
	case "outside":
		set["top"] = row == bounds.StartRow
		set["bottom"] = row == bounds.EndRow
		set["left"] = column == bounds.StartColumn
		set["right"] = column == bounds.EndColumn
	case "bottom":
		set["bottom"] = true
	}
	for side, enabled := range set {
		if enabled {
			putBorder(style, side)
		}
	}
}

func putBorder(style *excelize.Style, side string) {
	for index := range style.Border {
		if strings.EqualFold(style.Border[index].Type, side) {
			style.Border[index].Color = "FF000000"
			style.Border[index].Style = 1
			return
		}
	}
	style.Border = append(style.Border, excelize.Border{Type: side, Color: "FF000000", Style: 1})
}

func cellNumberFormat(style *excelize.Style) string {
	if style.CustomNumFmt != nil {
		return *style.CustomNumFmt
	}
	switch style.NumFmt {
	case 0:
		return "General"
	case 1:
		return "0"
	case 2:
		return "0.00"
	case 9:
		return "0%"
	case 10:
		return "0.00%"
	case 14:
		return "m/d/yy"
	default:
		return fmt.Sprintf("builtin:%d", style.NumFmt)
	}
}
