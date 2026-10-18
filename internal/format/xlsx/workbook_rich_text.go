package xlsx

import (
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

var _ document.SpreadsheetCellRichTextEditor = (*Workbook)(nil)

func normalizeHexColor(c string) string {
	c = strings.TrimSpace(strings.TrimPrefix(c, "#"))
	if len(c) == 8 { // ARGB
		c = c[2:]
	}
	if len(c) == 6 {
		return "#" + strings.ToUpper(c)
	}
	return ""
}

func (w *Workbook) GetCellRichText(sheet, cell string) ([]document.CellRichTextRun, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	runs, err := w.file.GetCellRichText(sheet, cell)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	docRuns := make([]document.CellRichTextRun, len(runs))
	for i, r := range runs {
		run := document.CellRichTextRun{Text: r.Text}
		if r.Font != nil {
			run.Bold = r.Font.Bold
			run.Italic = r.Font.Italic
			run.Underline = r.Font.Underline == "single" || (r.Font.Underline != "" && r.Font.Underline != "none")
			run.FontFamily = r.Font.Family
			run.FontSize = r.Font.Size
			if r.Font.Color != "" {
				run.FontColor = normalizeHexColor(r.Font.Color)
			}
		}
		docRuns[i] = run
	}
	return docRuns, nil
}

func (w *Workbook) SetCellRichText(sheet, cell string, runs []document.CellRichTextRun) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	excelizeRuns := make([]excelize.RichTextRun, len(runs))
	for i, r := range runs {
		u := ""
		if r.Underline {
			u = "single"
		}
		colorHex := strings.TrimPrefix(r.FontColor, "#")
		excelizeRuns[i] = excelize.RichTextRun{
			Text: r.Text,
			Font: &excelize.Font{
				Bold:      r.Bold,
				Italic:    r.Italic,
				Underline: u,
				Family:    r.FontFamily,
				Size:      r.FontSize,
				Color:     colorHex,
			},
		}
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return fmt.Errorf("capture rich text history: %w", err)
	}
	if err := w.file.SetCellRichText(sheet, cell, excelizeRuns); err != nil {
		return err
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	w.dirty = true
	return nil
}

type cellCharFormat struct {
	bold      bool
	italic    bool
	underline bool
	family    string
	size      float64
	color     string
}

func (w *Workbook) SetCellTextSelectionFormat(sheet, cell, text string, startRune, endRune int, patch document.CellFormatPatch) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	column, row, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		return fmt.Errorf("invalid cell reference %q: %w", cell, err)
	}
	if row > maxXLSXRows || column > maxXLSXColumns {
		return fmt.Errorf("cell reference %q exceeds XLSX worksheet limits", cell)
	}

	if text == "" {
		val, valErr := w.file.GetCellValue(sheet, cell)
		if valErr != nil {
			return valErr
		}
		text = val
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	if startRune < 0 {
		startRune = 0
	}
	if endRune > len(runes) {
		endRune = len(runes)
	}
	if startRune >= endRune {
		return errors.New("empty text selection range")
	}

	// Read existing style defaults for cell.
	styleID, _ := w.file.GetCellStyle(sheet, cell)
	defaultStyle, _ := w.cellStyleLocked(styleID)

	defaultFont, _ := w.file.GetDefaultFont()
	if defaultFont == "" {
		defaultFont = "Calibri"
	}
	defaultFamily := defaultStyle.FontFamily
	if defaultFamily == "" {
		defaultFamily = defaultFont
	}
	defaultSize := 11.0
	if defaultStyle.FontSize > 0 {
		defaultSize = defaultStyle.FontSize
	}
	defaultColor := defaultStyle.FontColor
	if defaultColor == "" {
		defaultColor = "#000000"
	}

	chars := make([]cellCharFormat, len(runes))
	for i := range chars {
		chars[i] = cellCharFormat{
			bold:      defaultStyle.Bold,
			italic:    defaultStyle.Italic,
			underline: defaultStyle.Underline,
			family:    defaultFamily,
			size:      defaultSize,
			color:     defaultColor,
		}
	}

	// If existing runs match text length, populate existing styles.
	existingRuns, _ := w.file.GetCellRichText(sheet, cell)
	var concatRunes []rune
	for _, r := range existingRuns {
		concatRunes = append(concatRunes, []rune(r.Text)...)
	}
	if len(concatRunes) == len(runes) && string(concatRunes) == text {
		idx := 0
		for _, r := range existingRuns {
			rRunes := []rune(r.Text)
			for range rRunes {
				if r.Font != nil {
					chars[idx].bold = r.Font.Bold
					chars[idx].italic = r.Font.Italic
					chars[idx].underline = r.Font.Underline == "single" || (r.Font.Underline != "" && r.Font.Underline != "none")
					if r.Font.Family != "" {
						chars[idx].family = r.Font.Family
					}
					if r.Font.Size > 0 {
						chars[idx].size = r.Font.Size
					}
					if r.Font.Color != "" {
						chars[idx].color = normalizeHexColor(r.Font.Color)
					}
				}
				idx++
			}
		}
	}

	// Apply patch to selection [startRune, endRune).
	for i := startRune; i < endRune; i++ {
		if patch.Bold != nil {
			chars[i].bold = *patch.Bold
		}
		if patch.Italic != nil {
			chars[i].italic = *patch.Italic
		}
		if patch.Underline != nil {
			chars[i].underline = *patch.Underline
		}
		if patch.FontFamily != nil && *patch.FontFamily != "" {
			chars[i].family = *patch.FontFamily
		}
		if patch.FontSize != nil && *patch.FontSize > 0 {
			chars[i].size = *patch.FontSize
		}
		if patch.FontColor != nil && *patch.FontColor != "" {
			chars[i].color = *patch.FontColor
		}
	}

	// Merge adjacent runes with identical format into rich text runs.
	var excelizeRuns []excelize.RichTextRun
	segStart := 0
	for segStart < len(runes) {
		segEnd := segStart + 1
		for segEnd < len(runes) && chars[segEnd] == chars[segStart] {
			segEnd++
		}
		prop := chars[segStart]
		u := ""
		if prop.underline {
			u = "single"
		}
		colorHex := strings.TrimPrefix(prop.color, "#")
		fnt := &excelize.Font{
			Bold:      prop.bold,
			Italic:    prop.italic,
			Underline: u,
			Family:    prop.family,
			Size:      prop.size,
			Color:     colorHex,
		}
		excelizeRuns = append(excelizeRuns, excelize.RichTextRun{
			Text: string(runes[segStart:segEnd]),
			Font: fnt,
		})
		segStart = segEnd
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return fmt.Errorf("capture rich text history: %w", err)
	}
	if err := w.file.SetCellRichText(sheet, cell, excelizeRuns); err != nil {
		return err
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	w.dirty = true
	return nil
}
