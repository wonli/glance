package application

import (
	"context"
	"encoding/json"
)

func registerPDFPrintCapability(catalog *Catalog, printer PrintService) error {
	risk := RiskHigh
	if printer == nil || !printer.Available() {
		risk = RiskLow
	}
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "pdf.document.print", Name: "打印 PDF 文档",
			Description: "将指定修订版本的 PDF 发送到桌面系统打印面板，可选择一基页码范围和份数；省略页码时打印全部页面。MCP 调用会产生实体打印副作用，须由 Gio 人工确认；仅在平台提供原生打印服务时可用。文档修订变化或未提交编辑草稿会阻止打印。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatPDF}, Risk: risk,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"copies":{"type":"integer","minimum":1,"description":"打印份数，默认 1。"},"first_page":{"type":"integer","minimum":1,"maximum":50000,"description":"一基起始页；与 last_page 同时省略时打印全部页。"},"last_page":{"type":"integer","minimum":1,"maximum":50000,"description":"一基结束页；与 first_page 同时省略时打印全部页。"}},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","const":"sent_to_system_print_queue"},"printed_revision":{"type":"integer","minimum":0},"copies":{"type":"integer","minimum":1},"page_selection":{"type":"string","enum":["all","range"]},"first_page":{"type":"integer","minimum":0,"maximum":50000},"last_page":{"type":"integer","minimum":0,"maximum":50000}},"required":["status","printed_revision","copies","page_selection","first_page","last_page"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			if printer == nil || !printer.Available() {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			var input struct {
				Copies    int `json:"copies"`
				FirstPage int `json:"first_page"`
				LastPage  int `json:"last_page"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if (input.FirstPage == 0) != (input.LastPage == 0) || input.FirstPage > input.LastPage {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			reader, ok := capability.Adapter.(DocumentSnapshotReader)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			if drafts, ok := capability.Adapter.(interface{ HasPendingDrafts() bool }); ok && drafts.HasPendingDrafts() {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			pdfData, err := reader.Snapshot(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			if input.Copies == 0 {
				input.Copies = 1
			}
			release, err := capability.Runtime.lockDocumentRevision(ctx, capability.Document.DocumentID, capability.Document.Revision)
			if err != nil {
				return CapabilityResult{}, err
			}
			release()
			if err := printer.PrintPDF(ctx, pdfData, PrintOptions{
				Copies: input.Copies, FirstPage: input.FirstPage, LastPage: input.LastPage,
			}); err != nil {
				return CapabilityResult{}, err
			}
			pageSelection := "all"
			if input.FirstPage > 0 {
				pageSelection = "range"
			}
			return capabilityJSONResult(struct {
				Status          string `json:"status"`
				PrintedRevision uint64 `json:"printed_revision"`
				Copies          int    `json:"copies"`
				PageSelection   string `json:"page_selection"`
				FirstPage       int    `json:"first_page"`
				LastPage        int    `json:"last_page"`
			}{
				Status: "sent_to_system_print_queue", PrintedRevision: capability.Document.Revision,
				Copies: input.Copies, PageSelection: pageSelection,
				FirstPage: input.FirstPage, LastPage: input.LastPage,
			}, false)
		},
	})
}
