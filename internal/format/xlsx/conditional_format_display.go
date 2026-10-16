package xlsx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

type conditionalFormatDisplayCache struct {
	sheet string
	state uint64
	valid bool
	rules []conditionalFormatDisplayRule
}

type conditionalFormatDisplayRule struct {
	bounds                       document.CellRange
	typeOf                       document.SpreadsheetConditionalFormatType
	fillColor, fontColor         string
	barColor                     string
	minColor, midColor, maxColor string
	min, mid, max, average       float64
	hasNumbers                   bool
	duplicates                   map[string]int
	numbers                      []float64
	sum                          float64
	count                        int
}

func (w *Workbook) applyConditionalFormatDisplayLocked(sheet string, window *document.SheetWindow) {
	rules := w.conditionalFormatDisplayRulesLocked(sheet)
	if len(rules) == 0 || window == nil {
		return
	}
	for rowIndex := range window.Cells {
		row := window.StartRow + rowIndex
		for columnIndex := range window.Cells[rowIndex] {
			column := window.StartColumn + columnIndex
			w.applyConditionalFormatCellLocked(sheet, row, column, &window.Cells[rowIndex][columnIndex], rules)
		}
	}
	for index := range window.Merges {
		anchor := &window.Merges[index].Anchor
		row, column := window.Merges[index].Range.StartRow, window.Merges[index].Range.StartColumn
		w.applyConditionalFormatCellLocked(sheet, row, column, anchor, rules)
	}
}

func (w *Workbook) conditionalFormatDisplayRulesLocked(sheet string) []conditionalFormatDisplayRule {
	cache := &w.conditionalFormats
	if cache.valid && cache.sheet == sheet && cache.state == w.historyState {
		return cache.rules
	}
	cache.sheet, cache.state, cache.valid, cache.rules = sheet, w.historyState, true, nil
	formats, err := w.file.GetConditionalFormats(sheet)
	if err != nil {
		return nil
	}

	refs := make([]string, 0, len(formats))
	for ref := range formats {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		bounds := conditionalFormatRanges(ref)
		if len(bounds) == 0 {
			continue
		}
		for _, option := range formats[ref] {
			rule, ok := w.conditionalFormatDisplayRuleForLocked(option)
			if !ok {
				continue
			}
			for _, cellRange := range bounds {
				rule.bounds = cellRange
				cache.rules = append(cache.rules, rule)
			}
		}
	}
	if len(cache.rules) == 0 {
		return nil
	}
	w.collectConditionalFormatStatisticsLocked(sheet, cache.rules)
	return cache.rules
}

func (w *Workbook) conditionalFormatDisplayRuleForLocked(option excelize.ConditionalFormatOptions) (conditionalFormatDisplayRule, bool) {
	rule := conditionalFormatDisplayRule{}
	switch option.Type {
	case "data_bar":
		if (option.MinType != "" && option.MinType != "min") || (option.MaxType != "" && option.MaxType != "max") {
			return rule, false
		}
		rule.typeOf = document.ConditionalFormatDataBar
		rule.barColor = normalizeRGBColor(option.BarColor)
		if rule.barColor == "" {
			rule.barColor = "#638EC6"
		}
	case "3_color_scale":
		if (option.MinType != "" && option.MinType != "min") || (option.MaxType != "" && option.MaxType != "max") {
			return rule, false
		}
		rule.typeOf = document.ConditionalFormat3ColorScale
		rule.minColor = normalizedOr(option.MinColor, "#F8696B")
		rule.midColor = normalizedOr(option.MidColor, "#FFEB84")
		rule.maxColor = normalizedOr(option.MaxColor, "#63BE7B")
	case "duplicate":
		rule.typeOf = document.ConditionalFormatDuplicate
		rule.fillColor, rule.fontColor = "#FFC7CE", "#9C0006"
	case "average":
		if !option.AboveAverage {
			return rule, false
		}
		rule.typeOf = document.ConditionalFormatAboveAvg
		rule.fillColor, rule.fontColor = "#C6EFCE", "#006100"
	default:
		return rule, false
	}
	if option.Format != nil {
		if style, err := w.file.GetConditionalStyle(*option.Format); err == nil {
			neutral := neutralCellStyle(style)
			if neutral.FillColor != "" {
				rule.fillColor = neutral.FillColor
			}
			if neutral.FontColor != "" {
				rule.fontColor = neutral.FontColor
			}
		}
	}
	return rule, true
}

func normalizedOr(value, fallback string) string {
	if normalized := normalizeRGBColor(value); normalized != "" {
		return normalized
	}
	return fallback
}

