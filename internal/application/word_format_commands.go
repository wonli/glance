package application

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"unicode"

	"glance/internal/document"
)

var (
	ErrWordTextRunUnavailable   = errors.New("Word text run is missing or not editable")
	ErrWordParagraphUnavailable = errors.New("Word paragraph is missing or not editable")
)

type WordTextRunStyleSetter interface {
	SetWordTextRunStyle(context.Context, string, document.WordTextFormatPatch) (bool, error)
}

type WordParagraphAlignmentSetter interface {
	SetWordParagraphAlignment(context.Context, string, document.WordAlignment) (bool, error)
}

type WordParagraphIndentSetter interface {
	SetWordParagraphIndents(context.Context, string, int64, int64) (bool, error)
}

type WordTextFontFamilySetter interface {
	SetWordTextRunFontFamily(context.Context, string, string) (bool, error)
}

type WordTextFontSizeSetter interface {
	SetWordTextRunFontSize(context.Context, string, float64) (bool, error)
}

type WordParagraphListSetter interface {
	SetWordParagraphList(context.Context, string, document.WordListKind) (bool, error)
}

type WordParagraphSpacingSetter interface {
	SetWordParagraphSpacing(context.Context, string, *int64, *int64) (bool, error)
}

func registerWordFormatCapabilities(catalog *Catalog) error {
	registration := CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.text_run.style.set", Name: "设置 Word 文本样式",
			Description: "对明确的可编辑文本运行设置一个或多个直接样式属性：bold、italic、underline、font_color（六位 RGB）和 highlight（Word 支持的颜色名）。可在一次调用中组合多个属性；未提供的属性保持原样。先查询 run_id 并使用当前文档修订。",
			Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string"},"bold":{"type":"boolean"},"italic":{"type":"boolean"},"underline":{"type":"boolean"},"font_color":{"type":"string"},"highlight":{"type":"string"}},"required":["run_id"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"run_id":{"type":"string"}},"required":["changed","run_id"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				RunID     string  `json:"run_id"`
				Bold      *bool   `json:"bold"`
				Italic    *bool   `json:"italic"`
				Underline *bool   `json:"underline"`
				FontColor *string `json:"font_color"`
				Highlight *string `json:"highlight"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, err
			}
			if strings.TrimSpace(input.RunID) == "" || input.Bold == nil && input.Italic == nil && input.Underline == nil && input.FontColor == nil && input.Highlight == nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			setter, ok := capability.Adapter.(WordTextRunStyleSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := setter.SetWordTextRunStyle(ctx, input.RunID, document.WordTextFormatPatch{
				Bold: input.Bold, Italic: input.Italic, Underline: input.Underline,
				FontColor: input.FontColor, Highlight: input.Highlight,
			})
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed bool   `json:"changed"`
				RunID   string `json:"run_id"`
			}{changed, input.RunID}, changed)
		},
	}
	registrations := []CapabilityRegistration{
		registration,
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.paragraph.alignment.set", Name: "设置 Word 段落对齐",
				Description: "设置明确段落的对齐方式：left、center、right 或 justified。可通过 document.inspect 返回的 paragraph_id 和当前修订定位。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"paragraph_id":{"type":"string"},"alignment":{"type":"string"}},"required":["paragraph_id","alignment"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"paragraph_id":{"type":"string"},"alignment":{"type":"string"}},"required":["changed","paragraph_id","alignment"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					ParagraphID string                 `json:"paragraph_id"`
					Alignment   document.WordAlignment `json:"alignment"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.ParagraphID) == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if input.Alignment != document.WordAlignLeft && input.Alignment != document.WordAlignCenter && input.Alignment != document.WordAlignRight && input.Alignment != document.WordAlignJustified {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(WordParagraphAlignmentSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetWordParagraphAlignment(ctx, input.ParagraphID, input.Alignment)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed     bool                   `json:"changed"`
					ParagraphID string                 `json:"paragraph_id"`
					Alignment   document.WordAlignment `json:"alignment"`
				}{changed, input.ParagraphID, input.Alignment}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.paragraph.indent.set", Name: "设置 Word 段落缩进",
				Description: "将明确段落的左右缩进设为绝对值，单位为 points。document.inspect 返回当前缩进值；合法范围为 -10000 至 10000 points。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"paragraph_id":{"type":"string"},"left_indent_points":{"type":"number"},"right_indent_points":{"type":"number"}},"required":["paragraph_id","left_indent_points","right_indent_points"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"paragraph_id":{"type":"string"},"left_indent_points":{"type":"number"},"right_indent_points":{"type":"number"}},"required":["changed","paragraph_id","left_indent_points","right_indent_points"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					ParagraphID string  `json:"paragraph_id"`
					LeftPoints  float64 `json:"left_indent_points"`
					RightPoints float64 `json:"right_indent_points"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.ParagraphID) == "" || math.IsNaN(input.LeftPoints) || math.IsInf(input.LeftPoints, 0) || math.IsNaN(input.RightPoints) || math.IsInf(input.RightPoints, 0) || math.Abs(input.LeftPoints) > 10000 || math.Abs(input.RightPoints) > 10000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(WordParagraphIndentSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				leftEMU, rightEMU := wordIndentPointsToEMU(input.LeftPoints), wordIndentPointsToEMU(input.RightPoints)
				changed, err := setter.SetWordParagraphIndents(ctx, input.ParagraphID, leftEMU, rightEMU)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed     bool    `json:"changed"`
					ParagraphID string  `json:"paragraph_id"`
					LeftPoints  float64 `json:"left_indent_points"`
					RightPoints float64 `json:"right_indent_points"`
				}{changed, input.ParagraphID, input.LeftPoints, input.RightPoints}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.text_run.font_family.set", Name: "设置 Word 字体",
				Description: "设置明确可编辑文本运行的字体族，长度为 1 至 128 个可打印字符。先通过 document.inspect 获取 run_id；修改前使用当前文档修订。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string"},"font_family":{"type":"string","minLength":1,"maxLength":128}},"required":["run_id","font_family"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"run_id":{"type":"string"},"font_family":{"type":"string"}},"required":["changed","run_id","font_family"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					RunID      string `json:"run_id"`
					FontFamily string `json:"font_family"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				input.FontFamily = strings.TrimSpace(input.FontFamily)
				if strings.TrimSpace(input.RunID) == "" || input.FontFamily == "" || len([]rune(input.FontFamily)) > 128 || strings.IndexFunc(input.FontFamily, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(WordTextFontFamilySetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetWordTextRunFontFamily(ctx, input.RunID, input.FontFamily)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed    bool   `json:"changed"`
					RunID      string `json:"run_id"`
					FontFamily string `json:"font_family"`
				}{changed, input.RunID, input.FontFamily}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.text_run.font_size.set", Name: "设置 Word 字号",
				Description: "将明确可编辑文本运行的字号设为 1 至 1638 points。先通过 document.inspect 获取 run_id 和当前字号；修改前使用当前文档修订。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string"},"font_size_points":{"type":"number","minimum":1,"maximum":1638}},"required":["run_id","font_size_points"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"run_id":{"type":"string"},"font_size_points":{"type":"number"}},"required":["changed","run_id","font_size_points"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					RunID          string  `json:"run_id"`
					FontSizePoints float64 `json:"font_size_points"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.RunID) == "" || math.IsNaN(input.FontSizePoints) || math.IsInf(input.FontSizePoints, 0) || input.FontSizePoints < 1 || input.FontSizePoints > 1638 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(WordTextFontSizeSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetWordTextRunFontSize(ctx, input.RunID, input.FontSizePoints)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed        bool    `json:"changed"`
					RunID          string  `json:"run_id"`
					FontSizePoints float64 `json:"font_size_points"`
				}{changed, input.RunID, input.FontSizePoints}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.paragraph.list.set", Name: "设置 Word 列表",
				Description: "将明确段落设为普通段落、项目符号列表或编号列表。支持 list_type：none、bullet、numbered；先通过 document.inspect 获取 paragraph_id。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"paragraph_id":{"type":"string"},"list_type":{"type":"string","enum":["none","bullet","numbered"]}},"required":["paragraph_id","list_type"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"paragraph_id":{"type":"string"},"list_type":{"type":"string"}},"required":["changed","paragraph_id","list_type"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					ParagraphID string                `json:"paragraph_id"`
					ListType    document.WordListKind `json:"list_type"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.ParagraphID) == "" || input.ListType != document.WordListNone && input.ListType != document.WordListBullet && input.ListType != document.WordListNumbered {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(WordParagraphListSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetWordParagraphList(ctx, input.ParagraphID, input.ListType)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed     bool                  `json:"changed"`
					ParagraphID string                `json:"paragraph_id"`
					ListType    document.WordListKind `json:"list_type"`
				}{changed, input.ParagraphID, input.ListType}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.paragraph.spacing.set", Name: "设置 Word 段落间距",
				Description: "设置明确段落的段前和/或段后间距，单位为 points，范围为 0 至 10000。至少提供 before_points 或 after_points 之一；未提供的一侧保持原样。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"paragraph_id":{"type":"string"},"before_points":{"type":"number","minimum":0,"maximum":10000},"after_points":{"type":"number","minimum":0,"maximum":10000}},"required":["paragraph_id"],"anyOf":[{"required":["before_points"]},{"required":["after_points"]}],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"paragraph_id":{"type":"string"},"before_points":{"type":"number"},"after_points":{"type":"number"}},"required":["changed","paragraph_id"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					ParagraphID string   `json:"paragraph_id"`
					Before      *float64 `json:"before_points"`
					After       *float64 `json:"after_points"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.ParagraphID) == "" || input.Before == nil && input.After == nil || !validWordSpacingPoints(input.Before) || !validWordSpacingPoints(input.After) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				var before, after *int64
				if input.Before != nil {
					value := int64(math.Round(*input.Before * 20))
					before = &value
				}
				if input.After != nil {
					value := int64(math.Round(*input.After * 20))
					after = &value
				}
				setter, ok := capability.Adapter.(WordParagraphSpacingSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetWordParagraphSpacing(ctx, input.ParagraphID, before, after)
				if err != nil {
					return CapabilityResult{}, err
				}
				result := map[string]any{"changed": changed, "paragraph_id": input.ParagraphID}
				if input.Before != nil {
					result["before_points"] = float64(*before) / 20
				}
				if input.After != nil {
					result["after_points"] = float64(*after) / 20
				}
				return capabilityJSONResult(result, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.text.search", Name: "搜索 Word 文本",
				Description: "在指定文档块页内搜索可编辑文本运行，返回段落 ID、运行 ID 和 Unicode 字符范围。搜索不跨文本运行；默认忽略大小写。start_block 和 max_blocks 控制扫描范围，结果条数有界，可按 has_more 继续分页。",
				Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow, RequiresDocument: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","minLength":1,"maxLength":4096},"case_sensitive":{"type":"boolean"},"start_block":{"type":"integer","minimum":0,"maximum":1000000},"max_blocks":{"type":"integer","minimum":1,"maximum":1000},"limit":{"type":"integer","minimum":1,"maximum":1000}},"required":["query"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"start_block":{"type":"integer"},"scanned_blocks":{"type":"integer"},"total_blocks":{"type":"integer"},"matches":{"type":"array","items":{"type":"object","properties":{"paragraph_id":{"type":"string"},"run_id":{"type":"string"},"start":{"type":"integer"},"end":{"type":"integer"},"text":{"type":"string"}},"required":["paragraph_id","run_id","start","end","text"],"additionalProperties":false}},"has_more":{"type":"boolean"}},"required":["start_block","scanned_blocks","total_blocks","matches","has_more"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Query         string `json:"query"`
					CaseSensitive bool   `json:"case_sensitive"`
					StartBlock    int    `json:"start_block"`
					MaxBlocks     int    `json:"max_blocks"`
					Limit         int    `json:"limit"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.Query) == "" || len([]rune(input.Query)) > 4096 || input.StartBlock < 0 || input.StartBlock > MaxQueryOffset || input.MaxBlocks < 0 || input.MaxBlocks > 1000 || input.Limit < 0 || input.Limit > 1000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				maxBlocks := input.MaxBlocks
				if maxBlocks == 0 {
					maxBlocks = 100
				}
				limit := input.Limit
				if limit == 0 {
					limit = 100
				}
				snapshot, err := capability.Runtime.Queries().InspectDocument(ctx, InspectRequest{
					DocumentID: capability.Document.DocumentID, Start: input.StartBlock, Limit: maxBlocks, MaxBytes: MaxQueryResultBytes,
				})
				if err != nil {
					return CapabilityResult{}, err
				}
				var content struct {
					Start  int `json:"start"`
					Total  int `json:"total_blocks"`
					Blocks []struct {
						Runs []struct {
							ParagraphID string `json:"paragraph_id"`
							ID          string `json:"id"`
							Text        string `json:"text"`
							Editable    bool   `json:"editable"`
						} `json:"runs"`
					} `json:"blocks"`
				}
				if err := json.Unmarshal(snapshot.Content, &content); err != nil {
					return CapabilityResult{}, ErrInvalidQuery
				}
				needle := []rune(input.Query)
				if !input.CaseSensitive {
					for index := range needle {
						needle[index] = unicode.ToLower(needle[index])
					}
				}
				type wordSearchMatch struct {
					ParagraphID string `json:"paragraph_id"`
					RunID       string `json:"run_id"`
					Start       int    `json:"start"`
					End         int    `json:"end"`
					Text        string `json:"text"`
				}
				matches := make([]wordSearchMatch, 0, min(limit, 32))
				moreMatches := false
			searching:
				for _, block := range content.Blocks {
					for _, run := range block.Runs {
						if !run.Editable || run.ID == "" || run.ParagraphID == "" {
							continue
						}
						value := []rune(run.Text)
						for start := 0; start+len(needle) <= len(value); start++ {
							matched := true
							for offset, want := range needle {
								got := value[start+offset]
								if !input.CaseSensitive {
									got = unicode.ToLower(got)
								}
								if got != want {
									matched = false
									break
								}
							}
							if matched {
								if len(matches) == limit {
									moreMatches = true
									break searching
								}
								matches = append(matches, wordSearchMatch{
									ParagraphID: run.ParagraphID, RunID: run.ID, Start: start, End: start + len(needle), Text: string(value[start : start+len(needle)]),
								})
							}
						}
					}
				}
				scanned := len(content.Blocks)
				return capabilityJSONResult(struct {
					StartBlock    int               `json:"start_block"`
					ScannedBlocks int               `json:"scanned_blocks"`
					TotalBlocks   int               `json:"total_blocks"`
					Matches       []wordSearchMatch `json:"matches"`
					HasMore       bool              `json:"has_more"`
				}{content.Start, scanned, content.Total, matches, moreMatches || content.Start+scanned < content.Total}, false)
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

