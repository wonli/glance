package desktopui

import (
	"regexp"
	"strings"

	"glance/internal/document"
)

var (
	formulaCellReferenceAtStart = regexp.MustCompile(`(?i)^\$?[A-Z]{1,3}\$?[1-9][0-9]*`)
	formulaSheetQualifierAtEnd  = regexp.MustCompile(`(?i)('([^']|'')+'|[A-Z_][A-Z0-9_.]*)\s*!\s*$`)
)

func formulaDependencyRanges(formula, currentSheet string) []document.CellRange {
	if formula == "" || currentSheet == "" {
		return nil
	}

	ranges := make([]document.CellRange, 0, 2)
	seen := make(map[document.CellRange]struct{})
	addRange := func(cellRange document.CellRange) {
		if _, exists := seen[cellRange]; exists {
			return
		}
		seen[cellRange] = struct{}{}
		ranges = append(ranges, cellRange)
	}
	for i := 0; i < len(formula); {
		if formula[i] == '"' {
			i = skipFormulaString(formula, i)
			continue
		}
		if formula[i] == '\'' {
			if sheet, end, ok := quotedFormulaSheet(formula, i); ok {
				afterSheet := skipFormulaSpaces(formula, end)
				if afterSheet < len(formula) && formula[afterSheet] == '!' {
					refStart := skipFormulaSpaces(formula, afterSheet+1)
					if cellRange, refEnd, ok := parseFormulaReferenceRange(formula, refStart); ok {
						if strings.EqualFold(sheet, currentSheet) {
							addRange(cellRange)
						}
						i = refEnd
						continue
					}
				}
				i = end
				continue
			}
		}
		if !isFormulaReferenceStart(formula, i) || (i > 0 && isFormulaIdentifierByte(formula[i-1])) {
			i++
			continue
		}

		cellRange, end, ok := parseFormulaReferenceRange(formula, i)
		if !ok {
			i++
			continue
		}
		afterReference := skipFormulaSpaces(formula, end)
		if hasFormulaTokenBoundary(formula, i, end) || afterReference < len(formula) && formula[afterReference] == '(' {
			i = end
			continue
		}
		if afterReference < len(formula) && formula[afterReference] == '!' {
			refStart := skipFormulaSpaces(formula, afterReference+1)
			if referenced, refEnd, ok := parseFormulaReferenceRange(formula, refStart); ok {
				if strings.EqualFold(strings.Trim(formula[i:end], "$"), currentSheet) {
					addRange(referenced)
				}
				i = refEnd
				continue
			}
		}
		if sheet, qualified := formulaSheetQualifier(formula[:i]); !qualified || strings.EqualFold(sheet, currentSheet) {
			addRange(cellRange)
		}
		i = end
	}
	return ranges
}

func isFormulaReferenceStart(formula string, i int) bool {
	return i < len(formula) && (formula[i] == '$' || formula[i] >= 'A' && formula[i] <= 'Z' || formula[i] >= 'a' && formula[i] <= 'z')
}

func isFormulaIdentifierByte(b byte) bool {
	return b == '$' || b == '_' || b == '.' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func hasFormulaTokenBoundary(formula string, start, end int) bool {
	if start > 0 && (isFormulaIdentifierByte(formula[start-1]) || formula[start-1] == '[') {
		return true
	}
	return end < len(formula) && (isFormulaIdentifierByte(formula[end]) || formula[end] == ']')
}

func parseFormulaReferenceRange(formula string, start int) (document.CellRange, int, bool) {
	first, end, ok := parseFormulaCellReference(formula, start)
	if !ok {
		return document.CellRange{}, start, false
	}
	last := first
	colon := skipFormulaSpaces(formula, end)
	if colon < len(formula) && formula[colon] == ':' {
		secondStart := skipFormulaSpaces(formula, colon+1)
		if second, secondEnd, valid := parseFormulaCellReference(formula, secondStart); valid {
			last = second
			end = secondEnd
		}
	}
	return document.CellRange{
		StartRow: min(first.Row, last.Row) + 1, StartColumn: min(first.Column, last.Column) + 1,
		EndRow: max(first.Row, last.Row) + 1, EndColumn: max(first.Column, last.Column) + 1,
	}, end, true
}

func parseFormulaCellReference(formula string, start int) (CellPosition, int, bool) {
	if start < 0 || start >= len(formula) {
		return CellPosition{}, start, false
	}
	match := formulaCellReferenceAtStart.FindString(formula[start:])
	if match == "" {
		return CellPosition{}, start, false
	}
	address := strings.ReplaceAll(match, "$", "")
	row, column, ok := parseCellAddress(address)
	if !ok || row > xlsxRows || column > xlsxColumns {
		return CellPosition{}, start, false
	}
	return CellPosition{Row: row - 1, Column: column - 1}, start + len(match), true
}

func quotedFormulaSheet(formula string, start int) (string, int, bool) {
	for i := start + 1; i < len(formula); i++ {
		if formula[i] != '\'' {
			continue
		}
		if i+1 < len(formula) && formula[i+1] == '\'' {
			i++
			continue
		}
		return strings.ReplaceAll(formula[start+1:i], "''", "'"), i + 1, true
	}
	return "", start, false
}

func formulaSheetQualifier(prefix string) (string, bool) {
	match := formulaSheetQualifierAtEnd.FindStringSubmatch(prefix)
	if len(match) == 0 {
		return "", false
	}
	sheet := match[1]
	if strings.HasPrefix(sheet, "'") && strings.HasSuffix(sheet, "'") {
		sheet = strings.TrimSuffix(strings.TrimPrefix(sheet, "'"), "'")
		sheet = strings.ReplaceAll(sheet, "''", "'")
	}
	return sheet, true
}

func skipFormulaString(formula string, start int) int {
	for i := start + 1; i < len(formula); i++ {
		if formula[i] != '"' {
			continue
		}
		if i+1 < len(formula) && formula[i+1] == '"' {
			i++
			continue
		}
		return i + 1
	}
	return len(formula)
}

func skipFormulaSpaces(formula string, i int) int {
	for i < len(formula) && (formula[i] == ' ' || formula[i] == '\t' || formula[i] == '\r' || formula[i] == '\n') {
		i++
	}
	return i
}
