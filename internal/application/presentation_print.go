package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"glance/internal/format/presentationpdf"
)

func registerPresentationPrintCapability(catalog *Catalog, printer PrintService) error {
	risk := RiskHigh
	if printer == nil || !printer.Available() {
		risk = RiskLow
	}
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.document.print", Name: "打印演示文稿",
			Description: "将明确 PPTX 实时会话的快照按每张幻灯片一页渲染为 A4 PDF，再发送到桌面系统打印面板。MCP 调用会产生实体打印副作用，必须由 Gio 人类确认；仅在平台提供原生打印服务时可用。可选择页面方向、份数和一基页码范围；页码同时省略时打印全部页面。导出使用 Glance 当前投影的 PPTX 支持子集，动画、切换和媒体播放不会打印，音视频使用占位符。未提交草稿、过期修订或无法安全渲染的内容会阻止打印。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatPPTX}, Risk: risk,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"orientation":{"type":"string","enum":["portrait","landscape"],"description":"A4 页面方向；省略时按首张幻灯片方向选择。"},"copies":{"type":"integer","minimum":1,"maximum":100,"description":"打印份数，默认 1。"},"first_page":{"type":"integer","minimum":1,"maximum":2000,"description":"一基起始页；与 last_page 同时省略时打印全部页。"},"last_page":{"type":"integer","minimum":1,"maximum":2000,"description":"一基结束页；与 first_page 同时省略时打印全部页。"}},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","const":"sent_to_system_print_queue"},"printed_revision":{"type":"integer","minimum":0},"orientation":{"type":"string","enum":["portrait","landscape"]},"page_count":{"type":"integer","minimum":1,"maximum":2000},"copies":{"type":"integer","minimum":1,"maximum":100},"first_page":{"type":"integer","minimum":1},"last_page":{"type":"integer","minimum":1}},"required":["status","printed_revision","orientation","page_count","copies","first_page","last_page"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			if printer == nil || !printer.Available() {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			var input struct {
				Orientation string `json:"orientation"`
				Copies      int    `json:"copies"`
				FirstPage   int    `json:"first_page"`
				LastPage    int    `json:"last_page"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if input.Copies < 0 || input.Copies > 100 ||
				(input.FirstPage == 0) != (input.LastPage == 0) ||
				input.FirstPage > input.LastPage {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			reader, ok := capability.Adapter.(presentationSnapshotReader)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			if drafts, ok := capability.Adapter.(interface{ HasPendingDrafts() bool }); ok && drafts.HasPendingDrafts() {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			release, err := capability.Runtime.lockDocumentRevision(ctx, capability.Document.DocumentID, capability.Document.Revision)
			if err != nil {
				return CapabilityResult{}, err
			}
			snapshot, err := reader.PresentationSnapshot(ctx)
			release()
			if err != nil {
				return CapabilityResult{}, err
			}
			if input.Orientation == "" {
				input.Orientation = presentationpdf.DefaultOrientation(snapshot)
			}
			if input.Copies == 0 {
				input.Copies = 1
			}
			temporaryDirectory, err := os.MkdirTemp("", "glance-presentation-print-")
			if err != nil {
				return CapabilityResult{}, err
			}
			defer os.RemoveAll(temporaryDirectory)
			pdfPath := filepath.Join(temporaryDirectory, "print.pdf")
			fonts, err := capability.Runtime.FontSnapshot(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			export, err := presentationpdf.ExportSnapshotWithFontSnapshot(ctx, pdfPath, snapshot, input.Orientation, fonts, nil)
			if err != nil {
				return CapabilityResult{}, err
			}
			firstPage, lastPage, err := normalizePrintPageRange(input.FirstPage, input.LastPage, export.PageCount)
			if err != nil {
				return CapabilityResult{}, err
			}
			pdfData, err := os.ReadFile(pdfPath)
			if err != nil {
				return CapabilityResult{}, err
			}
			if drafts, ok := capability.Adapter.(interface{ HasPendingDrafts() bool }); ok && drafts.HasPendingDrafts() {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			release, err = capability.Runtime.lockDocumentRevision(ctx, capability.Document.DocumentID, capability.Document.Revision)
			if err != nil {
				return CapabilityResult{}, err
			}
			release()
			if err := printer.PrintPDF(ctx, pdfData, PrintOptions{Copies: input.Copies, FirstPage: firstPage, LastPage: lastPage}); err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Status          string `json:"status"`
				PrintedRevision uint64 `json:"printed_revision"`
				Orientation     string `json:"orientation"`
				PageCount       int    `json:"page_count"`
				Copies          int    `json:"copies"`
				FirstPage       int    `json:"first_page"`
				LastPage        int    `json:"last_page"`
			}{
				Status: "sent_to_system_print_queue", PrintedRevision: capability.Document.Revision,
				Orientation: input.Orientation, PageCount: export.PageCount, Copies: input.Copies,
				FirstPage: firstPage, LastPage: lastPage,
			}, false)
		},
	})
}
