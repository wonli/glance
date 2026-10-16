package xlsx

import (
	"strings"

	"github.com/xuri/excelize/v2"
)

const maxExcelDecimalPlaces = 30

// adjustDecimalPlaces keeps the existing number-format category and changes
// only its fractional precision. Unknown built-ins and date/time formats are
// left untouched rather than being converted to a potentially misleading
// numeric format.
func adjustDecimalPlaces(style *excelize.Style, delta int) {
	if style == nil || (delta != -1 && delta != 1) {
		return
	}
	format := ""
	if style.CustomNumFmt != nil {
		format = *style.CustomNumFmt
	} else if builtin, ok := adjustableBuiltinNumberFormats[style.NumFmt]; ok {
		format = builtin
	} else {
		return
	}
	updated, changed := adjustNumberFormatCode(format, delta)
	if !changed {
		return
	}
	style.CustomNumFmt = &updated
	style.NumFmt = 0
	style.DecimalPlaces = nil
}

var adjustableBuiltinNumberFormats = map[int]string{
	0:  "General",
	1:  "0",
	2:  "0.00",
	3:  "#,##0",
	4:  "#,##0.00",
	9:  "0%",
	10: "0.00%",
	11: "0.00E+00",
	12: "# ?/?",
	13: "# ??/??",
	37: "#,##0 ;(#,##0)",
	38: "#,##0 ;[Red](#,##0)",
	39: "#,##0.00 ;(#,##0.00)",
	40: "#,##0.00 ;[Red](#,##0.00)",
	41: `_(* #,##0_);_(* \(#,##0\);_(* "-"_);_(@_)`,
	42: `_("$"* #,##0_);_("$"* \(#,##0\);_("$"* "-"_);_(@_)`,
	43: `_(* #,##0.00_);_(* \(#,##0.00\);_(* "-"??_);_(@_)`,
	44: `_("$"* #,##0.00_);_("$"* \(#,##0.00\);_("$"* "-"??_);_(@_)`,
	48: "##0.0E+0",
}

func adjustNumberFormatCode(format string, delta int) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(format), "general") {
		if delta > 0 {
			return "0.0", true
		}
		return format, false
	}
	sections := splitNumberFormatSections(format)
	changed := false
	for index, section := range sections {
		updated, sectionChanged := adjustNumberFormatSection(section, delta)
		if sectionChanged {
			sections[index] = updated
			changed = true
		}
	}
	if !changed {
		return format, false
	}
	return strings.Join(sections, ";"), true
}

func splitNumberFormatSections(format string) []string {
	sections := make([]string, 0, 4)
	start := 0
	for index := 0; index < len(format); {
		switch format[index] {
		case '\\', '_', '*':
			index += 2
		case '"':
			index = skipNumberFormatQuoted(format, index)
		case '[':
			index = skipNumberFormatBracket(format, index)
		case ';':
			sections = append(sections, format[start:index])
			start = index + 1
			index++
		default:
			index++
		}
	}
	return append(sections, format[start:])
}

func skipNumberFormatQuoted(format string, start int) int {
	for index := start + 1; index < len(format); index++ {
		if format[index] != '"' {
			continue
		}
		if index+1 < len(format) && format[index+1] == '"' {
			index++
			continue
		}
		return index + 1
	}
	return len(format)
}

func skipNumberFormatBracket(format string, start int) int {
	if end := strings.IndexByte(format[start+1:], ']'); end >= 0 {
		return start + end + 2
	}
	return len(format)
}

func adjustNumberFormatSection(section string, delta int) (string, bool) {
	placeholders := make([]int, 0, 8)
	decimalPoints := make([]int, 0, 1)
	hasFractionSlash := false
	for index := 0; index < len(section); {
		switch section[index] {
		case '\\', '_', '*':
			index += 2
		case '"':
			index = skipNumberFormatQuoted(section, index)
		case '[':
			index = skipNumberFormatBracket(section, index)
		case '0', '#', '?':
			placeholders = append(placeholders, index)
			index++
		case '.':
			decimalPoints = append(decimalPoints, index)
			index++
		case '/':
			hasFractionSlash = true
			index++
		default:
			index++
		}
	}
	if len(placeholders) == 0 || hasFractionSlash {
		return section, false
	}

	exponent := len(section)
	for index := 0; index < len(section); {
		switch section[index] {
		case '\\', '_', '*':
			index += 2
		case '"':
			index = skipNumberFormatQuoted(section, index)
		case '[':
			index = skipNumberFormatBracket(section, index)
		default:
			if section[index] == 'E' || section[index] == 'e' {
				exponent = index
				index = len(section)
			} else {
				index++
			}
		}
	}

	point := -1
	for _, candidate := range decimalPoints {
		if candidate >= exponent {
			continue
		}
		hasInteger := false
		hasFraction := false
		for _, position := range placeholders {
			if position < candidate {
				hasInteger = true
			}
			if position > candidate && position < exponent {
				hasFraction = true
			}
		}
		if hasInteger && hasFraction {
			point = candidate
			break
		}
	}

	if point >= 0 {
		fractionEnd := point + 1
		for fractionEnd < exponent && isNumberPlaceholder(section[fractionEnd]) {
			fractionEnd++
		}
		places := fractionEnd - point - 1
		if delta > 0 {
			if places >= maxExcelDecimalPlaces {
				return section, false
			}
			return section[:fractionEnd] + "0" + section[fractionEnd:], true
		}
		if places == 0 {
			return section, false
		}
		if places == 1 {
			return section[:point] + section[fractionEnd:], true
		}
		return section[:fractionEnd-1] + section[fractionEnd:], true
	}
	if delta < 0 {
		return section, false
	}
	lastIntegerPlaceholder := -1
	for _, position := range placeholders {
		if position < exponent {
			lastIntegerPlaceholder = position
		}
	}
	if lastIntegerPlaceholder < 0 {
		return section, false
	}
	insertAt := lastIntegerPlaceholder + 1
	return section[:insertAt] + ".0" + section[insertAt:], true
}

func isNumberPlaceholder(value byte) bool {
	return value == '0' || value == '#' || value == '?'
}