func conditionalFormatRanges(ref string) []document.CellRange {
	var ranges []document.CellRange
	for _, item := range strings.Fields(ref) {
		if strings.Contains(item, "!") {
			parts := strings.Split(item, "!")
			item = parts[len(parts)-1]
		}
		item = strings.ReplaceAll(item, "$", "")
		parts := strings.Split(item, ":")
		if len(parts) == 0 || len(parts) > 2 {
			continue
		}
		startColumn, startRow, err := excelize.CellNameToCoordinates(parts[0])
		if err != nil {
			continue
		}
		endColumn, endRow := startColumn, startRow
		if len(parts) == 2 {
			endColumn, endRow, err = excelize.CellNameToCoordinates(parts[1])
			if err != nil {
				continue
			}
		}
		if endColumn < startColumn || endRow < startRow {
			continue
		}
		ranges = append(ranges, document.CellRange{StartRow: startRow, StartColumn: startColumn, EndRow: endRow, EndColumn: endColumn})
	}
	return ranges
}

func (w *Workbook) collectConditionalFormatStatisticsLocked(sheet string, rules []conditionalFormatDisplayRule) {
	worksheetXML, err := w.worksheetXMLLocked(sheet)
	if err != nil {
		return
	}
	decoder := xml.NewDecoder(bytes.NewReader(worksheetXML))
	for {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			if tokenErr == io.EOF {
				break
			}
			return
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "c" {
			continue
		}
		var cell worksheetNavigationCell
		if decoder.DecodeElement(&cell, &start) != nil || !cell.occupied() {
			continue
		}
		column, row, coordinateErr := excelize.CellNameToCoordinates(cell.Reference)
		if coordinateErr != nil {
			continue
		}
		matched := make([]int, 0, 2)
		for index := range rules {
			if cellRangeContains(rules[index].bounds, row, column) {
				matched = append(matched, index)
			}
		}
		if len(matched) == 0 {
			continue
		}
		value, cellType, ok := w.conditionalFormatRawValueLocked(sheet, cell.Reference)
		if !ok || value == "" {
			continue
		}
		number, numeric := conditionalFormatNumber(value, cellType)
		for _, index := range matched {
			rule := &rules[index]
			if rule.typeOf == document.ConditionalFormatDuplicate {
				if rule.duplicates == nil {
					rule.duplicates = make(map[string]int)
				}
				key := conditionalFormatDuplicateKey(value, cellType, number, numeric)
				if key != "" {
					rule.duplicates[key]++
				}
			}
			if numeric && (rule.typeOf == document.ConditionalFormatDataBar || rule.typeOf == document.ConditionalFormat3ColorScale || rule.typeOf == document.ConditionalFormatAboveAvg) {
				rule.numbers = append(rule.numbers, number)
				rule.sum += number
				rule.count++
			}
		}
	}
	for index := range rules {
		rule := &rules[index]
		if rule.count > 0 {
			sort.Float64s(rule.numbers)
			rule.min, rule.max = rule.numbers[0], rule.numbers[len(rule.numbers)-1]
			rule.mid = conditionalFormatPercentile(rule.numbers, 0.5)
			rule.average = rule.sum / float64(rule.count)
			rule.hasNumbers = true
			rule.numbers = nil
		}
		for key, count := range rule.duplicates {
			if count < 2 {
				delete(rule.duplicates, key)
			}
		}
	}
}

func (w *Workbook) conditionalFormatRawValueLocked(sheet, cellRef string) (string, excelize.CellType, bool) {
	value, err := w.file.GetCellValue(sheet, cellRef, excelize.Options{RawCellValue: true})
	if err != nil {
		return "", excelize.CellTypeUnset, false
	}
	formula, _ := w.file.GetCellFormula(sheet, cellRef)
	if formula != "" {
		if calculated, calcErr := w.file.CalcCellValue(sheet, cellRef, excelize.Options{RawCellValue: true}); calcErr == nil && calculated != "" {
			value = calculated
		}
	}
	cellType, _ := w.file.GetCellType(sheet, cellRef)
	return value, cellType, true
}

