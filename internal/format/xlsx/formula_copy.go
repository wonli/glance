package xlsx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/xuri/efp"
	"github.com/xuri/excelize/v2"
)

var (
	clipboardCellRef = regexp.MustCompile("^(\\$?)([A-Za-z]{1,3})(\\$?)([0-9]+)$")
	clipboardColumn  = regexp.MustCompile("^(\\$?)([A-Za-z]{1,3})$")
	clipboardRow     = regexp.MustCompile("^(\\$?)([0-9]+)$")
)

// translateClipboardFormula adjusts A1 references by the source-to-target
// offset while preserving absolute row and column markers. Named references
// and structured references are left untouched.
func translateClipboardFormula(formula string, rowDelta, columnDelta int, definedNames map[string]bool) (string, error) {
	parser := efp.ExcelParser()
	tokens := parser.Parse(formula)
	if len(tokens) == 0 {
		return "", fmt.Errorf("cannot parse copied formula")
	}
	var out strings.Builder
	for _, token := range tokens {
		if token.TType == efp.TokenTypeUnknown {
			return "", fmt.Errorf("formula contains a reference or operator that cannot be adjusted safely")
		}
		if token.TType == efp.TokenTypeOperand && token.TSubType == efp.TokenSubTypeRange {
			if definedNames[strings.ToLower(token.TValue)] || strings.Contains(token.TValue, "[") {
				out.WriteString(token.TValue)
				continue
			}
			value, changed, err := translateClipboardFormulaRange(token.TValue, rowDelta, columnDelta)
			if err != nil {
				return "", err
			}
			if changed {
				out.WriteString(value)
			} else {
				out.WriteString(token.TValue)
			}
			continue
		}
		if token.TType == efp.TokenTypeOperand && token.TSubType == efp.TokenSubTypeText {
			out.WriteByte('"')
			out.WriteString(strings.ReplaceAll(token.TValue, "\"", "\"\""))
			out.WriteByte('"')
			continue
		}
		if token.TType == efp.TokenTypeFunction && token.TSubType == efp.TokenSubTypeStart {
			out.WriteString(token.TValue)
			out.WriteRune(efp.ParenOpen)
			continue
		}
		if token.TType == efp.TokenTypeFunction && token.TSubType == efp.TokenSubTypeStop {
			out.WriteRune(efp.ParenClose)
			continue
		}
		if token.TType == efp.TokenTypeSubexpression && token.TSubType == efp.TokenSubTypeStart {
			out.WriteString(token.TValue)
			out.WriteRune(efp.ParenOpen)
			continue
		}
		if token.TType == efp.TokenTypeSubexpression && token.TSubType == efp.TokenSubTypeStop {
			out.WriteRune(efp.ParenClose)
			continue
		}
		out.WriteString(token.TValue)
	}
	return out.String(), nil
}

func translateClipboardFormulaRange(value string, rowDelta, columnDelta int) (string, bool, error) {
	if strings.Contains(value, "#REF!") {
		return value, false, nil
	}
	separator := clipboardRangeSeparator(value)
	if separator < 0 {
		adjusted, changed, recognized, err := translateClipboardEndpoint(value, rowDelta, columnDelta, false)
		if err != nil || !recognized {
			return value, false, err
		}
		return adjusted, changed, nil
	}
	left, right := value[:separator], value[separator+1:]
	leftAdjusted, leftChanged, leftKind, err := translateClipboardEndpoint(left, rowDelta, columnDelta, true)
	if err != nil {
		return "", false, err
	}
	rightAdjusted, rightChanged, rightKind, err := translateClipboardEndpoint(right, rowDelta, columnDelta, true)
	if err != nil {
		return "", false, err
	}
	if !leftKind || !rightKind {
		return value, false, nil
	}
	return leftAdjusted + ":" + rightAdjusted, leftChanged || rightChanged, nil
}

