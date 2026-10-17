package application

import (
	"context"
	"encoding/json"

	"glance/internal/document"
)

func registerLifecycleCapabilities(catalog *Catalog) error {
	documentInfoSchema := schemaForDocumentInfo()
	listSchema := json.RawMessage(`{"type":"array","items":{"type":"object","properties":{"document_id":{"type":"string"},"format":{"type":"string"},"source_path":{"type":"string"},"revision":{"type":"integer"},"dirty":{"type":"boolean"},"read_only":{"type":"boolean"},"supported_operations":{"type":"array","items":{"type":"string"}}}}}`)
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "document.list", Name: "列出已打开文档", Description: "列出此 Glance 运行时当前持有的实时文档会话及其修订、脏状态和格式。",
				Category: CapabilityQuery, Risk: RiskLow, InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`), OutputSchema: listSchema,
			},
			Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
				documents, err := capability.Runtime.ListDocuments(ctx)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(documents, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "document.inspect", Name: "查询文档内容", Description: "按格式读取当前实时文档的中立内容视图。表格可指定工作表和有界单元格区域；其他文档可使用 start/limit 分页。PPTX 查询额外返回 slide_width_pt、slide_height_pt、content.accessibility、稳定 slide_id/element_id、background_color_hex 与 background_image_present；辅助功能报告检查缺少可读取文本和图片替代文本的规则，complete=false 表示需要继续分页或达到元素上限。该报告不判断图片语义、颜色对比度或内容质量。",
				Category: CapabilityQuery, Risk: RiskLow, RequiresDocument: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"range":{"type":"object","properties":{"start_row":{"type":"integer"},"start_column":{"type":"integer"},"end_row":{"type":"integer"},"end_column":{"type":"integer"}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"start":{"type":"integer"},"limit":{"type":"integer"},"max_bytes":{"type":"integer"}},"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object"}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
					Range *struct {
						StartRow    int `json:"start_row"`
						StartColumn int `json:"start_column"`
						EndRow      int `json:"end_row"`
						EndColumn   int `json:"end_column"`
					} `json:"range"`
					Start    int `json:"start"`
					Limit    int `json:"limit"`
					MaxBytes int `json:"max_bytes"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				var bounds *document.CellRange
				if input.Range != nil {
					bounds = &document.CellRange{
						StartRow: input.Range.StartRow, StartColumn: input.Range.StartColumn,
						EndRow: input.Range.EndRow, EndColumn: input.Range.EndColumn,
					}
				}
				snapshot, err := capability.Runtime.Queries().InspectDocument(ctx, InspectRequest{
					DocumentID: capability.Document.DocumentID, Sheet: input.Sheet, Range: bounds,
					Start: input.Start, Limit: input.Limit, MaxBytes: input.MaxBytes,
				})
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(snapshot, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "document.open", Name: "打开文档", Description: "按显式本地路径打开文档，并返回实时会话 ID 与当前状态。重复打开同一路径会复用现有会话。",
				Category: CapabilityWorkflow, Risk: RiskLow,
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`), OutputSchema: documentInfoSchema,
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Path string `json:"path"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				info, err := capability.Runtime.OpenDocument(ctx, input.Path)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(info, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "document.create", Name: "新建文档", Description: "在当前运行时创建指定格式的空白文档会话。支持 docx、xlsx、pptx、pdf、json 和 markdown。",
				Category: CapabilityWorkflow, Risk: RiskLow,
				InputSchema: json.RawMessage(`{"type":"object","properties":{"format":{"type":"string"}},"required":["format"],"additionalProperties":false}`), OutputSchema: documentInfoSchema,
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Format Format `json:"format"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				info, err := capability.Runtime.CreateDocument(ctx, input.Format)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(info, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "document.get", Name: "读取文档状态", Description: "读取一个已打开文档的格式、路径、修订版本、脏状态和支持的操作。",
				Category: CapabilityQuery, Risk: RiskLow, RequiresDocument: true, InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`), OutputSchema: documentInfoSchema,
			},
			Handler: func(_ context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
				return capabilityJSONResult(capability.Document, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "document.save", Name: "保存文档",
				Description: "将明确文档保存回当前源路径，会覆盖该路径上的文件。Agent 调用需要桌面人类确认；没有源路径的新文档请使用 document.save_as。只在内容处于脏状态时写入。",
				Category:    CapabilityWorkflow, Risk: RiskHigh, RequiresDocument: true, RequiresRevision: true,
				Formats:     []Format{FormatDocx, FormatXLSX, FormatCSV, FormatPPTX, FormatPDF, FormatJSON, FormatMarkdown},
				InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"saved":{"type":"boolean"},"document":{"type":"object"}},"required":["saved","document"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
				if !capability.Document.Dirty {
					return capabilityJSONResult(struct {
						Saved    bool         `json:"saved"`
						Document DocumentInfo `json:"document"`
					}{Saved: false, Document: capability.Document}, false)
				}
				info, err := capability.Runtime.SaveDocument(ctx, capability.Document.DocumentID)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Saved    bool         `json:"saved"`
					Document DocumentInfo `json:"document"`
				}{Saved: true, Document: info}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "document.save_as", Name: "另存文档", Description: "将当前实时文档写入一个新的、尚不存在的本地路径。目标扩展名必须匹配文档格式；已有文件不会被覆盖。",
				Category: CapabilityWorkflow, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`), OutputSchema: documentInfoSchema,
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Path string `json:"path"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if canonicalPath(input.Path) == canonicalPath(capability.Document.SourcePath) && capability.Document.SourcePath != "" {
					return CapabilityResult{}, ErrTargetExists
				}
				info, err := capability.Runtime.SaveDocumentAs(ctx, capability.Document.DocumentID, input.Path)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(info, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "document.close", Name: "关闭文档会话", Description: "关闭一个干净的文档会话。存在未保存修改时会返回错误，必须先保存或由用户处理。",
				Category: CapabilityWorkflow, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"closed":{"type":"boolean"}},"required":["closed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
				if err := capability.Runtime.CloseDocument(ctx, capability.Document.DocumentID); err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Closed bool `json:"closed"`
				}{Closed: true}, false)
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
