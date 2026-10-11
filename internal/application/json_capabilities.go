package application

import (
	"context"
	"encoding/json"
	"fmt"

	"glance/internal/format/jsondoc"
)

const maxJSONCapabilityBytes = 8 * 1024 * 1024

// JSONTextAccess is the narrow source-text contract used by JSON Agent
// capabilities. Desktop and headless adapters implement it against the same
// live document object that save and inspection use.
type JSONTextAccess interface {
	ReadJSONText(context.Context) (string, error)
	SetJSONText(context.Context, string) (bool, error)
}

type jsonTransformInput struct {
	Operation string `json:"operation"`
}

type jsonTransformOutput struct {
	Changed      bool   `json:"changed"`
	ContentBytes int    `json:"content_bytes"`
	Operation    string `json:"operation"`
}

type jsonValidationOutput struct {
	Valid      bool   `json:"valid"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

func registerJSONCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "json.text.transform", Name: "转换 JSON 文本",
				Description: "对明确 JSON 文档执行格式化、压缩或键排序。操作直接作用于实时文档源文本；输入和输出最多 8 MiB，内容无效时不修改文档。",
				Category:    CapabilityCommand, Formats: []Format{FormatJSON}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"operation":{"type":"string","enum":["format","minify","sort_keys"],"description":"format 添加缩进，minify 删除无意义空白，sort_keys 按对象键排序。"}},"required":["operation"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"content_bytes":{"type":"integer","minimum":0,"maximum":8388608},"operation":{"type":"string","enum":["format","minify","sort_keys"]}},"required":["changed","content_bytes","operation"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input jsonTransformInput
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				access, ok := capability.Adapter.(JSONTextAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				text, err := access.ReadJSONText(ctx)
				if err != nil {
					return CapabilityResult{}, err
				}
				if len(text) > maxJSONCapabilityBytes {
					return CapabilityResult{}, ErrQueryResultTooLarge
				}
				var transformed string
				switch input.Operation {
				case "format":
					transformed, err = jsondoc.Format(text)
				case "minify":
					transformed, err = jsondoc.Minify(text)
				case "sort_keys":
					transformed, err = jsondoc.SortKeys(text)
				default:
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if err != nil {
					return CapabilityResult{}, err
				}
				if len(transformed) > maxJSONCapabilityBytes {
					return CapabilityResult{}, fmt.Errorf("%w: transformed JSON exceeds %d bytes", ErrQueryResultTooLarge, maxJSONCapabilityBytes)
				}
				if err := ctx.Err(); err != nil {
					return CapabilityResult{}, err
				}
				changed, err := access.SetJSONText(ctx, transformed)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(jsonTransformOutput{
					Changed: changed, ContentBytes: len(transformed), Operation: input.Operation,
				}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "json.document.validate", Name: "验证 JSON 文档",
				Description: "验证明确 JSON 文档的完整源文本，最多读取 8 MiB。无效 JSON 作为结构化诊断返回，不修改文档。",
				Category:    CapabilityQuery, Formats: []Format{FormatJSON}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"valid":{"type":"boolean"},"diagnostic":{"type":"string","maxLength":4096}},"required":["valid"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
				access, ok := capability.Adapter.(JSONTextAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				text, err := access.ReadJSONText(ctx)
				if err != nil {
					return CapabilityResult{}, err
				}
				if len(text) > maxJSONCapabilityBytes {
					return CapabilityResult{}, ErrQueryResultTooLarge
				}
				result := jsonValidationOutput{Valid: true}
				if err := jsondoc.Validate(text); err != nil {
					result.Valid = false
					diagnostic := []rune(err.Error())
					if len(diagnostic) > 4096 {
						diagnostic = diagnostic[:4096]
					}
					result.Diagnostic = string(diagnostic)
				}
				return capabilityJSONResult(result, false)
			},
		},
	}
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	if err := registerJSONSearchCapability(catalog); err != nil {
		return err
	}
	return registerJSONReadTextCapability(catalog)
}
