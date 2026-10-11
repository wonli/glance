package application

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"glance/internal/document"
)

const (
	maxPresentationCommentPageItems = 100
	maxPresentationCommentPreview   = 8 << 10
)

type PresentationCommentAccess interface {
	PresentationComments(context.Context, uint32, int, int) ([]document.PresentationComment, int, error)
	AddPresentationComment(context.Context, uint32, float64, float64, string, string) (string, bool, error)
	DeletePresentationComment(context.Context, uint32, string) (bool, error)
}

func registerPresentationCommentCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "presentation.comments.list", Name: "列出演示文稿批注",
				Description: "读取明确 PPTX 实时会话中的经典幻灯片批注，可按 slide_id 过滤并分页。comment_id 由作者 ID 和批注索引组成，可传给 presentation.comment.delete。text_truncated=true 表示正文被截断。当前不编辑新版 PowerPoint 批注格式。",
				Category:    CapabilityQuery, Formats: []Format{FormatPPTX}, Risk: RiskLow, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":0,"maximum":4294967295,"description":"幻灯片稳定 ID；省略或 0 表示全部幻灯片。"},"start":{"type":"integer","minimum":0,"maximum":1000000,"description":"分页起点，从 0 开始。"},"limit":{"type":"integer","minimum":1,"maximum":100,"description":"返回条数，省略时默认 50。"}},"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":0},"start":{"type":"integer","minimum":0},"total":{"type":"integer","minimum":0},"has_more":{"type":"boolean"},"comments":{"type":"array","maxItems":100,"items":{"type":"object","properties":{"comment_id":{"type":"string"},"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"slide_number":{"type":"integer","minimum":1},"author":{"type":"string"},"text":{"type":"string"},"text_truncated":{"type":"boolean"},"x_pt":{"type":"number","minimum":0},"y_pt":{"type":"number","minimum":0}},"required":["comment_id","slide_id","slide_number","author","text","text_truncated","x_pt","y_pt"],"additionalProperties":false}}},"required":["slide_id","start","total","has_more","comments"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				for key := range fields {
					if key != "slide_id" && key != "start" && key != "limit" {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
				}
				var input struct {
					SlideID uint32 `json:"slide_id"`
					Start   int    `json:"start"`
					Limit   int    `json:"limit"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.Start < 0 || input.Start > MaxQueryOffset || input.Limit < 0 || input.Limit > maxPresentationCommentPageItems {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				limit := input.Limit
				if limit == 0 {
					limit = 50
				}
				access, ok := capability.Adapter.(PresentationCommentAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				comments, total, err := access.PresentationComments(ctx, input.SlideID, input.Start, limit)
				if err != nil {
					return CapabilityResult{}, err
				}
				items := make([]presentationCommentListItem, 0, len(comments))
				for _, comment := range comments {
					if err := ctx.Err(); err != nil {
						return CapabilityResult{}, err
					}
					text, truncated := truncatePresentationComment(comment.Text, maxPresentationCommentPreview)
					items = append(items, presentationCommentListItem{
						CommentID: comment.ID, SlideID: comment.SlideID, SlideNumber: comment.SlideNumber,
						Author: comment.Author, Text: text, TextTruncated: truncated, XPT: comment.XPT, YPT: comment.YPT,
					})
				}
				return capabilityJSONResult(struct {
					SlideID  uint32                        `json:"slide_id"`
					Start    int                           `json:"start"`
					Total    int                           `json:"total"`
					HasMore  bool                          `json:"has_more"`
					Comments []presentationCommentListItem `json:"comments"`
				}{input.SlideID, input.Start, total, input.Start+len(items) < total, items}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "presentation.comment.add", Name: "添加演示文稿批注",
				Description: "在指定 PPTX 幻灯片坐标处添加经典原生批注。slide_id 必须来自 document.inspect；x_pt/y_pt 是从幻灯片左上角起算的点坐标（1 点 = 1/72 英寸）。author 省略或为空时使用 Glance。Ribbon“审阅/新建批注”调用同一能力。",
				Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的幻灯片稳定 ID。"},"x_pt":{"type":"number","minimum":0,"description":"批注在幻灯片上的水平坐标，单位为点。"},"y_pt":{"type":"number","minimum":0,"description":"批注在幻灯片上的垂直坐标，单位为点。"},"author":{"type":"string","maxLength":256,"description":"作者名称，最多 256 个 Unicode 字符；省略或空字符串使用 Glance。"},"text":{"type":"string","minLength":1,"maxLength":8192,"description":"批注正文，最多 8192 个 Unicode 字符。"}},"required":["slide_id","x_pt","y_pt","text"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"comment_id":{"type":"string"},"slide_id":{"type":"integer","minimum":1}},"required":["changed","comment_id","slide_id"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					SlideID uint32  `json:"slide_id"`
					XPT     float64 `json:"x_pt"`
					YPT     float64 `json:"y_pt"`
					Author  string  `json:"author"`
					Text    string  `json:"text"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || utf8.RuneCountInString(input.Author) > 256 || utf8.RuneCountInString(input.Text) == 0 || utf8.RuneCountInString(input.Text) > 8192 || !utf8.ValidString(input.Author) || !utf8.ValidString(input.Text) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				access, ok := capability.Adapter.(PresentationCommentAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				commentID, changed, err := access.AddPresentationComment(ctx, input.SlideID, input.XPT, input.YPT, input.Author, input.Text)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed   bool   `json:"changed"`
					CommentID string `json:"comment_id"`
					SlideID   uint32 `json:"slide_id"`
				}{changed, commentID, input.SlideID}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "presentation.comment.delete", Name: "删除演示文稿批注",
				Description: "按 presentation.comments.list 返回的 slide_id 和明确 comment_id 删除一条 PPTX 经典原生批注。删除不可自动撤销，Agent 调用需要 Gio 人类确认；Ribbon“审阅/删除批注”先列出批注，再要求用户确认目标 ID。",
				Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskHigh, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"批注所属幻灯片稳定 ID。"},"comment_id":{"type":"string","minLength":1,"maxLength":32,"description":"presentation.comments.list 返回的批注 ID。"}},"required":["slide_id","comment_id"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"comment_id":{"type":"string"},"slide_id":{"type":"integer","minimum":1}},"required":["changed","comment_id","slide_id"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					SlideID   uint32 `json:"slide_id"`
					CommentID string `json:"comment_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || strings.TrimSpace(input.CommentID) == "" || len(input.CommentID) > 32 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				access, ok := capability.Adapter.(PresentationCommentAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := access.DeletePresentationComment(ctx, input.SlideID, input.CommentID)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed   bool   `json:"changed"`
					CommentID string `json:"comment_id"`
					SlideID   uint32 `json:"slide_id"`
				}{changed, input.CommentID, input.SlideID}, changed)
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

type presentationCommentListItem struct {
	CommentID     string  `json:"comment_id"`
	SlideID       uint32  `json:"slide_id"`
	SlideNumber   int     `json:"slide_number"`
	Author        string  `json:"author"`
	Text          string  `json:"text"`
	TextTruncated bool    `json:"text_truncated"`
	XPT           float64 `json:"x_pt"`
	YPT           float64 `json:"y_pt"`
}

func truncatePresentationComment(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end], true
}
