package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"glance/internal/format/wordpdf"
)

type PrintOptions struct {
	Copies    int
	FirstPage int
	LastPage  int
}

// PrintService is the platform boundary for sending a rendered document PDF
// to a native print system. Headless runtimes may leave it unset.
type PrintService interface {
	Available() bool
	PrintPDF(context.Context, []byte, PrintOptions) error
}

func registerWordPrintCapability(catalog *Catalog, printer PrintService) error {
	risk := RiskHigh
	if printer == nil || !printer.Available() {
		risk = RiskLow
	}
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.document.print", Name: "打印 Word 文档",
			Description: "将指定修订版本的 DOCX 渲染为 PDF 并发送到桌面系统打印面板。此操作会产生实体打印副作用，MCP 调用必须由桌面人类确认；仅在已运行桌面且平台提供原生打印服务时可用。可指定份数和一基页码范围，未指定页码时打印全部页面；未提交编辑草稿或文档修订变化会阻止打印。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatDocx}, Risk: risk,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"copies":{"type":"integer","minimum":1},"first_page":{"type":"integer","minimum":1},"last_page":{"type":"integer","minimum":1}},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","const":"sent_to_system_print_queue"},"printed_revision":{"type":"integer","minimum":0},"page_count":{"type":"integer","minimum":1},"copies":{"type":"integer","minimum":1},"first_page":{"type":"integer","minimum":1},"last_page":{"type":"integer","minimum":1}},"required":["status","printed_revision","page_count","copies","first_page","last_page"],"additionalProperties":false}`),
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
				return CapabilityResult{}, err
			}
			reader, ok := capability.Adapter.(DocumentSnapshotReader)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			if drafts, ok := capability.Adapter.(interface{ HasPendingDrafts() bool }); ok && drafts.HasPendingDrafts() {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			snapshot, err := reader.Snapshot(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			current, err := capability.Runtime.GetDocument(ctx, capability.Document.DocumentID)
			if err != nil {
				return CapabilityResult{}, err
			}
			if current.Revision != capability.Document.Revision {
				return CapabilityResult{}, fmt.Errorf("%w: current=%d expected=%d", ErrRevisionConflict, current.Revision, capability.Document.Revision)
			}
			if drafts, ok := capability.Adapter.(interface{ HasPendingDrafts() bool }); ok && drafts.HasPendingDrafts() {
				return CapabilityResult{}, ErrPendingEditorDraft
			}

			temporaryDirectory, err := os.MkdirTemp("", "glance-word-print-")
			if err != nil {
				return CapabilityResult{}, err
			}
			defer os.RemoveAll(temporaryDirectory)
			pdfPath := filepath.Join(temporaryDirectory, "print.pdf")
			export, err := wordpdf.ExportSnapshot(ctx, pdfPath, snapshot, capability.Document.DocumentID, capability.Document.Revision, "document", nil)
			if err != nil {
				return CapabilityResult{}, err
			}
			if input.Copies == 0 {
				input.Copies = 1
			}
			firstPage, lastPage, err := normalizePrintPageRange(input.FirstPage, input.LastPage, export.PageCount)
			if err != nil {
				return CapabilityResult{}, err
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
			printErr := printer.PrintPDF(ctx, pdfData, PrintOptions{
				Copies: input.Copies, FirstPage: firstPage, LastPage: lastPage,
			})
			if printErr != nil {
				return CapabilityResult{}, printErr
			}
			return capabilityJSONResult(struct {
				Status          string `json:"status"`
				PrintedRevision uint64 `json:"printed_revision"`
				PageCount       int    `json:"page_count"`
				Copies          int    `json:"copies"`
				FirstPage       int    `json:"first_page"`
				LastPage        int    `json:"last_page"`
			}{
				Status: "sent_to_system_print_queue", PrintedRevision: capability.Document.Revision,
				PageCount: export.PageCount, Copies: input.Copies, FirstPage: firstPage, LastPage: lastPage,
			}, false)
		},
	})
}

func normalizePrintPageRange(firstPage, lastPage, pageCount int) (int, int, error) {
	if pageCount < 1 {
		return 0, 0, errors.New("print document has no pages")
	}
	if firstPage == 0 && lastPage == 0 {
		return 1, pageCount, nil
	}
	if firstPage < 1 || lastPage < 1 || firstPage > lastPage || lastPage > pageCount {
		return 0, 0, fmt.Errorf("invalid print page range %d-%d for %d-page document", firstPage, lastPage, pageCount)
	}
	return firstPage, lastPage, nil
}
