package application

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"glance/internal/document"
)

const maxWordCommentListItems = 100
const maxWordCommentPreviewBytes = 8 << 10

type WordCommentAccess interface {
	WordComments(context.Context) ([]document.WordComment, error)
	AddWordComment(context.Context, string, string, string) (string, bool, error)
	DeleteWordComment(context.Context, string) (bool, error)
}

func registerWordCommentCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.comments.list", Name: "列出 Word 原生批注",
				Description: "读取明确 DOCX 实时会话中的原生 Word 批注，不读取 UI 选区。用 start 和 limit 分页，返回批注 ID、作者、文本和可唯一映射到的 run_id；run_id 为空表示范围跨越多个文本运行或无法可靠映射。新增批注前先用 document.inspect 获取可编辑 run_id。每条文本最多返回 8192 字节，text_truncated 表示原批注文本被截断。",
				Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow, RequiresDocument: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"start":{"type":"integer","minimum":0,"maximum":1000000,"description":"批注分页起点，从 0 开始。"},"limit":{"type":"integer","minimum":1,"maximum":100,"description":"返回条数，省略时默认 50。"}},"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"start":{"type":"integer","minimum":0},"total":{"type":"integer","minimum":0},"has_more":{"type":"boolean"},"comments":{"type":"array","maxItems":100,"items":{"type":"object","properties":{"id":{"type":"string"},"author":{"type":"string"},"text":{"type":"string"},"text_truncated":{"type":"boolean"},"run_id":{"type":"string"}},"required":["id","author","text","text_truncated","run_id"],"additionalProperties":false}}},"required":["start","total","has_more","comments"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Start int `json:"start"`
					Limit int `json:"limit"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.Start < 0 || input.Start > MaxQueryOffset || input.Limit < 0 || input.Limit > maxWordCommentListItems {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				limit := input.Limit
				if limit == 0 {
					limit = 50
				}
				access, ok := capability.Adapter.(WordCommentAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				comments, err := access.WordComments(ctx)
				if err != nil {
					return CapabilityResult{}, err
				}
				if input.Start > len(comments) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				end := input.Start + limit
				if end > len(comments) {
					end = len(comments)
				}
				items := make([]wordCommentListItem, 0, end-input.Start)
				for _, comment := range comments[input.Start:end] {
					if err := ctx.Err(); err != nil {
						return CapabilityResult{}, err
					}
					text, truncated := truncateWordComment(comment.Text, maxWordCommentPreviewBytes)
					items = append(items, wordCommentListItem{ID: comment.ID, Author: comment.Author, Text: text, TextTruncated: truncated, RunID: comment.RunID})
				}
				return capabilityJSONResult(struct {
					Start    int                   `json:"start"`
					Total    int                   `json:"total"`
					HasMore  bool                  `json:"has_more"`
					Comments []wordCommentListItem `json:"comments"`
				}{input.Start, len(comments), end < len(comments), items}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.comment.add", Name: "添加 Word 原生批注",
				Description: "在明确的可编辑正文文本运行上添加 DOCX 原生批注。run_id 必须来自 document.inspect；不读取 UI 选区。text 最多 32768 个 Unicode 字符，author 最多 256 个 Unicode 字符；author 省略或为空时使用 Glance。相同作者、文本和目标上的重复请求返回已有 comment_id 且 changed=false。Ribbon“审阅/新建批注”也调用此能力。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","minLength":1,"maxLength":512,"description":"document.inspect 返回的可编辑正文文本运行 ID。"},"author":{"type":"string","maxLength":256,"description":"批注作者；省略或空字符串使用 Glance。"},"text":{"type":"string","minLength":1,"maxLength":32768,"description":"批注正文。"}},"required":["run_id","text"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"comment_id":{"type":"string"},"run_id":{"type":"string"}},"required":["changed","comment_id","run_id"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					RunID  string `json:"run_id"`
					Author string `json:"author"`
					Text   string `json:"text"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || strings.TrimSpace(input.RunID) == "" || len(input.RunID) > 512 || utf8.RuneCountInString(input.Author) > 256 || utf8.RuneCountInString(input.Text) == 0 || utf8.RuneCountInString(input.Text) > 32768 || !utf8.ValidString(input.Text) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				access, ok := capability.Adapter.(WordCommentAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				commentID, changed, err := access.AddWordComment(ctx, input.RunID, input.Author, input.Text)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed   bool   `json:"changed"`
					CommentID string `json:"comment_id"`
					RunID     string `json:"run_id"`
				}{changed, commentID, input.RunID}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.comment.delete", Name: "删除 Word 原生批注",
				Description: "按 word.comments.list 返回的明确 comment_id 删除一条 DOCX 原生批注及其正文锚点。此操作会从文档移除批注，Agent 调用需要 Gio 人类确认；Ribbon“审阅/删除批注”列出当前文档批注并让用户输入目标 ID，再调用同一能力。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskHigh, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"comment_id":{"type":"string","minLength":1,"maxLength":32,"description":"word.comments.list 返回的批注 ID。"}},"required":["comment_id"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"comment_id":{"type":"string"}},"required":["changed","comment_id"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					CommentID string `json:"comment_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || strings.TrimSpace(input.CommentID) == "" || len(input.CommentID) > 32 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				access, ok := capability.Adapter.(WordCommentAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := access.DeleteWordComment(ctx, input.CommentID)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed   bool   `json:"changed"`
					CommentID string `json:"comment_id"`
				}{changed, input.CommentID}, changed)
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

type wordCommentListItem struct {
	ID            string `json:"id"`
	Author        string `json:"author"`
	Text          string `json:"text"`
	TextTruncated bool   `json:"text_truncated"`
	RunID         string `json:"run_id"`
}

func truncateWordComment(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end], true
}
