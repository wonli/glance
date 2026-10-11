package application

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"glance/internal/document"
)

type WordDocumentPropertiesPatch struct {
	Title       *string
	Subject     *string
	Creator     *string
	Keywords    *string
	Description *string
}

type WordDocumentPropertiesAccess interface {
	GetWordDocumentProperties(context.Context) (document.WordDocumentProperties, error)
	PatchWordDocumentProperties(context.Context, WordDocumentPropertiesPatch) (document.WordDocumentProperties, bool, error)
}

type WordPageBackgroundAccess interface {
	GetWordPageBackground(context.Context) (string, error)
	SetWordPageBackground(context.Context, string) (bool, error)
}

func registerWordDocumentCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.document.properties.get", Name: "读取 Word 文档信息",
				Description: "读取 DOCX 核心属性中的标题、主题、作者、关键词和说明。结果是文件元数据，不包含正文内容。",
				Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow, RequiresDocument: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
				OutputSchema: wordDocumentPropertiesOutputSchema,
			},
			Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
				access, ok := capability.Adapter.(WordDocumentPropertiesAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				properties, err := access.GetWordDocumentProperties(ctx)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(properties, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.document.properties.set", Name: "更新 Word 文档信息",
				Description: "更新 DOCX 的标题、主题、作者、关键词或说明。只修改提供的字段；可传空字符串清除字段。每个字段最多 4096 个 Unicode 字符，至少提供一个字段。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","maxLength":4096},"subject":{"type":"string","maxLength":4096},"creator":{"type":"string","maxLength":4096},"keywords":{"type":"string","maxLength":4096},"description":{"type":"string","maxLength":4096}},"anyOf":[{"required":["title"]},{"required":["subject"]},{"required":["creator"]},{"required":["keywords"]},{"required":["description"]}],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"properties":{"type":"object","properties":{"title":{"type":"string"},"subject":{"type":"string"},"creator":{"type":"string"},"keywords":{"type":"string"},"description":{"type":"string"}},"required":["title","subject","creator","keywords","description"],"additionalProperties":false}},"required":["changed","properties"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Title       *string `json:"title"`
					Subject     *string `json:"subject"`
					Creator     *string `json:"creator"`
					Keywords    *string `json:"keywords"`
					Description *string `json:"description"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				for _, value := range []*string{input.Title, input.Subject, input.Creator, input.Keywords, input.Description} {
					if value != nil && utf8.RuneCountInString(*value) > 4096 {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
				}
				access, ok := capability.Adapter.(WordDocumentPropertiesAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				properties, changed, err := access.PatchWordDocumentProperties(ctx, WordDocumentPropertiesPatch{
					Title: input.Title, Subject: input.Subject, Creator: input.Creator,
					Keywords: input.Keywords, Description: input.Description,
				})
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed    bool                            `json:"changed"`
					Properties document.WordDocumentProperties `json:"properties"`
				}{changed, properties}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.page.background.get", Name: "读取 Word 页面底色",
				Description: "读取 DOCX 的页面底色。空字符串表示没有显式底色，页面使用阅读器默认背景。",
				Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow, RequiresDocument: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"color":{"type":"string"}},"required":["color"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
				access, ok := capability.Adapter.(WordPageBackgroundAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				color, err := access.GetWordPageBackground(ctx)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Color string `json:"color"`
				}{strings.ToUpper(strings.TrimPrefix(color, "#"))}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.page.background.set", Name: "设置 Word 页面底色",
				Description: "设置 DOCX 页面底色为六位 RGB 十六进制颜色，例如 E3F2FD；传空字符串、none 或 auto 可清除显式底色。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"color":{"type":"string","maxLength":7}},"required":["color"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"color":{"type":"string"}},"required":["changed","color"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Color string `json:"color"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				color, err := normalizeWordPageBackground(input.Color)
				if err != nil {
					return CapabilityResult{}, err
				}
				access, ok := capability.Adapter.(WordPageBackgroundAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := access.SetWordPageBackground(ctx, color)
				if err != nil {
					return CapabilityResult{}, err
				}
				resultColor := color
				if resultColor == "" {
					resultColor = "none"
				}
				return capabilityJSONResult(struct {
					Changed bool   `json:"changed"`
					Color   string `json:"color"`
				}{changed, resultColor}, changed)
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

var wordDocumentPropertiesOutputSchema = json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"},"subject":{"type":"string"},"creator":{"type":"string"},"keywords":{"type":"string"},"description":{"type":"string"}},"required":["title","subject","creator","keywords","description"],"additionalProperties":false}`)

func normalizeWordPageBackground(value string) (string, error) {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "#"))
	if value == "" || strings.EqualFold(value, "none") || strings.EqualFold(value, "auto") {
		return "", nil
	}
	if len(value) != 6 {
		return "", ErrInvalidCommandArguments
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f' || character >= 'A' && character <= 'F') {
			return "", ErrInvalidCommandArguments
		}
	}
	return strings.ToUpper(value), nil
}

// ApplyWordDocumentPropertiesPatch merges only the fields explicitly present in a patch.
func ApplyWordDocumentPropertiesPatch(current document.WordDocumentProperties, patch WordDocumentPropertiesPatch) document.WordDocumentProperties {
	if patch.Title != nil {
		current.Title = *patch.Title
	}
	if patch.Subject != nil {
		current.Subject = *patch.Subject
	}
	if patch.Creator != nil {
		current.Creator = *patch.Creator
	}
	if patch.Keywords != nil {
		current.Keywords = *patch.Keywords
	}
	if patch.Description != nil {
		current.Description = *patch.Description
	}
	return current
}