func conditionalFormatNumber(value string, cellType excelize.CellType) (float64, bool) {
	if cellType != excelize.CellTypeNumber && cellType != excelize.CellTypeDate && cellType != excelize.CellTypeUnset {
		return 0, false
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func conditionalFormatDuplicateKey(value string, cellType excelize.CellType, number float64, numeric bool) string {
	if numeric {
		return "n:" + strconv.FormatFloat(number, 'g', 15, 64)
	}
	if value == "" {
		return ""
	}
	if cellType == excelize.CellTypeBool {
		return "b:" + strings.ToLower(value)
	}
	return "s:" + strings.ToUpper(value)
}

func conditionalFormatPercentile(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if percentile <= 0 {
		return values[0]
	}
	if percentile >= 1 {
		return values[len(values)-1]
	}
	position := percentile * float64(len(values)-1)
	low := int(position)
	high := min(low+1, len(values)-1)
	return values[low] + (values[high]-values[low])*(position-float64(low))
}

func (w *Workbook) applyConditionalFormatCellLocked(sheet string, row, column int, cell *document.Cell, rules []conditionalFormatDisplayRule) {
	if cell == nil || row <= 0 || column <= 0 {
		return
	}
	matched := make([]*conditionalFormatDisplayRule, 0, 2)
	for index := range rules {
		if cellRangeContains(rules[index].bounds, row, column) {
			matched = append(matched, &rules[index])
		}
	}
	if len(matched) == 0 {
		return
	}
	cellRef := CellName(column, row)
	value, cellType, ok := w.conditionalFormatRawValueLocked(sheet, cellRef)
	if !ok || value == "" {
		return
	}
	number, numeric := conditionalFormatNumber(value, cellType)
	var display document.CellConditionalDisplay
	for _, rule := range matched {
		switch rule.typeOf {
		case document.ConditionalFormatDataBar:
			if !numeric || !rule.hasNumbers {
				continue
			}
			display.HasDataBar = true
			display.DataBarColor = rule.barColor
			display.DataBarStart, display.DataBarEnd = conditionalFormatDataBarRange(number, rule.min, rule.max)
		case document.ConditionalFormat3ColorScale:
			if numeric && rule.hasNumbers {
				display.FillColor = conditionalFormatScaleColor(number, rule.min, rule.mid, rule.max, rule.minColor, rule.midColor, rule.maxColor)
			}
		case document.ConditionalFormatDuplicate:
			key := conditionalFormatDuplicateKey(value, cellType, number, numeric)
			if rule.duplicates[key] > 1 {
				display.FillColor, display.FontColor = rule.fillColor, rule.fontColor
			}
		case document.ConditionalFormatAboveAvg:
			if numeric && rule.hasNumbers && number > rule.average {
				display.FillColor, display.FontColor = rule.fillColor, rule.fontColor
			}
		}
	}
	cell.ConditionalDisplay = display
}

func conditionalFormatDataBarRange(value, minimum, maximum float64) (float64, float64) {
	if maximum == minimum {
		return 0, 1
	}
	normalized := (value - minimum) / (maximum - minimum)
	normalized = min(1, max(0, normalized))
	if minimum < 0 && maximum > 0 {
		zero := -minimum / (maximum - minimum)
		if value < 0 {
			return normalized, zero
		}
		return zero, normalized
	}
	if maximum <= 0 {
		return normalized, 1
	}
	return 0, normalized
}

func conditionalFormatScaleColor(value, minimum, midpoint, maximum float64, minColor, midColor, maxColor string) string {
	if minimum == maximum {
		return midColor
	}
	if value <= midpoint {
		position := 1.0
		if midpoint > minimum {
			position = (value - minimum) / (midpoint - minimum)
		}
		return interpolateConditionalFormatColor(minColor, midColor, position)
	}
	position := 1.0
	if maximum > midpoint {
		position = (value - midpoint) / (maximum - midpoint)
	}
	return interpolateConditionalFormatColor(midColor, maxColor, position)
}

func interpolateConditionalFormatColor(from, to string, position float64) string {
	fromRGB, fromOK := parseConditionalFormatRGB(from)
	toRGB, toOK := parseConditionalFormatRGB(to)
	if !fromOK || !toOK {
		return from
	}
	position = min(1, max(0, position))
	channels := [3]uint8{}
	for index := range channels {
		channels[index] = uint8(math.Round(float64(fromRGB[index]) + (float64(toRGB[index])-float64(fromRGB[index]))*position))
	}
	return fmt.Sprintf("#%02X%02X%02X", channels[0], channels[1], channels[2])
}

func parseConditionalFormatRGB(value string) ([3]uint8, bool) {
	var channels [3]uint8
	value = strings.TrimPrefix(normalizeRGBColor(value), "#")
	if len(value) != 6 {
		return channels, false
	}
	parsed, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return channels, false
	}
	channels[0] = uint8(parsed >> 16)
	channels[1] = uint8(parsed >> 8)
	channels[2] = uint8(parsed)
	return channels, true
}

func cellRangeContains(bounds document.CellRange, row, column int) bool {
	return row >= bounds.StartRow && row <= bounds.EndRow && column >= bounds.StartColumn && column <= bounds.EndColumn
}
