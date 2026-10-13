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

type testWordpdfTargetPathArguments struct {
	TargetPath string `json:"target_path"`
}

type testWordpdfTargetPathOrientationArguments struct {
	TargetPath  string `json:"target_path"`
	Orientation string `json:"orientation"`
}

type testWordpdfTextArguments struct {
	Text string `json:"text"`
}

func TestWordExportPDFWritesValidatedNewFileWithoutChangingSession(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	command, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "append-before-pdf-export", DocumentID: doc.DocumentID,
		CommandID: "word.paragraph.append", ExpectedRevision: doc.Revision,
		Operator:  Operator{ID: "pdf-export-agent", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, testWordpdfTextArguments{Text: "周报汇总：项目进展正常。"}),
	})
	if err != nil {
		t.Fatalf("append Word paragraph: %v", err)
	}
	doc, err = runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before export: %v", err)
	}
	before := doc
	path := filepath.Join(t.TempDir(), "weekly-report.pdf")

	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "export-weekly-report-pdf", DocumentID: doc.DocumentID,
		CapabilityID: "word.document.export_pdf", ExpectedRevision: command.Revision,
		Operator:  Operator{ID: "pdf-export-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, testWordpdfTargetPathOrientationArguments{TargetPath: path, Orientation: "landscape"}),
	})
	if err != nil {
		t.Fatalf("InvokeCapability(word.document.export_pdf): %v", err)
	}
	var output struct {
		Path         string `json:"path"`
		BytesWritten int64  `json:"bytes_written"`
		PageCount    int    `json:"page_count"`
		Diagnostics  []struct {
			Severity string `json:"severity"`
			SourceID string `json:"source_id"`
			Code     string `json:"code"`
			Detail   string `json:"detail"`
		} `json:"diagnostics"`
		DiagnosticsTruncated bool `json:"diagnostics_truncated"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode PDF export result: %v; data=%s", err, result.Data)
	}
	if output.Path != canonicalPath(path) || output.BytesWritten <= 0 || output.PageCount < 1 || output.DiagnosticsTruncated {
		t.Fatalf("PDF export result = %+v", output)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != output.BytesWritten {
		t.Fatalf("PDF output stat = %v, error=%v; result bytes=%d", info, err, output.BytesWritten)
	}
	reader, err := gopdf.Open(path)
	if err != nil {
		t.Fatalf("open exported PDF: %v", err)
	}
	if reader.Repaired() || reader.NumPages() != output.PageCount {
		t.Fatalf("exported PDF repaired=%v pages=%d; result pages=%d", reader.Repaired(), reader.NumPages(), output.PageCount)
	}
	pageSize, err := reader.PageSize(0)
	if err != nil || pageSize.W <= pageSize.H {
		t.Fatalf("exported landscape page size = %+v, error=%v", pageSize, err)
	}
	after, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument after export: %v", err)
	}
	if after.Revision != before.Revision || after.Dirty != before.Dirty || after.SourcePath != before.SourcePath {
		t.Fatalf("PDF export changed source session: before=%+v after=%+v", before, after)
	}

	t.Run("rejects existing target", func(t *testing.T) {
		existing := filepath.Join(t.TempDir(), "existing.pdf")
		if err := os.WriteFile(existing, []byte("preserve me"), 0o600); err != nil {
			t.Fatalf("create existing target: %v", err)
		}
		_, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "export-over-existing-pdf", DocumentID: doc.DocumentID,
			CapabilityID: "word.document.export_pdf", ExpectedRevision: before.Revision,
			Operator:  Operator{ID: "pdf-export-agent", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, testWordpdfTargetPathArguments{TargetPath: existing}),
		})
		if !errors.Is(err, ErrTargetExists) {
			t.Fatalf("export overwrite error = %v; want ErrTargetExists", err)
		}
		contents, readErr := os.ReadFile(existing)
		if readErr != nil || string(contents) != "preserve me" {
			t.Fatalf("existing PDF changed: contents=%q error=%v", contents, readErr)
		}
	})

	t.Run("rejects wrong extension", func(t *testing.T) {
		_, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "export-word-with-wrong-extension", DocumentID: doc.DocumentID,
			CapabilityID: "word.document.export_pdf", ExpectedRevision: before.Revision,
			Operator:  Operator{ID: "pdf-export-agent", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, testWordpdfTargetPathArguments{TargetPath: filepath.Join(t.TempDir(), "wrong.docx")}),
		})
		if !errors.Is(err, ErrInvalidCommandArguments) {
			t.Fatalf("wrong extension error = %v; want ErrInvalidCommandArguments", err)
		}
	})
}

func TestWordPDFCommitGuardRejectsPendingDraft(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	adapter := &pendingDraftDocumentAdapter{pending: true}
	if _, err := runtime.RegisterLiveSession(ctx, "doc-with-new-draft", 7, adapter); err != nil {
		t.Fatalf("RegisterLiveSession: %v", err)
	}
	_, err = runtime.lockDocumentRevision(ctx, "doc-with-new-draft", 7)
	if !errors.Is(err, ErrPendingEditorDraft) {
		t.Fatalf("document commit guard error = %v; want ErrPendingEditorDraft", err)
	}
}

type pendingDraftDocumentAdapter struct{ pending bool }

func (a *pendingDraftDocumentAdapter) Format() Format                       { return FormatDocx }
func (a *pendingDraftDocumentAdapter) Path() string                         { return "" }
func (a *pendingDraftDocumentAdapter) Dirty() bool                          { return false }
func (a *pendingDraftDocumentAdapter) ReadOnly() bool                       { return false }
func (a *pendingDraftDocumentAdapter) Save(context.Context) error           { return nil }
func (a *pendingDraftDocumentAdapter) SaveAs(context.Context, string) error { return nil }
func (a *pendingDraftDocumentAdapter) Close(context.Context) error          { return nil }
func (a *pendingDraftDocumentAdapter) HasPendingDrafts() bool               { return a.pending }
