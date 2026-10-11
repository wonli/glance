package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"
	"glance/internal/document"
)

func TestSpreadsheetPrintCapabilityRequiresHumanAndPreservesWorkbook(t *testing.T) {
	ctx := context.Background()
	printer := &capturingPrintService{}
	runtime, err := NewRuntime(ctx, RuntimeOptions{PrintService: printer})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument(XLSX): %v", err)
	}
	seedInfo := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	seed := []map[string]any{
		{"row": 1, "column": 1, "value": "部门"}, {"row": 1, "column": 2, "value": "人数"},
		{"row": 2, "column": 1, "value": "研发"}, {"row": 2, "column": 2, "value": "12"},
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "spreadsheet-print-seed", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.cells.set",
		ExpectedRevision: seedInfo.Revision, Operator: Operator{ID: "print-agent-test", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"sheet": "Sheet1", "edits": seed}),
	}); err != nil {
		t.Fatalf("seed print range: %v", err)
	}
	before := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2}
	request := CapabilityRequest{
		RequestID: "spreadsheet-print-agent-request", DocumentID: doc.DocumentID,
		CapabilityID: "spreadsheet.document.print", ExpectedRevision: before.Revision,
		Operator: Operator{ID: "print-agent-test", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{
			"sheet": "Sheet1", "bounds": bounds, "orientation": "landscape", "copies": 2, "first_page": 1, "last_page": 1,
		}),
	}
	pending, err := runtime.InvokeCapability(ctx, request)
	if err != nil {
		t.Fatalf("stage spreadsheet print: %v", err)
	}
	var staged struct {
		Status         string `json:"status"`
		ConfirmationID string `json:"confirmation_id"`
	}
	if err := json.Unmarshal(pending.Data, &staged); err != nil || staged.Status != "confirmation_required" || staged.ConfirmationID == "" {
		t.Fatalf("Agent print did not await human approval: %+v data=%s err=%v", staged, pending.Data, err)
	}
	if printer.Calls != 0 {
		t.Fatalf("printer was called before approval: %d", printer.Calls)
	}
	if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, Operator{ID: "human-reviewer", Kind: OperatorHuman}); err != nil {
		t.Fatalf("approve spreadsheet print: %v", err)
	}
	completed := waitForConfirmationStatus(t, runtime, staged.ConfirmationID, ConfirmationCompleted)
	if completed.Result == nil {
		t.Fatalf("approved print returned no result: %+v", completed)
	}
	var output struct {
		Status          string             `json:"status"`
		PrintedRevision uint64             `json:"printed_revision"`
		Sheet           string             `json:"sheet"`
		Bounds          document.CellRange `json:"bounds"`
		Orientation     string             `json:"orientation"`
		PageCount       int                `json:"page_count"`
		Copies          int                `json:"copies"`
		FirstPage       int                `json:"first_page"`
		LastPage        int                `json:"last_page"`
	}
	if err := json.Unmarshal(completed.Result.Data, &output); err != nil {
		t.Fatalf("decode print result: %v; data=%s", err, completed.Result.Data)
	}
	if output.Status != "sent_to_system_print_queue" || output.PrintedRevision != before.Revision || output.Sheet != "Sheet1" || output.Bounds != bounds || output.Orientation != "landscape" || output.PageCount < 1 || output.Copies != 2 || output.FirstPage != 1 || output.LastPage != 1 {
		t.Fatalf("approved spreadsheet print result = %+v", output)
	}
	if printer.Calls != 1 || printer.Options != (PrintOptions{Copies: 2, FirstPage: 1, LastPage: 1}) || len(printer.Data) == 0 {
		t.Fatalf("printer calls=%d options=%+v PDF bytes=%d", printer.Calls, printer.Options, len(printer.Data))
	}
	pdfPath := filepath.Join(t.TempDir(), "captured-spreadsheet-print.pdf")
	if err := os.WriteFile(pdfPath, printer.Data, 0o600); err != nil {
		t.Fatalf("write captured PDF: %v", err)
	}
	pdf, err := gopdf.Open(pdfPath)
	if err != nil || pdf.Repaired() || pdf.NumPages() != output.PageCount {
		t.Fatalf("captured PDF pages=%v repaired=%v error=%v", pdf, pdf != nil && pdf.Repaired(), err)
	}
	if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, Operator{ID: "human-reviewer", Kind: OperatorHuman}); !errors.Is(err, ErrConfirmationNotPending) {
		t.Fatalf("repeat print approval error=%v; want ErrConfirmationNotPending", err)
	}
	after := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if after.Revision != before.Revision || after.Dirty != before.Dirty || completed.Result.Revision != before.Revision {
		t.Fatalf("printing changed workbook state: before=%+v after=%+v result=%+v", before, after, completed.Result)
	}
}
