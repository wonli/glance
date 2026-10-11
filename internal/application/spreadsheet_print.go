package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"glance/internal/document"
	"glance/internal/format/sheetpdf"
)

func registerSpreadsheetPrintCapability(catalog *Catalog, printer PrintService) error {
	risk := RiskHigh
	if printer == nil || !printer.Available() {
		risk = RiskLow
	}
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.document.print", Name: "打印工作表",
			Description: "将指定 XLSX 工作表和单元格范围渲染为 PDF 并发送到桌面系统打印面板。MCP 调用会产生实体打印副作用，须由 Gio 人工确认；仅在平台提供原生打印服务时可用。范围最多 5000 行、100 列和 500000 个单元格，可指定份数、页码范围及方向；未提交单元格草稿或文档修订变化会阻止打印。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatXLSX}, Risk: risk,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"工作表精确名称；建议直接使用 document.inspect 返回的名称，输入首尾空白会自动清理。"},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false,"description":"要打印的一基闭区间；最多 5000 行、100 列和 500000 个单元格。"},"orientation":{"type":"string","enum":["portrait","landscape"],"description":"页面方向，默认 portrait。"},"copies":{"type":"integer","minimum":1,"description":"打印份数，默认 1。"},"first_page":{"type":"integer","minimum":1,"description":"一基起始页；与 last_page 同时省略时打印全部页。"},"last_page":{"type":"integer","minimum":1,"description":"一基结束页；与 first_page 同时省略时打印全部页。"}},"required":["sheet","bounds"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","const":"sent_to_system_print_queue"},"printed_revision":{"type":"integer","minimum":0},"sheet":{"type":"string"},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1},"start_column":{"type":"integer","minimum":1},"end_row":{"type":"integer","minimum":1},"end_column":{"type":"integer","minimum":1}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"orientation":{"type":"string","enum":["portrait","landscape"]},"page_count":{"type":"integer","minimum":1},"copies":{"type":"integer","minimum":1},"first_page":{"type":"integer","minimum":1},"last_page":{"type":"integer","minimum":1}},"required":["status","printed_revision","sheet","bounds","orientation","page_count","copies","first_page","last_page"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			if printer == nil || !printer.Available() {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			var input struct {
				Sheet       string             `json:"sheet"`
				Bounds      document.CellRange `json:"bounds"`
				Orientation string             `json:"orientation"`
				Copies      int                `json:"copies"`
				FirstPage   int                `json:"first_page"`
				LastPage    int                `json:"last_page"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			input.Sheet = strings.TrimSpace(input.Sheet)
			rows := input.Bounds.EndRow - input.Bounds.StartRow + 1
			columns := input.Bounds.EndColumn - input.Bounds.StartColumn + 1
			if input.Sheet == "" || len([]rune(input.Sheet)) > 255 ||
				!validCellRange(input.Bounds, 500_000) || rows > 5000 || columns > 100 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if input.Orientation == "" {
				input.Orientation = "portrait"
			}
			controller, ok := capability.Adapter.(SpreadsheetPDFController)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			if drafts, ok := capability.Adapter.(interface{ HasPendingDrafts() bool }); ok && drafts.HasPendingDrafts() {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			window, err := controller.ReadSpreadsheetPDFWindow(ctx, input.Sheet, input.Bounds)
			if err != nil {
				return CapabilityResult{}, err
			}
			temporaryDirectory, err := os.MkdirTemp("", "glance-spreadsheet-print-")
			if err != nil {
				return CapabilityResult{}, err
			}
			defer os.RemoveAll(temporaryDirectory)
			pdfPath := filepath.Join(temporaryDirectory, "print.pdf")
			images := spreadsheetPDFImageSource{ctx: ctx, controller: controller}
			export, err := sheetpdf.ExportWindow(ctx, pdfPath, input.Sheet, window, input.Orientation, images, nil)
			if err != nil {
				return CapabilityResult{}, err
			}
			firstPage, lastPage, err := normalizePrintPageRange(input.FirstPage, input.LastPage, export.PageCount)
			if err != nil {
				return CapabilityResult{}, err
			}
			if input.Copies == 0 {
				input.Copies = 1
			}
			pdfData, err := os.ReadFile(pdfPath)
			if err != nil {
				return CapabilityResult{}, err
			}
			release, err := capability.Runtime.lockDocumentRevision(ctx, capability.Document.DocumentID, capability.Document.Revision)
			if err != nil {
				return CapabilityResult{}, err
			}
			release()
			if err := printer.PrintPDF(ctx, pdfData, PrintOptions{
				Copies: input.Copies, FirstPage: firstPage, LastPage: lastPage,
			}); err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Status          string             `json:"status"`
				PrintedRevision uint64             `json:"printed_revision"`
				Sheet           string             `json:"sheet"`
				Bounds          document.CellRange `json:"bounds"`
				Orientation     string             `json:"orientation"`
				PageCount       int                `json:"page_count"`
				Copies          int                `json:"copies"`
				FirstPage       int                `json:"first_page"`
				LastPage        int                `json:"last_page"`
			}{
				Status: "sent_to_system_print_queue", PrintedRevision: capability.Document.Revision,
				Sheet: input.Sheet, Bounds: input.Bounds, Orientation: input.Orientation,
				PageCount: export.PageCount, Copies: input.Copies, FirstPage: firstPage, LastPage: lastPage,
			}, false)
		},
	})
}
