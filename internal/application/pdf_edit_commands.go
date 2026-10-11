package application

import (
	"context"
	"encoding/json"
	"math"

	"glance/internal/document"
)

type PDFCommandExecutor interface {
	ApplyPDFCommand(context.Context, document.PDFCommand) (bool, error)
}

func registerPDFEditCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "pdf.page.rotate", Name: "旋转 PDF 页面",
				Description: "将明确 PDF 页顺时针旋转 90 度。页码从 1 开始；操作基于 expected_revision，旋转后的文本和批注定位会按新修订重新读取。",
				Category:    CapabilityCommand, Formats: []Format{FormatPDF}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"page_number":{"type":"integer","minimum":1,"maximum":50000}},"required":["page_number"],"additionalProperties":false}`),
				OutputSchema: pdfEditOutputSchema(),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					PageNumber int `json:"page_number"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.PageNumber < 1 || input.PageNumber > 50000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				return applyPDFCommand(ctx, capability, document.PDFCommand{Kind: "rotate", Page: input.PageNumber - 1})
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "pdf.text.replace", Name: "替换 PDF 文本运行",
				Description: "替换明确 PDF 页面中的一个可编辑文本运行。run 是 document.inspect/search 返回的零起始运行索引，仅在 expected_revision 对应修订内有效。该高影响修改需要桌面人类确认；已签名、旋转、字体资源不安全或非原生文本会被拒绝。",
				Category:    CapabilityCommand, Formats: []Format{FormatPDF}, Risk: RiskHigh,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"page_number":{"type":"integer","minimum":1,"maximum":50000},"run":{"type":"integer","minimum":0,"maximum":500000},"text":{"type":"string","maxLength":32767}},"required":["page_number","run","text"],"additionalProperties":false}`),
				OutputSchema: pdfEditOutputSchema(),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					PageNumber int    `json:"page_number"`
					Run        int    `json:"run"`
					Text       string `json:"text"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.PageNumber < 1 || input.PageNumber > 50000 || input.Run < 0 || input.Run > 500000 || len([]rune(input.Text)) > 32767 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				return applyPDFCommand(ctx, capability, document.PDFCommand{Kind: "replace", Page: input.PageNumber - 1, Run: input.Run, Text: input.Text})
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "pdf.annotation.note", Name: "添加 PDF 批注",
				Description: "在明确 PDF 页坐标处添加文本批注。坐标使用 PDF 页面点数，不依赖当前鼠标位置；页码从 1 开始，单条批注最多 4000 个 Unicode 字符。",
				Category:    CapabilityCommand, Formats: []Format{FormatPDF}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"page_number":{"type":"integer","minimum":1,"maximum":50000},"x":{"type":"number"},"y":{"type":"number"},"text":{"type":"string","maxLength":4000}},"required":["page_number","x","y","text"],"additionalProperties":false}`),
				OutputSchema: pdfEditOutputSchema(),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					PageNumber int     `json:"page_number"`
					X          float64 `json:"x"`
					Y          float64 `json:"y"`
					Text       string  `json:"text"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.PageNumber < 1 || input.PageNumber > 50000 || !finitePDFPoint(input.X, input.Y) || len([]rune(input.Text)) > 4000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				return applyPDFCommand(ctx, capability, document.PDFCommand{Kind: "note", Page: input.PageNumber - 1, X: input.X, Y: input.Y, Text: input.Text, Author: pdfAnnotationAuthor(capability.Operator)})
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "pdf.annotation.highlight", Name: "高亮 PDF 区域",
				Description: "在明确 PDF 页上高亮一个矩形区域，bounds 坐标使用 PDF 页面点数，可选 text 作为批注内容。页面须处于原方向；页码从 1 开始。",
				Category:    CapabilityCommand, Formats: []Format{FormatPDF}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"page_number":{"type":"integer","minimum":1,"maximum":50000},"bounds":{"type":"object","properties":{"x":{"type":"number"},"y":{"type":"number"},"w":{"type":"number"},"h":{"type":"number"}},"required":["x","y","w","h"],"additionalProperties":false},"text":{"type":"string","maxLength":4000}},"required":["page_number","bounds"],"additionalProperties":false}`),
				OutputSchema: pdfEditOutputSchema(),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					PageNumber int `json:"page_number"`
					Bounds     struct {
						X float64 `json:"x"`
						Y float64 `json:"y"`
						W float64 `json:"w"`
						H float64 `json:"h"`
					} `json:"bounds"`
					Text string `json:"text"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.PageNumber < 1 || input.PageNumber > 50000 || !finitePDFPoint(input.Bounds.X, input.Bounds.Y) || !finitePDFPoint(input.Bounds.W, input.Bounds.H) || input.Bounds.W <= 0 || input.Bounds.H <= 0 || len([]rune(input.Text)) > 4000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				return applyPDFCommand(ctx, capability, document.PDFCommand{
					Kind: "highlight", Page: input.PageNumber - 1,
					X: input.Bounds.X, Y: input.Bounds.Y, W: input.Bounds.W, H: input.Bounds.H, Text: input.Text,
					Author: pdfAnnotationAuthor(capability.Operator),
				})
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "pdf.annotation.ink", Name: "在 PDF 页面绘制墨迹",
				Description: "按明确页面点坐标绘制一条 PDF 墨迹。points 至少包含 2 点、最多 10000 点；坐标使用 PDF 页面点数，超出页面的笔迹会被拒绝。",
				Category:    CapabilityCommand, Formats: []Format{FormatPDF}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"page_number":{"type":"integer","minimum":1,"maximum":50000},"points":{"type":"array","items":{"type":"object","properties":{"x":{"type":"number"},"y":{"type":"number"}},"required":["x","y"],"additionalProperties":false},"minItems":2,"maxItems":10000}},"required":["page_number","points"],"additionalProperties":false}`),
				OutputSchema: pdfEditOutputSchema(),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					PageNumber int                 `json:"page_number"`
					Points     []document.PDFPoint `json:"points"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.PageNumber < 1 || input.PageNumber > 50000 || len(input.Points) < 2 || len(input.Points) > 10000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				for _, point := range input.Points {
					if !finitePDFPoint(point.X, point.Y) {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
				}
				return applyPDFCommand(ctx, capability, document.PDFCommand{Kind: "ink", Page: input.PageNumber - 1, Points: input.Points})
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "pdf.history.apply", Name: "撤销或重做 PDF 修改",
				Description: "对明确 PDF 文档执行一次撤销或重做。操作基于 expected_revision；没有对应历史记录时返回 changed=false。",
				Category:    CapabilityCommand, Formats: []Format{FormatPDF}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"operation":{"type":"string","enum":["undo","redo"]}},"required":["operation"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"operation":{"type":"string","enum":["undo","redo"]},"changed":{"type":"boolean"}},"required":["operation","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Operation string `json:"operation"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.Operation != "undo" && input.Operation != "redo" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				changed, err := executePDFCommand(ctx, capability, document.PDFCommand{Kind: input.Operation})
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Operation string `json:"operation"`
					Changed   bool   `json:"changed"`
				}{input.Operation, changed}, changed)
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