// clipboardRangeSeparator finds the range colon after any sheet qualifier.
// Colons inside a quoted sheet name or a 3-D sheet qualifier are not separators.
func clipboardRangeSeparator(value string) int {
	quoted := false
	lastBang, colon := -1, -1
	for i := 0; i < len(value); i++ {
		if value[i] == '\'' {
			if quoted && i+1 < len(value) && value[i+1] == '\'' {
				i++
				continue
			}
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		if value[i] == '!' {
			lastBang = i
			colon = -1
		} else if value[i] == ':' {
			colon = i
		}
	}
	if colon <= lastBang {
		return -1
	}
	return colon
}

func translateClipboardEndpoint(value string, rowDelta, columnDelta int, allowWholeRowOrColumn bool) (string, bool, bool, error) {
	prefix, ref := splitClipboardSheetPrefix(value)
	if strings.Contains(prefix, "[") {
		return value, false, false, nil
	}
	if match := clipboardCellRef.FindStringSubmatch(ref); match != nil {
		column, err := excelize.ColumnNameToNumber(match[2])
		if err != nil {
			return "", false, true, err
		}
		row, err := strconv.Atoi(match[4])
		if err != nil {
			return "", false, true, err
		}
		if match[1] == "" {
			column += columnDelta
		}
		if match[3] == "" {
			row += rowDelta
		}
		if row < 1 || row > maxXLSXRows || column < 1 || column > maxXLSXColumns {
			return "", false, true, fmt.Errorf("copied formula reference %s would move outside the worksheet", value)
		}
		columnName, err := excelize.ColumnNumberToName(column)
		if err != nil {
			return "", false, true, err
		}
		return escapeClipboardSheetPrefix(prefix) + match[1] + columnName + match[3] + strconv.Itoa(row), true, true, nil
	}
	if allowWholeRowOrColumn {
		if match := clipboardColumn.FindStringSubmatch(ref); match != nil {
			column, err := excelize.ColumnNameToNumber(match[2])
			if err != nil {
				return "", false, true, err
			}
			if match[1] == "" {
				column += columnDelta
			}
			if column < 1 || column > maxXLSXColumns {
				return "", false, true, fmt.Errorf("copied formula reference %s would move outside the worksheet", value)
			}
			name, err := excelize.ColumnNumberToName(column)
			if err != nil {
				return "", false, true, err
			}
			return escapeClipboardSheetPrefix(prefix) + match[1] + name, true, true, nil
		}
		if match := clipboardRow.FindStringSubmatch(ref); match != nil {
			row, err := strconv.Atoi(match[2])
			if err != nil {
				return "", false, true, err
			}
			if match[1] == "" {
				row += rowDelta
			}
			if row < 1 || row > maxXLSXRows {
				return "", false, true, fmt.Errorf("copied formula reference %s would move outside the worksheet", value)
			}
			return escapeClipboardSheetPrefix(prefix) + match[1] + strconv.Itoa(row), true, true, nil
		}
	}
	if strings.ContainsAny(ref, "$!0123456789:") {
		return "", false, false, fmt.Errorf("copied formula reference %s cannot be adjusted safely", value)
	}
	return value, false, false, nil
}

func splitClipboardSheetPrefix(value string) (string, string) {
	quoted := false
	lastBang := -1
	for i := 0; i < len(value); i++ {
		if value[i] == '\'' {
			if quoted && i+1 < len(value) && value[i+1] == '\'' {
				i++
				continue
			}
			quoted = !quoted
		} else if value[i] == '!' && !quoted {
			lastBang = i
		}
	}
	if lastBang < 0 {
		return "", value
	}
	return value[:lastBang+1], value[lastBang+1:]
}

func escapeClipboardSheetPrefix(prefix string) string {
	if prefix == "" {
		return prefix
	}
	name := strings.TrimSuffix(prefix, "!")
	if strings.HasPrefix(name, "'") && strings.HasSuffix(name, "'") {
		return prefix
	}
	if strings.Contains(name, ":") {
		return prefix
	}
	if strings.IndexFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) < 0 {
		return prefix
	}
	return "'" + strings.ReplaceAll(name, "'", "''") + "'!"
}
