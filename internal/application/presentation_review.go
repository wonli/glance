package application

import (
	"context"
	"encoding/json"
	"strings"
)

const (
	presentationProofreadPageSize = 100
	maxPresentationFindings       = 500
)

type presentationInspectionReader interface {
	Inspect(context.Context, InspectRequest) (json.RawMessage, []Diagnostic, error)
}

type presentationProofreadFinding struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	SlideID     uint32 `json:"slide_id"`
	SlideNumber int    `json:"slide_number"`
	ElementID   int64  `json:"element_id"`
}

type presentationProofreadResult struct {
	Start             int                            `json:"start"`
	ScannedSlides     int                            `json:"scanned_slides"`
	TotalSlides       int                            `json:"total_slides"`
	Complete          bool                           `json:"complete"`
	ElementsTruncated bool                           `json:"elements_truncated"`
	HasMore           bool                           `json:"has_more"`
	FindingsTruncated bool                           `json:"findings_truncated"`
	Findings          []presentationProofreadFinding `json:"findings"`
	findingCount      int
}

func registerPresentationReviewCapabilities(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.text.proofread", Name: "检查演示文稿文本",
			Description: "对明确 PPTX 文档按幻灯片分页执行本地启发式检查，报告连续重复中文标点和中文括号/弯引号数量不平衡，并返回稳定 slide_id 与 element_id。每页最多检查 100 张幻灯片，最多返回 500 项；complete=false 或 has_more=true 表示结果不完整。此能力不是词典拼写、语法或语义检查，不修改文档。",
			Category:    CapabilityQuery, Formats: []Format{FormatPPTX}, Risk: RiskLow, RequiresDocument: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"start":{"type":"integer","minimum":0,"maximum":1000000,"description":"幻灯片分页起点，从 0 开始。"},"limit":{"type":"integer","minimum":1,"maximum":100,"description":"本次检查的幻灯片数量，最多 100 张；省略时为 100。"}},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"start":{"type":"integer","minimum":0},"scanned_slides":{"type":"integer","minimum":0,"maximum":100},"total_slides":{"type":"integer","minimum":0},"complete":{"type":"boolean"},"elements_truncated":{"type":"boolean"},"has_more":{"type":"boolean"},"findings_truncated":{"type":"boolean"},"findings":{"type":"array","maxItems":500,"items":{"type":"object","properties":{"code":{"type":"string","enum":["repeated_punctuation","unbalanced_delimiters"]},"message":{"type":"string"},"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"slide_number":{"type":"integer","minimum":1},"element_id":{"type":"integer","minimum":1}},"required":["code","message","slide_id","slide_number","element_id"],"additionalProperties":false}}},"required":["start","scanned_slides","total_slides","complete","elements_truncated","has_more","findings_truncated","findings"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			for key := range fields {
				if key != "start" && key != "limit" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			}
			var input struct {
				Start int `json:"start"`
				Limit int `json:"limit"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.Start < 0 || input.Start > MaxQueryOffset || input.Limit < 0 || input.Limit > presentationProofreadPageSize {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			limit := input.Limit
			if limit == 0 {
				limit = presentationProofreadPageSize
			}
			reader, ok := capability.Adapter.(presentationInspectionReader)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			content, diagnostics, err := reader.Inspect(ctx, InspectRequest{
				Start: input.Start, Limit: limit, MaxBytes: MaxQueryResultBytes,
			})
			if err != nil {
				return CapabilityResult{}, err
			}
			var inspected struct {
				SlideCount int `json:"slide_count"`
				Start      int `json:"start"`
				Slides     []struct {
					Index    int    `json:"index"`
					ID       uint32 `json:"slide_id"`
					Elements []struct {
						ID   int64  `json:"id"`
						Kind string `json:"kind"`
						Text string `json:"text"`
					} `json:"elements"`
				} `json:"slides"`
			}
			if err := json.Unmarshal(content, &inspected); err != nil || inspected.SlideCount < 0 {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			if input.Start > inspected.SlideCount {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if inspected.Start != input.Start || len(inspected.Slides) > limit {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			result := presentationProofreadResult{
				Start: input.Start, ScannedSlides: len(inspected.Slides), TotalSlides: inspected.SlideCount,
				Complete: input.Start+len(inspected.Slides) >= inspected.SlideCount, HasMore: input.Start+len(inspected.Slides) < inspected.SlideCount,
				Findings: make([]presentationProofreadFinding, 0, 16),
			}
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == "elements_truncated" {
					result.ElementsTruncated = true
					result.Complete = false
				}
			}
			for _, slide := range inspected.Slides {
				if err := ctx.Err(); err != nil {
					return CapabilityResult{}, err
				}
				for _, element := range slide.Elements {
					if strings.TrimSpace(element.Text) == "" {
						continue
					}
					appendPresentationProofreadFinding(&result, element.Text, slide.ID, slide.Index+1, element.ID)
				}
			}
			result.FindingsTruncated = result.findingCount > len(result.Findings)
			return capabilityJSONResult(result, false)
		},
	})
}

func appendPresentationProofreadFinding(result *presentationProofreadResult, text string, slideID uint32, slideNumber int, elementID int64) {
	if strings.Contains(text, "，，") || strings.Contains(text, "。。") || strings.Contains(text, "、、") {
		appendPresentationFinding(result, presentationProofreadFinding{
			Code: "repeated_punctuation", Message: "文本包含连续重复的中文标点。",
			SlideID: slideID, SlideNumber: slideNumber, ElementID: elementID,
		})
	}
	if strings.Count(text, "（") != strings.Count(text, "）") || strings.Count(text, "“") != strings.Count(text, "”") {
		appendPresentationFinding(result, presentationProofreadFinding{
			Code: "unbalanced_delimiters", Message: "中文括号或弯引号数量不平衡。",
			SlideID: slideID, SlideNumber: slideNumber, ElementID: elementID,
		})
	}
}

func appendPresentationFinding(result *presentationProofreadResult, finding presentationProofreadFinding) {
	result.findingCount++
	if len(result.Findings) < maxPresentationFindings {
		result.Findings = append(result.Findings, finding)
	}
}
