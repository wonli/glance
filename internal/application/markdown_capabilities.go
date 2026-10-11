package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"glance/internal/format/markdowndoc"
)

const maxMarkdownCapabilityBytes = 8 * 1024 * 1024

type MarkdownTextAccess interface {
	ReadMarkdownText(context.Context) (string, error)
	SetMarkdownText(context.Context, string) (bool, error)
}

type markdownTextEditInput struct {
	Action      string `json:"action"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
	Label       string `json:"label,omitempty"`
	Destination string `json:"destination,omitempty"`
	Display     bool   `json:"display,omitempty"`
}

type markdownTextEditOutput struct {
	Changed      bool `json:"changed"`
	ContentBytes int  `json:"content_bytes"`
	Selection    struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"selection"`
}

func registerMarkdownCapabilities(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "markdown.text.edit", Name: "编辑 Markdown 文本",
			Description: "在明确的 Markdown 文档和 Unicode 字符区间执行标题、加粗、斜体、引用、代码、链接、图片、列表、待办或公式编辑。start/end 是从 0 开始的 rune 偏移，end 不包含在区间内；空区间表示插入位置。链接和图片需提供 destination；label 可替换选区文本。公式可提供 label 作为公式正文，display=true 表示独立公式块。文档和结果最多 8 MiB。",
			Category:    CapabilityCommand, Formats: []Format{FormatMarkdown}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["heading","bold","italic","quote","code","link","image","list","todo","math"],"description":"要应用的 Markdown 文本操作。"},"start":{"type":"integer","minimum":0,"description":"选区起始 rune 偏移，包含该位置。"},"end":{"type":"integer","minimum":0,"description":"选区结束 rune 偏移，不包含该位置。"},"label":{"type":"string","maxLength":4096,"description":"链接/图片显示文本，或公式正文；未提供时保留选中文字。"},"destination":{"type":"string","maxLength":4096,"description":"链接或图片目标地址。"},"display":{"type":"boolean","description":"仅用于公式：true 生成独立公式块，false 生成行内公式。"}},"required":["action","start","end"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"content_bytes":{"type":"integer","minimum":0,"maximum":8388608},"selection":{"type":"object","properties":{"start":{"type":"integer","minimum":0},"end":{"type":"integer","minimum":0}},"required":["start","end"],"additionalProperties":false}},"required":["changed","content_bytes","selection"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input markdownTextEditInput
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if utf8.RuneCountInString(input.Label) > 4096 || utf8.RuneCountInString(input.Destination) > 4096 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if (input.Action == "link" || input.Action == "image") && strings.TrimSpace(input.Destination) == "" {
				return CapabilityResult{}, fmt.Errorf("%w: destination is required for %s", ErrInvalidCommandArguments, input.Action)
			}
			if input.Action != "link" && input.Action != "image" && input.Destination != "" ||
				input.Action != "link" && input.Action != "image" && input.Action != "math" && input.Label != "" ||
				input.Action != "math" && input.Display {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			access, ok := capability.Adapter.(MarkdownTextAccess)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			source, err := access.ReadMarkdownText(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			if len(source) > maxMarkdownCapabilityBytes {
				return CapabilityResult{}, ErrQueryResultTooLarge
			}
			editSource, start, end := source, input.Start, input.End
			if input.Action == "math" && input.Label != "" {
				runes := []rune(source)
				if start > end {
					start, end = end, start
				}
				if start < 0 || end > len(runes) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				labelRunes := []rune(input.Label)
				replaced := make([]rune, 0, len(runes)-(end-start)+len(labelRunes))
				replaced = append(replaced, runes[:start]...)
				replaced = append(replaced, labelRunes...)
				replaced = append(replaced, runes[end:]...)
				editSource = string(replaced)
				end = start + utf8.RuneCountInString(input.Label)
			}
			if err := ctx.Err(); err != nil {
				return CapabilityResult{}, err
			}
			result, err := markdowndoc.ApplyEdit(editSource, input.Action, start, end, markdowndoc.EditArgs{
				Label: input.Label, Destination: input.Destination, Display: input.Display,
			})
			if err != nil {
				return CapabilityResult{}, err
			}
			if len(result.Text) > maxMarkdownCapabilityBytes {
				return CapabilityResult{}, ErrQueryResultTooLarge
			}
			if err := ctx.Err(); err != nil {
				return CapabilityResult{}, err
			}
			changed, err := access.SetMarkdownText(ctx, result.Text)
			if err != nil {
				return CapabilityResult{}, err
			}
			output := markdownTextEditOutput{Changed: changed, ContentBytes: len(result.Text)}
			output.Selection.Start, output.Selection.End = result.Start, result.End
			return capabilityJSONResult(output, changed)
		},
	})
}
