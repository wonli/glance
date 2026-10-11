package application

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"glance/internal/document"
)

type WordTextBlockReader interface {
	ReadWordBlockWindow(context.Context, int, int) ([]document.WordBlock, int, error)
}

type WordLanguageSetter interface {
	SetWordLanguage(context.Context, string, string) (bool, error)
}

func registerWordReviewCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.review.editor.analyze", Name: "执行 Word 文档轻量审阅",
				Description: "对指定 Word 文档的一段正文块运行两项本地启发式检查：同一可编辑文本运行内的重复中文标点（，，、。。、、、）和括号/中文引号数量不平衡。结果包含稳定 block_id、block_index、paragraph_id、cell_id 和 run_id，可分页读取。返回的 heuristic_score 只是每条发现扣 5 分、最低 60 分的旧版 Ribbon 展示分，不是质量或正确性评分。本能力不含词典拼写、语法或语义检查，也不会修改文档。",
				Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow, RequiresDocument: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"start_block":{"type":"integer","minimum":0,"maximum":1000000,"description":"正文块分页起点，包含该块。"},"max_blocks":{"type":"integer","minimum":1,"maximum":1000,"description":"本页扫描的正文块数量；省略时默认 100。"}},"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"start_block":{"type":"integer","minimum":0},"scanned_blocks":{"type":"integer","minimum":0},"total_blocks":{"type":"integer","minimum":0},"heuristic_score":{"type":"integer","minimum":60,"maximum":100},"characters_with_whitespace":{"type":"integer","minimum":0},"characters_without_whitespace":{"type":"integer","minimum":0},"paragraphs":{"type":"integer","minimum":0},"tables":{"type":"integer","minimum":0},"unsupported_blocks":{"type":"integer","minimum":0},"findings":{"type":"array","maxItems":500,"items":{"type":"object","properties":{"code":{"type":"string","enum":["repeated_punctuation","unbalanced_delimiters"]},"block_id":{"type":"string"},"block_index":{"type":"integer","minimum":0},"paragraph_id":{"type":"string"},"cell_id":{"type":"string"},"run_id":{"type":"string"},"detail":{"type":"string"}},"required":["code","block_id","block_index","paragraph_id","cell_id","run_id","detail"],"additionalProperties":false}},"findings_truncated":{"type":"boolean"},"has_more":{"type":"boolean"}},"required":["start_block","scanned_blocks","total_blocks","heuristic_score","characters_with_whitespace","characters_without_whitespace","paragraphs","tables","unsupported_blocks","findings","findings_truncated","has_more"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					StartBlock int `json:"start_block"`
					MaxBlocks  int `json:"max_blocks"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if input.StartBlock < 0 || input.StartBlock > MaxQueryOffset || input.MaxBlocks < 0 || input.MaxBlocks > MaxQueryItems {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				limit := input.MaxBlocks
				if limit == 0 {
					limit = 100
				}
				reader, ok := capability.Adapter.(WordTextBlockReader)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				blocks, total, err := reader.ReadWordBlockWindow(ctx, input.StartBlock, limit)
				if err != nil {
					return CapabilityResult{}, err
				}
				if input.StartBlock > total {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				result := newWordEditorReviewResult(input.StartBlock, len(blocks), total)
				for offset, block := range blocks {
					if err := ctx.Err(); err != nil {
						return CapabilityResult{}, err
					}
					blockIndex := input.StartBlock + offset
					if block.Table != nil {
						result.Tables++
						for _, row := range block.Table.Rows {
							for _, cell := range row.Cells {
								for _, paragraph := range cell.Paragraphs {
									result.Paragraphs++
									countWordParagraphText(&result.wordTextStatistics, paragraph)
									addWordEditorFindings(&result, block.ID, blockIndex, paragraph.ID, cell.ID, paragraph)
								}
							}
						}
						continue
					}
					if block.Paragraph != nil {
						result.Paragraphs++
						countWordParagraphText(&result.wordTextStatistics, *block.Paragraph)
						addWordEditorFindings(&result, block.ID, blockIndex, block.Paragraph.ID, "", *block.Paragraph)
					}
					if block.Unsupported != nil || block.Kind == document.WordUnsupportedBlock {
						result.UnsupportedBlocks++
					}
				}
				return capabilityJSONResult(result.output(), false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.text.statistics", Name: "统计 Word 文本",
				Description: "统计明确 Word 文档正文和表格中的可编辑文本运行，按 Unicode 字符计数；返回字符数（含/不含空白）、段落数和表格数。图片和无法提取的对象不计入字符；使用 start_block、max_blocks 分页，has_more 为 true 时继续读取。",
				Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow, RequiresDocument: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"start_block":{"type":"integer","minimum":0,"maximum":1000000},"max_blocks":{"type":"integer","minimum":1,"maximum":1000}},"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"start_block":{"type":"integer"},"scanned_blocks":{"type":"integer"},"total_blocks":{"type":"integer"},"characters_with_whitespace":{"type":"integer"},"characters_without_whitespace":{"type":"integer"},"paragraphs":{"type":"integer"},"tables":{"type":"integer"},"unsupported_blocks":{"type":"integer"},"has_more":{"type":"boolean"}},"required":["start_block","scanned_blocks","total_blocks","characters_with_whitespace","characters_without_whitespace","paragraphs","tables","unsupported_blocks","has_more"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					StartBlock int `json:"start_block"`
					MaxBlocks  int `json:"max_blocks"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if input.StartBlock < 0 || input.StartBlock > MaxQueryOffset || input.MaxBlocks < 0 || input.MaxBlocks > MaxQueryItems {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				limit := input.MaxBlocks
				if limit == 0 {
					limit = 100
				}
				reader, ok := capability.Adapter.(WordTextBlockReader)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				blocks, total, err := reader.ReadWordBlockWindow(ctx, input.StartBlock, limit)
				if err != nil {
					return CapabilityResult{}, err
				}
				if input.StartBlock > total {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				var stats wordTextStatistics
				for _, block := range blocks {
					if err := ctx.Err(); err != nil {
						return CapabilityResult{}, err
					}
					if block.Table != nil {
						stats.Tables++
						for _, row := range block.Table.Rows {
							for _, cell := range row.Cells {
								for _, paragraph := range cell.Paragraphs {
									stats.Paragraphs++
									countWordParagraphText(&stats, paragraph)
								}
							}
						}
						continue
					}
					if block.Paragraph != nil {
						stats.Paragraphs++
						countWordParagraphText(&stats, *block.Paragraph)
					}
					if block.Unsupported != nil || block.Kind == document.WordUnsupportedBlock {
						stats.UnsupportedBlocks++
					}
				}
				return capabilityJSONResult(struct {
					StartBlock                  int  `json:"start_block"`
					ScannedBlocks               int  `json:"scanned_blocks"`
					TotalBlocks                 int  `json:"total_blocks"`
					CharactersWithWhitespace    int  `json:"characters_with_whitespace"`
					CharactersWithoutWhitespace int  `json:"characters_without_whitespace"`
					Paragraphs                  int  `json:"paragraphs"`
					Tables                      int  `json:"tables"`
					UnsupportedBlocks           int  `json:"unsupported_blocks"`
					HasMore                     bool `json:"has_more"`
				}{input.StartBlock, len(blocks), total, stats.CharactersWithWhitespace, stats.CharactersWithoutWhitespace, stats.Paragraphs, stats.Tables, stats.UnsupportedBlocks, input.StartBlock+len(blocks) < total}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.proofing.language.set", Name: "设置 Word 校对语言",
				Description: "为指定可编辑文本运行设置校对语言；run_id 省略或为空时设置文档正文和表格中的全部文本运行。language_code 使用 zh-CN、en-US 等 BCP 47 语言标签；先查询目标运行并使用当前文档修订。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string"},"language_code":{"type":"string","minLength":2,"maxLength":35}},"required":["language_code"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"run_id":{"type":"string"},"language_code":{"type":"string"}},"required":["changed","language_code"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					RunID        string `json:"run_id"`
					LanguageCode string `json:"language_code"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				input.LanguageCode = strings.TrimSpace(input.LanguageCode)
				if !validWordLanguageCode(input.LanguageCode) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(WordLanguageSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetWordLanguage(ctx, input.RunID, input.LanguageCode)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed      bool   `json:"changed"`
					RunID        string `json:"run_id,omitempty"`
					LanguageCode string `json:"language_code"`
				}{changed, input.RunID, input.LanguageCode}, changed)
			},
		},
	}
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	return nil
}

type wordTextStatistics struct {
	CharactersWithWhitespace    int
	CharactersWithoutWhitespace int
	Paragraphs                  int
	Tables                      int
	UnsupportedBlocks           int
}

const maxWordEditorFindings = 500

type wordEditorReviewFinding struct {
	BlockID     string `json:"block_id"`
	Code        string `json:"code"`
	BlockIndex  int    `json:"block_index"`
	ParagraphID string `json:"paragraph_id"`
	CellID      string `json:"cell_id"`
	RunID       string `json:"run_id"`
	Detail      string `json:"detail"`
}

type wordEditorReviewResult struct {
	StartBlock int `json:"start_block"`
	Scanned    int `json:"scanned_blocks"`
	Total      int `json:"total_blocks"`
	Score      int `json:"heuristic_score"`
	wordTextStatistics
	Findings          []wordEditorReviewFinding `json:"findings"`
	FindingsTruncated bool                      `json:"findings_truncated"`
	HasMore           bool                      `json:"has_more"`
	findingCount      int
}

func newWordEditorReviewResult(startBlock, scanned, total int) wordEditorReviewResult {
	return wordEditorReviewResult{
		StartBlock: startBlock, Scanned: scanned, Total: total,
		Findings: make([]wordEditorReviewFinding, 0, min(maxWordEditorFindings, 16)),
		HasMore:  startBlock+scanned < total,
	}
}

func addWordEditorFindings(result *wordEditorReviewResult, blockID string, blockIndex int, paragraphID, cellID string, paragraph document.WordParagraph) {
	for _, inline := range paragraph.Inlines {
		if inline.Text == nil {
			continue
		}
		value := inline.Text.Text
		if strings.Contains(value, "，，") || strings.Contains(value, "。。") || strings.Contains(value, "、、") {
			appendWordEditorFinding(result, wordEditorReviewFinding{
				Code: "repeated_punctuation", BlockID: blockID, BlockIndex: blockIndex, ParagraphID: paragraphID,
				CellID: cellID, RunID: inline.Text.ID, Detail: "文本运行中包含连续重复的中文标点。",
			})
		}
		if strings.Count(value, "（") != strings.Count(value, "）") || strings.Count(value, "“") != strings.Count(value, "”") {
			appendWordEditorFinding(result, wordEditorReviewFinding{
				Code: "unbalanced_delimiters", BlockID: blockID, BlockIndex: blockIndex, ParagraphID: paragraphID,
				CellID: cellID, RunID: inline.Text.ID, Detail: "文本运行中的中文括号或弯引号数量不平衡。",
			})
		}
	}
}

func appendWordEditorFinding(result *wordEditorReviewResult, finding wordEditorReviewFinding) {
	result.findingCount++
	if len(result.Findings) < maxWordEditorFindings {
		result.Findings = append(result.Findings, finding)
	}
}

func (result wordEditorReviewResult) output() any {
	score := max(60, 100-5*result.findingCount)
	return struct {
		StartBlock                  int                       `json:"start_block"`
		ScannedBlocks               int                       `json:"scanned_blocks"`
		TotalBlocks                 int                       `json:"total_blocks"`
		HeuristicScore              int                       `json:"heuristic_score"`
		CharactersWithWhitespace    int                       `json:"characters_with_whitespace"`
		CharactersWithoutWhitespace int                       `json:"characters_without_whitespace"`
		Paragraphs                  int                       `json:"paragraphs"`
		Tables                      int                       `json:"tables"`
		UnsupportedBlocks           int                       `json:"unsupported_blocks"`
		Findings                    []wordEditorReviewFinding `json:"findings"`
		FindingsTruncated           bool                      `json:"findings_truncated"`
		HasMore                     bool                      `json:"has_more"`
	}{
		result.StartBlock, result.Scanned, result.Total, score,
		result.CharactersWithWhitespace, result.CharactersWithoutWhitespace,
		result.Paragraphs, result.Tables, result.UnsupportedBlocks,
		result.Findings, result.findingCount > len(result.Findings), result.HasMore,
	}
}

func countWordParagraphText(stats *wordTextStatistics, paragraph document.WordParagraph) {
	for _, inline := range paragraph.Inlines {
		if inline.Text == nil {
			continue
		}
		for _, character := range inline.Text.Text {
			stats.CharactersWithWhitespace++
			if !unicode.IsSpace(character) {
				stats.CharactersWithoutWhitespace++
			}
		}
	}
}

func validWordLanguageCode(value string) bool {
	if len(value) < 2 || len(value) > 35 || !utf8.ValidString(value) {
		return false
	}
	parts := strings.Split(value, "-")
	if len(parts[0]) < 2 || len(parts[0]) > 8 {
		return false
	}
	for index, part := range parts {
		if len(part) == 0 || len(part) > 8 {
			return false
		}
		for _, character := range part {
			isLetter := character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
			isDigit := character >= '0' && character <= '9'
			if !isLetter && (index == 0 || !isDigit) {
				return false
			}
		}
	}
	return true
}