func applyPDFCommand(ctx context.Context, capability CapabilityContext, command document.PDFCommand) (CapabilityResult, error) {
	command.Revision = capability.Document.Revision
	changed, err := executePDFCommand(ctx, capability, command)
	if err != nil {
		return CapabilityResult{}, err
	}
	return capabilityJSONResult(struct {
		Changed    bool `json:"changed"`
		PageNumber int  `json:"page_number"`
	}{changed, command.Page + 1}, changed)
}

func executePDFCommand(ctx context.Context, capability CapabilityContext, command document.PDFCommand) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	executor, ok := capability.Adapter.(PDFCommandExecutor)
	if !ok {
		return false, ErrCapabilityUnsupported
	}
	command.Revision = capability.Document.Revision
	return executor.ApplyPDFCommand(ctx, command)
}

func finitePDFPoint(x, y float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0) && !math.IsNaN(y) && !math.IsInf(y, 0)
}

func pdfAnnotationAuthor(operator Operator) string {
	switch operator.Kind {
	case OperatorHuman:
		return "我"
	case OperatorExternalAgent, OperatorBuiltInAgent:
		return "Agent"
	default:
		return "Glance"
	}
}

func pdfEditOutputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"page_number":{"type":"integer","minimum":1}},"required":["changed","page_number"],"additionalProperties":false}`)
}
