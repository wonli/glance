package application

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

type WordWatermarkReader interface {
	GetWordWatermark(context.Context) (string, error)
}

type WordWatermarkSetter interface {
	SetWordWatermark(context.Context, string) (bool, error)
}

func registerWordWatermarkCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.watermark.get", Name: "读取 Word 水印",
				Description: "读取当前 DOCX 的文本水印；返回空字符串表示没有文本水印。查询需要使用当前文档修订号。",
				Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
				reader, ok := capability.Adapter.(WordWatermarkReader)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				text, err := reader.GetWordWatermark(ctx)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Text string `json:"text"`
				}{text}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.watermark.set", Name: "设置 Word 水印",
				Description: "为 DOCX 设置或清除文本水印。text 会显示在文档页面背景中；传空字符串可清除水印。最多 256 个 Unicode 字符，不接受控制字符；先查询并使用当前文档修订号。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","maxLength":256}},"required":["text"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"text":{"type":"string"}},"required":["changed","text"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if !utf8.ValidString(input.Text) || utf8.RuneCountInString(input.Text) > 256 || strings.IndexFunc(input.Text, unicode.IsControl) >= 0 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				input.Text = strings.TrimSpace(input.Text)
				setter, ok := capability.Adapter.(WordWatermarkSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetWordWatermark(ctx, input.Text)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool   `json:"changed"`
					Text    string `json:"text"`
				}{changed, input.Text}, changed)
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
