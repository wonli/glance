package xlsx

import "strings"

func normalizeFormulaFunctionNames(formula string) string {
	var normalized strings.Builder
	normalized.Grow(len(formula))
	for i := 0; i < len(formula); {
		if formula[i] == '"' {
			start := i
			i = skipFormulaStringLiteral(formula, i)
			normalized.WriteString(formula[start:i])
			continue
		}
		if !isFormulaNameStart(formula[i]) {
			normalized.WriteByte(formula[i])
			i++
			continue
		}
		start := i
		for i < len(formula) && isFormulaNameByte(formula[i]) {
			i++
		}
		name := formula[start:i]
		next := i
		for next < len(formula) && (formula[next] == ' ' || formula[next] == '\t' || formula[next] == '\r' || formula[next] == '\n') {
			next++
		}
		if next < len(formula) && formula[next] == '(' {
			name = canonicalFormulaFunctionName(name)
		}
		normalized.WriteString(name)
	}
	return normalized.String()
}

func canonicalFormulaFunctionName(name string) string {
	const xlfnPrefix = "_xlfn."
	if strings.HasPrefix(strings.ToLower(name), xlfnPrefix) {
		return xlfnPrefix + strings.ToUpper(name[len(xlfnPrefix):])
	}
	return strings.ToUpper(name)
}

func isFormulaNameStart(b byte) bool {
	return b == '_' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func isFormulaNameByte(b byte) bool {
	return isFormulaNameStart(b) || b == '.' || b >= '0' && b <= '9'
}

func skipFormulaStringLiteral(formula string, start int) int {
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
