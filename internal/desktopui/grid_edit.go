package desktopui

import (
	"errors"
	"strconv"
	"strings"

	"glance/internal/document"
)

var errNoSelectedCell = errors.New("no spreadsheet cell is selected")

type cellEditState struct {
	Address  string
	Original string
	Draft    string
	Editing  bool
	session  *sharedDocumentSession
}

func (s *cellEditState) selectCell(address, value string) {
	s.setEditing(false)
	s.Address, s.Original, s.Draft = address, value, value
}
func (s *cellEditState) begin() bool {
	if s.Address == "" {
		return false
	}
	s.Draft = s.Original
	s.setEditing(true)
	return true
}
func (s *cellEditState) setDraft(value string) { s.Draft = value }
func (s *cellEditState) commit(apply func(address, value string) error) error {
	if s.Address == "" {
		return errNoSelectedCell
	}
	if s.Draft == s.Original {
		s.setEditing(false)
		return nil
	}
	if err := apply(s.Address, s.Draft); err != nil {
		return err
	}
	s.Original = s.Draft
	s.setEditing(false)
	return nil
}
func (s *cellEditState) cancel() {
	s.Draft = s.Original
	s.setEditing(false)
}

func (s *cellEditState) setEditing(editing bool) {
	if s.Editing == editing {
		return
	}
	s.Editing = editing
	if s.session == nil {
		return
	}
	if editing {
		s.session.spreadsheetDraftViews.Add(1)
	} else {
		s.session.spreadsheetDraftViews.Add(-1)
	}
}

func parseCellAddress(address string) (row, column int, ok bool) {
	a := strings.ToUpper(strings.TrimSpace(address))
	i := 0
	for i < len(a) && a[i] >= 'A' && a[i] <= 'Z' {
		column = column*26 + int(a[i]-'A'+1)
		i++
	}
	if i == 0 || i == len(a) {
		return 0, 0, false
	}
	r, err := strconv.Atoi(a[i:])
	if err != nil || r < 1 || column < 1 {
		return 0, 0, false
	}
	return r, column, true
}

// parseCellRangeAddress parses an A1 cell reference or a rectangular range.
// It accepts Excel's absolute-reference markers and keeps the typed endpoint
// order so the first endpoint can remain the active cell after Go To.
func parseCellRangeAddress(reference string) (start, end CellPosition, ok bool) {
	parts := strings.Split(strings.TrimSpace(reference), ":")
	if len(parts) < 1 || len(parts) > 2 {
		return CellPosition{}, CellPosition{}, false
	}
	parseEndpoint := func(value string) (CellPosition, bool) {
		value = strings.TrimSpace(value)
		if value == "" {
			return CellPosition{}, false
		}
		if strings.HasPrefix(value, "$") {
			value = value[1:]
		}
		letters := 0
		for letters < len(value) && ((value[letters] >= 'A' && value[letters] <= 'Z') || (value[letters] >= 'a' && value[letters] <= 'z')) {
			letters++
		}
		if letters == 0 {
			return CellPosition{}, false
		}
		rowPart := value[letters:]
		if strings.HasPrefix(rowPart, "$") {
			rowPart = rowPart[1:]
		}
		if strings.Contains(value[:letters], "$") || strings.Contains(rowPart, "$") {
			return CellPosition{}, false
		}
		row, column, valid := parseCellAddress(value[:letters] + rowPart)
		if !valid || row > xlsxRows || column > xlsxColumns {
			return CellPosition{}, false
		}
		return CellPosition{Row: row - 1, Column: column - 1}, true
	}

	start, ok = parseEndpoint(parts[0])
	if !ok {
		return CellPosition{}, CellPosition{}, false
	}
	end = start
	if len(parts) == 2 {
		end, ok = parseEndpoint(parts[1])
		if !ok {
			return CellPosition{}, CellPosition{}, false
		}
	}
	return start, end, true
}

func parseSpreadsheetGoToReference(reference, currentSheet string, names []document.SpreadsheetDefinedName) (sheet string, start, end CellPosition, ok bool) {
	value := strings.TrimSpace(reference)
	qualifiedSheet, cellReference, qualified, valid := splitSpreadsheetSheetReference(value)
	if !valid {
		return "", CellPosition{}, CellPosition{}, false
	}
	if qualified {
		start, end, ok = parseCellRangeAddress(cellReference)
		if !ok {
			return "", CellPosition{}, CellPosition{}, false
		}
		return qualifiedSheet, start, end, true
	}
	if start, end, ok = parseCellRangeAddress(value); ok {
		return currentSheet, start, end, true
	}

	var localMatch, workbookMatch *document.SpreadsheetDefinedName
	for index := range names {
		name := &names[index]
		if !strings.EqualFold(strings.TrimSpace(name.Name), value) {
			continue
		}
		scope := strings.TrimSpace(name.Scope)
		if scope == "" || strings.EqualFold(scope, "Workbook") {
			if workbookMatch == nil {
				workbookMatch = name
			}
			continue
		}
		if strings.EqualFold(scope, currentSheet) && localMatch == nil {
			localMatch = name
		}
	}
	selected := localMatch
	if selected == nil {
		selected = workbookMatch
	}
	if selected == nil {
		return "", CellPosition{}, CellPosition{}, false
	}
	nameReference := strings.TrimSpace(selected.Ref)
	nameReference = strings.TrimPrefix(nameReference, "=")
	qualifiedSheet, cellReference, qualified, valid = splitSpreadsheetSheetReference(nameReference)
	if !valid {
		return "", CellPosition{}, CellPosition{}, false
	}
	start, end, ok = parseCellRangeAddress(cellReference)
	if !ok {
		return "", CellPosition{}, CellPosition{}, false
	}
	if qualified {
		return qualifiedSheet, start, end, true
	}
	return currentSheet, start, end, true
}

// splitSpreadsheetSheetReference distinguishes a direct A1-style reference
// from a sheet-qualified reference while respecting Excel's quoted sheet names
// and doubled apostrophes.
func splitSpreadsheetSheetReference(reference string) (sheet, cellReference string, qualified, ok bool) {
	value := strings.TrimSpace(reference)
	if !strings.Contains(value, "!") {
		return "", value, false, true
	}
	if strings.HasPrefix(value, "'") {
		for index := 1; index < len(value); index++ {
			if value[index] != '\'' {
				continue
			}
			if index+1 < len(value) && value[index+1] == '\'' {
				index++
				continue
			}
			if index+1 >= len(value) || value[index+1] != '!' {
				return "", "", true, false
			}
			sheet = strings.ReplaceAll(value[1:index], "''", "'")
			cellReference = strings.TrimSpace(value[index+2:])
			return sheet, cellReference, true, sheet != "" && cellReference != ""
		}
		return "", "", true, false
	}
	separator := strings.IndexByte(value, '!')
	if separator <= 0 || separator == len(value)-1 || strings.Contains(value[separator+1:], "!") || strings.Contains(value[:separator], "'") {
		return "", "", true, false
	}
	sheet = strings.TrimSpace(value[:separator])
	cellReference = strings.TrimSpace(value[separator+1:])
	return sheet, cellReference, true, sheet != "" && cellReference != ""
}
