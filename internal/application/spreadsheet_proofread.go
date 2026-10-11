package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"glance/internal/document"
)

const (
	maxProofreadCells           = 10_000
	maxProofreadTextBytes       = 8 << 20
	maxProofreadDictionaryItems = 5_000
	maxProofreadIssues          = 500
)

type SpreadsheetProofreadIssue struct {
	Cell     string `json:"cell"`
	Category string `json:"category"`
	Token    string `json:"token,omitempty"`
	Message  string `json:"message"`
}

type SpreadsheetProofreadResult struct {
	Sheet             string                      `json:"sheet"`
	Bounds            document.CellRange          `json:"bounds"`
	CellsScanned      int                         `json:"cells_scanned"`
	DictionaryChecked bool                        `json:"dictionary_checked"`
	Issues            []SpreadsheetProofreadIssue `json:"issues"`
	IssuesTruncated   bool                        `json:"issues_truncated"`
}

func registerSpreadsheetProofreadCapability(catalog *Catalog) error {
	registration := CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.text.proofread", Name: "检查工作表文本",
			Description: "对明确 XLSX 工作表和范围执行有界文本检查，报告重复 ASCII 空格与不可见格式字符。可选 dictionary 提供最多 5000 个仅含拉丁字母的词条；结果只表示词条未出现在调用方词典中，不断言它一定拼错。未提供词典时不会猜测拼写。范围最多 10000 个单元格，文本最多 8 MiB，最多返回 500 条问题。Ribbon 拼写检查调用相同能力但不提供词典。",
			Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"要检查的精确工作表名称。"},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false,"description":"要检查的 1 起始闭区间。最多 10000 个单元格。"},"dictionary":{"type":"array","maxItems":5000,"items":{"type":"string","minLength":1,"maxLength":128},"description":"可选调用方词典。每项必须是仅含拉丁字母的单词。只有不在此词典中的拉丁词才会被报告；这不是语言学意义上的拼写错误判定。"}},"required":["sheet","bounds"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"bounds":{"type":"object","properties":{"start_row":{"type":"integer"},"start_column":{"type":"integer"},"end_row":{"type":"integer"},"end_column":{"type":"integer"}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"cells_scanned":{"type":"integer","minimum":0,"maximum":10000},"dictionary_checked":{"type":"boolean"},"issues":{"type":"array","maxItems":500,"items":{"type":"object","properties":{"cell":{"type":"string"},"category":{"type":"string","enum":["repeated_whitespace","invisible_format_character","unknown_dictionary_word"]},"token":{"type":"string","maxLength":128},"message":{"type":"string","maxLength":256}},"required":["cell","category","message"],"additionalProperties":false}},"issues_truncated":{"type":"boolean"}},"required":["sheet","bounds","cells_scanned","dictionary_checked","issues","issues_truncated"],"additionalProperties":false}`),
		},
		Handler: runSpreadsheetProofread,
	}
	return catalog.Register(registration)
}

func runSpreadsheetProofread(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
	var input struct {
		Sheet      string             `json:"sheet"`
		Bounds     document.CellRange `json:"bounds"`
		Dictionary []string           `json:"dictionary"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return CapabilityResult{}, fmt.Errorf("decode spreadsheet.text.proofread: %w", err)
	}
	if strings.TrimSpace(input.Sheet) == "" || len(input.Dictionary) > maxProofreadDictionaryItems || !validCellRange(input.Bounds, maxProofreadCells) {
		return CapabilityResult{}, ErrInvalidCommandArguments
	}
	words := make(map[string]struct{}, len(input.Dictionary))
	for _, word := range input.Dictionary {
		if !validProofreadDictionaryWord(word) {
			return CapabilityResult{}, ErrInvalidCommandArguments
		}
		words[strings.ToLower(word)] = struct{}{}
	}
	reader, ok := capability.Adapter.(SpreadsheetRangeReaderCapability)
	if !ok {
		return CapabilityResult{}, ErrCapabilityUnsupported
	}
	rows, err := reader.ReadSpreadsheetRange(ctx, input.Sheet, input.Bounds)
	if err != nil {
		return CapabilityResult{}, err
	}
	result := SpreadsheetProofreadResult{
		Sheet: input.Sheet, Bounds: input.Bounds,
		DictionaryChecked: len(words) > 0,
		Issues:            []SpreadsheetProofreadIssue{},
	}
	var textBytes int
	for rowOffset, row := range rows {
		for columnOffset, cell := range row {
			if err := ctx.Err(); err != nil {
				return CapabilityResult{}, err
			}
			if rowOffset >= input.Bounds.EndRow-input.Bounds.StartRow+1 || columnOffset >= input.Bounds.EndColumn-input.Bounds.StartColumn+1 {
				continue
			}
			result.CellsScanned++
			textBytes += len(cell.Value)
			if textBytes > maxProofreadTextBytes {
				return CapabilityResult{}, fmt.Errorf("%w: spreadsheet proofread input exceeds %d bytes", ErrQueryResultTooLarge, maxProofreadTextBytes)
			}
			cellName := spreadsheetCellName(input.Bounds.StartRow+rowOffset, input.Bounds.StartColumn+columnOffset)
			addIssue := func(issue SpreadsheetProofreadIssue) {
				if len(result.Issues) < maxProofreadIssues {
					result.Issues = append(result.Issues, issue)
				} else {
					result.IssuesTruncated = true
				}
			}
			if strings.Contains(cell.Value, "  ") {
				addIssue(SpreadsheetProofreadIssue{Cell: cellName, Category: "repeated_whitespace", Message: "文本包含连续 ASCII 空格。"})
			}
			for _, char := range cell.Value {
				if unicode.Is(unicode.Cf, char) || unicode.IsControl(char) && char != '\n' && char != '\r' && char != '\t' {
					addIssue(SpreadsheetProofreadIssue{Cell: cellName, Category: "invisible_format_character", Message: "文本包含不可见格式或控制字符。"})
					break
				}
			}
			if len(words) > 0 {
				for _, token := range latinWordTokens(cell.Value) {
					if _, exists := words[strings.ToLower(token)]; !exists {
						addIssue(SpreadsheetProofreadIssue{Cell: cellName, Category: "unknown_dictionary_word", Token: token, Message: "词条不在调用方提供的词典中；请结合语境复核。"})
					}
				}
			}
		}
	}
	return capabilityJSONResult(result, false)
}

func validProofreadDictionaryWord(word string) bool {
	if word == "" || len([]rune(word)) > 128 || strings.TrimSpace(word) != word {
		return false
	}
	for _, char := range word {
		if !unicode.IsLetter(char) || !unicode.In(char, unicode.Latin) {
			return false
		}
	}
	return true
}

func latinWordTokens(text string) []string {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, char := range text {
		if unicode.IsLetter(char) && unicode.In(char, unicode.Latin) {
			current.WriteRune(char)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}