func validWordSpacingPoints(points *float64) bool {
	return points == nil || !math.IsNaN(*points) && !math.IsInf(*points, 0) && *points >= 0 && *points <= 10000
}

func wordIndentPointsToEMU(points float64) int64 {
	return int64(math.Round(points*20)) * 635
}

func editableWordTextRun(blocks []document.WordBlock, runID string) (document.WordTextRun, bool) {
	if strings.TrimSpace(runID) == "" {
		return document.WordTextRun{}, false
	}
	find := func(paragraph document.WordParagraph) (document.WordTextRun, bool) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == runID && inline.Text.Editable {
				return *inline.Text, true
			}
		}
		return document.WordTextRun{}, false
	}
	for _, block := range blocks {
		if block.Paragraph != nil {
			if run, ok := find(*block.Paragraph); ok {
				return run, true
			}
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					for _, paragraph := range cell.Paragraphs {
						if run, ok := find(paragraph); ok {
							return run, true
						}
					}
				}
			}
		}
	}
	return document.WordTextRun{}, false
}

func wordBlocksContainEditableText(blocks []document.WordBlock) bool {
	for _, block := range blocks {
		if block.Paragraph != nil {
			for _, inline := range block.Paragraph.Inlines {
				if inline.Text != nil && inline.Text.Editable {
					return true
				}
			}
		}
		if block.Table == nil {
			continue
		}
		for _, row := range block.Table.Rows {
			for _, cell := range row.Cells {
				for _, paragraph := range cell.Paragraphs {
					for _, inline := range paragraph.Inlines {
						if inline.Text != nil && inline.Text.Editable {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func wordTextStylePatchUnchanged(style document.WordTextStyle, patch document.WordTextFormatPatch) bool {
	changed := false
	if patch.Bold != nil {
		changed = changed || style.Bold != *patch.Bold
	}
	if patch.Italic != nil {
		changed = changed || style.Italic != *patch.Italic
	}
	if patch.Underline != nil {
		changed = changed || style.Underline != *patch.Underline
	}
	if patch.FontColor != nil {
		want := strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(*patch.FontColor), "#"))
		got := strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(style.FontColor), "#"))
		changed = changed || want != got
	}
	if patch.Highlight != nil {
		want, got := strings.ToLower(strings.TrimSpace(*patch.Highlight)), strings.ToLower(strings.TrimSpace(style.Highlight))
		if want == "none" {
			want = ""
		}
		changed = changed || want != got
	}
	return !changed
}

func wordParagraphByID(blocks []document.WordBlock, paragraphID string) (document.WordParagraph, bool) {
	for _, block := range blocks {
		if block.Paragraph != nil && block.Paragraph.ID == paragraphID {
			return *block.Paragraph, true
		}
		if block.Table == nil {
			continue
		}
		for _, row := range block.Table.Rows {
			for _, cell := range row.Cells {
				for _, paragraph := range cell.Paragraphs {
					if paragraph.ID == paragraphID {
						return paragraph, true
					}
				}
			}
		}
	}
	return document.WordParagraph{}, false
}

func wordBodyParagraphByID(blocks []document.WordBlock, paragraphID string) (document.WordParagraph, bool) {
	for _, block := range blocks {
		if block.Paragraph != nil && block.Paragraph.ID == paragraphID {
			return *block.Paragraph, true
		}
	}
	return document.WordParagraph{}, false
}

func wordParagraphSpacingUnchanged(paragraph document.WordParagraph, beforeTwips, afterTwips *int64) bool {
	if beforeTwips != nil && (paragraph.Layout.Set&document.WordParagraphBeforeSet == 0 || int64(math.Round(paragraph.Layout.BeforePT*20)) != *beforeTwips) {
		return false
	}
	if afterTwips != nil && (paragraph.Layout.Set&document.WordParagraphAfterSet == 0 || int64(math.Round(paragraph.Layout.AfterPT*20)) != *afterTwips) {
		return false
	}
	return true
}
