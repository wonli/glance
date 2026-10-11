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

func TestSpreadsheetExportPDFWritesBoundedLiveWindowWithoutChangingWorkbook(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	cell, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "spreadsheet-pdf-cell", DocumentID: doc.DocumentID, CommandID: "spreadsheet.cell.set",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "sheet-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"sheet": "Sheet1", "cell": "A1", "value": "工资汇总"}),
	})
	if err != nil {
		t.Fatalf("set PDF source cell: %v", err)
	}
	doc, err = runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before export: %v", err)
	}
	before := doc
	dir := t.TempDir()
	path := filepath.Join(dir, "payroll.pdf")
	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "spreadsheet-pdf-export", DocumentID: doc.DocumentID,
		CapabilityID: "spreadsheet.document.export_pdf", ExpectedRevision: cell.Revision,
		Operator: Operator{ID: "sheet-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{
			"sheet": "Sheet1", "bounds": map[string]int{"start_row": 1, "start_column": 1, "end_row": 3, "end_column": 4},
			"target_path": path, "orientation": "landscape",
		}),
	})
	if err != nil {
		t.Fatalf("InvokeCapability(spreadsheet.document.export_pdf): %v", err)
	}
	var output struct {
		Path         string `json:"path"`
		BytesWritten int64  `json:"bytes_written"`
		PageCount    int    `json:"page_count"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode spreadsheet PDF result: %v; data=%s", err, result.Data)
	}
	if output.Path != canonicalPath(path) || output.BytesWritten <= 0 || output.PageCount < 1 {
		t.Fatalf("spreadsheet PDF result = %+v", output)
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
		t.Fatalf("spreadsheet PDF export changed source session: before=%+v after=%+v", before, after)
	}

	t.Run("rejects oversized range", func(t *testing.T) {
		_, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "spreadsheet-pdf-too-large", DocumentID: doc.DocumentID,
			CapabilityID: "spreadsheet.document.export_pdf", ExpectedRevision: doc.Revision,
			Operator: Operator{ID: "sheet-agent", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, map[string]any{
				"sheet": "Sheet1", "bounds": map[string]int{"start_row": 1, "start_column": 1, "end_row": 5001, "end_column": 100},
				"target_path": filepath.Join(dir, "too-large.pdf"),
			}),
		})
		if !errors.Is(err, ErrInvalidCommandArguments) {
			t.Fatalf("oversized range error = %v; want ErrInvalidCommandArguments", err)
		}
	})
	t.Run("rejects existing target without replacing it", func(t *testing.T) {
		existing := filepath.Join(dir, "existing.pdf")
		if err := os.WriteFile(existing, []byte("preserve me"), 0o600); err != nil {
			t.Fatalf("create existing target: %v", err)
		}
		_, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "spreadsheet-pdf-existing", DocumentID: doc.DocumentID,
			CapabilityID: "spreadsheet.document.export_pdf", ExpectedRevision: doc.Revision,
			Operator: Operator{ID: "sheet-agent", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, map[string]any{
				"sheet": "Sheet1", "bounds": map[string]int{"start_row": 1, "start_column": 1, "end_row": 3, "end_column": 4},
				"target_path": existing,
			}),
		})
		if !errors.Is(err, ErrTargetExists) {
			t.Fatalf("existing PDF error = %v; want ErrTargetExists", err)
		}
		contents, readErr := os.ReadFile(existing)
		if readErr != nil || string(contents) != "preserve me" {
			t.Fatalf("existing PDF changed: contents=%q error=%v", contents, readErr)
		}
	})
}
