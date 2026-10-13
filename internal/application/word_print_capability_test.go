package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"
)

type testWordprintCopiesFirstPageArguments struct {
	Copies    int `json:"copies"`
	FirstPage int `json:"first_page"`
	LastPage  int `json:"last_page"`
}

type testWordprintTextArguments struct {
	Text string `json:"text"`
}

func TestWordPrintCapabilityRequiresHumanAndValidatesPages(t *testing.T) {
	ctx := context.Background()
	printer := &capturingPrintService{}
	runtime, err := NewRuntime(ctx, RuntimeOptions{PrintService: printer})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument(DOCX): %v", err)
	}
	seed, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-print-seed", DocumentID: doc.DocumentID, CapabilityID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "word-print-agent-test", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, testWordprintTextArguments{Text: "周报：本周完成工资表核对。"}),
	})
	if err != nil {
		t.Fatalf("seed Word document: %v", err)
	}
	before := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	stagePrint := func(requestID string, firstPage, lastPage int) string {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: doc.DocumentID, CapabilityID: "word.document.print",
			ExpectedRevision: before.Revision, Operator: Operator{ID: "word-print-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, testWordprintCopiesFirstPageArguments{Copies: 2, FirstPage: firstPage, LastPage: lastPage}),
		})
		if invokeErr != nil {
			t.Fatalf("stage Word print: %v", invokeErr)
		}
		var staged struct {
			Status         string `json:"status"`
			ConfirmationID string `json:"confirmation_id"`
		}
		if err := json.Unmarshal(result.Data, &staged); err != nil || staged.Status != "confirmation_required" || staged.ConfirmationID == "" {
			t.Fatalf("Word print did not require human confirmation: %+v data=%s err=%v", staged, result.Data, err)
		}
		return staged.ConfirmationID
	}
	approve := func(id string, want ConfirmationStatus) ConfirmationInfo {
		t.Helper()
		if err := runtime.ResolveConfirmation(ctx, id, true, Operator{ID: "human-reviewer", Kind: OperatorHuman}); err != nil {
			t.Fatalf("approve Word print: %v", err)
		}
		return waitForConfirmationStatus(t, runtime, id, want)
	}
	if printer.Calls != 0 {
		t.Fatalf("printer was called before human approval: %d", printer.Calls)
	}

	validID := stagePrint("word-print-approved", 1, 1)
	valid := approve(validID, ConfirmationCompleted)
	if valid.Result == nil {
		t.Fatalf("approved Word print returned no result: %+v", valid)
	}
	var output struct {
		Status          string `json:"status"`
		PrintedRevision uint64 `json:"printed_revision"`
		PageCount       int    `json:"page_count"`
		Copies          int    `json:"copies"`
		FirstPage       int    `json:"first_page"`
		LastPage        int    `json:"last_page"`
	}
	if err := json.Unmarshal(valid.Result.Data, &output); err != nil {
		t.Fatalf("decode Word print result: %v; data=%s", err, valid.Result.Data)
	}
	if output.Status != "sent_to_system_print_queue" || output.PrintedRevision != before.Revision || output.PageCount != 1 || output.Copies != 2 || output.FirstPage != 1 || output.LastPage != 1 {
		t.Fatalf("approved Word print result = %+v", output)
	}
	if printer.Calls != 1 || printer.Options != (PrintOptions{Copies: 2, FirstPage: 1, LastPage: 1}) || len(printer.Data) == 0 {
		t.Fatalf("Word printer calls=%d options=%+v PDF bytes=%d", printer.Calls, printer.Options, len(printer.Data))
	}
	pdfPath := filepath.Join(t.TempDir(), "captured-word-print.pdf")
	if err := os.WriteFile(pdfPath, printer.Data, 0o600); err != nil {
		t.Fatalf("write captured Word PDF: %v", err)
	}
	pdf, err := gopdf.Open(pdfPath)
	if err != nil || pdf.Repaired() || pdf.NumPages() != output.PageCount {
		t.Fatalf("captured Word PDF pages=%v repaired=%v error=%v", pdf, pdf != nil && pdf.Repaired(), err)
	}

	invalidID := stagePrint("word-print-invalid-page-range", 2, 3)
	failed := approve(invalidID, ConfirmationFailed)
	if failed.Error == "" || failed.Result != nil || printer.Calls != 1 {
		t.Fatalf("invalid Word page range state=%+v printer calls=%d; want failure without second print", failed, printer.Calls)
	}
	if err := runtime.ResolveConfirmation(ctx, validID, true, Operator{ID: "human-reviewer", Kind: OperatorHuman}); !errors.Is(err, ErrConfirmationNotPending) {
		t.Fatalf("repeat Word print approval error=%v; want ErrConfirmationNotPending", err)
	}
	after := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if after.Revision != before.Revision || after.Dirty != before.Dirty || valid.Result.Revision != before.Revision || seed.Revision != before.Revision {
		t.Fatalf("printing changed Word document state: seeded=%+v before=%+v after=%+v print=%+v", seed, before, after, valid.Result)
	}
}
