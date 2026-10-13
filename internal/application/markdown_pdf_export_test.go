package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"
)

type testMarkdownpdfTargetPathArguments struct {
	TargetPath string `json:"target_path"`
}

type testMarkdownpdfTargetPathOrientationArguments struct {
	TargetPath  string `json:"target_path"`
	Orientation string `json:"orientation"`
}

func TestMarkdownExportPDFWritesValidatedNewFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "weekly.md")
	if err := os.WriteFile(source, []byte("# 周报\n\n本周完成 MCP 能力接入。\n\n![团队图表](missing-chart.png)\n"), 0o600); err != nil {
		t.Fatalf("write Markdown source: %v", err)
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, source)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	path := filepath.Join(dir, "weekly.pdf")
	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "markdown-pdf-export", DocumentID: doc.DocumentID,
		CapabilityID: "markdown.document.export_pdf", ExpectedRevision: doc.Revision,
		Operator:  Operator{ID: "markdown-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, testMarkdownpdfTargetPathOrientationArguments{TargetPath: path, Orientation: "landscape"}),
	})
	if err != nil {
		t.Fatalf("InvokeCapability(markdown.document.export_pdf): %v", err)
	}
	var output struct {
		Path              string   `json:"path"`
		BytesWritten      int64    `json:"bytes_written"`
		PageCount         int      `json:"page_count"`
		MissingImageCount int      `json:"missing_image_count"`
		ResourceWarnings  []string `json:"resource_warnings"`
		Diagnostics       []struct {
			Severity string `json:"severity"`
			SourceID string `json:"source_id"`
			Code     string `json:"code"`
			Detail   string `json:"detail"`
		} `json:"diagnostics"`
		DiagnosticsTruncated bool `json:"diagnostics_truncated"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode Markdown PDF result: %v; data=%s", err, result.Data)
	}
	if output.Path != canonicalPath(path) || output.BytesWritten <= 0 || output.PageCount < 1 || output.MissingImageCount != 1 || len(output.ResourceWarnings) != 1 || !strings.Contains(output.ResourceWarnings[0], "团队图表") || output.DiagnosticsTruncated {
		t.Fatalf("Markdown PDF result = %+v", output)
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
	if after.Revision != doc.Revision || after.Dirty != doc.Dirty || after.SourcePath != doc.SourcePath {
		t.Fatalf("PDF export changed Markdown session: before=%+v after=%+v", doc, after)
	}

	t.Run("rejects existing target", func(t *testing.T) {
		existing := filepath.Join(dir, "existing.pdf")
		if err := os.WriteFile(existing, []byte("preserve me"), 0o600); err != nil {
			t.Fatalf("create existing target: %v", err)
		}
		_, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "markdown-pdf-existing", DocumentID: doc.DocumentID,
			CapabilityID: "markdown.document.export_pdf", ExpectedRevision: doc.Revision,
			Operator:  Operator{ID: "markdown-agent", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, testMarkdownpdfTargetPathArguments{TargetPath: existing}),
		})
		if !errors.Is(err, ErrTargetExists) {
			t.Fatalf("existing PDF error = %v; want ErrTargetExists", err)
		}
		contents, readErr := os.ReadFile(existing)
		if readErr != nil || string(contents) != "preserve me" {
			t.Fatalf("existing PDF changed: contents=%q error=%v", contents, readErr)
		}
	})
	t.Run("rejects wrong extension", func(t *testing.T) {
		_, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "markdown-pdf-wrong-extension", DocumentID: doc.DocumentID,
			CapabilityID: "markdown.document.export_pdf", ExpectedRevision: doc.Revision,
			Operator:  Operator{ID: "markdown-agent", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, testMarkdownpdfTargetPathArguments{TargetPath: filepath.Join(dir, "wrong.md")}),
		})
		if !errors.Is(err, ErrInvalidCommandArguments) {
			t.Fatalf("wrong extension error = %v; want ErrInvalidCommandArguments", err)
		}
	})
}
