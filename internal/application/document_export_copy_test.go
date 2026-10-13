package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/format/docx"
	"glance/internal/format/xlsx"
)

type testDocumentexportPathArguments struct {
	Path string `json:"path"`
}

type testDocumentexportEnabledArguments struct {
	Enabled bool `json:"enabled"`
}

func TestDocumentExportCopyPreservesLiveSessionAndRejectsOverwrite(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	for _, format := range []Format{FormatDocx, FormatXLSX} {
		t.Run(string(format), func(t *testing.T) {
			doc, err := runtime.CreateDocument(ctx, format)
			if err != nil {
				t.Fatalf("CreateDocument(%s): %v", format, err)
			}
			if format == FormatDocx {
				_, err := runtime.Commands().executeApproved(ctx, CommandRequest{
					RequestID: "protect-before-copy", DocumentID: doc.DocumentID, CommandID: "word.document.protection.set",
					ExpectedRevision: doc.Revision, Operator: Operator{ID: "human-1", Kind: OperatorHuman},
					Arguments: mustJSON(t, testDocumentexportEnabledArguments{Enabled: true}),
				})
				if err != nil {
					t.Fatalf("protect source document: %v", err)
				}
				doc, err = runtime.GetDocument(ctx, doc.DocumentID)
				if err != nil {
					t.Fatalf("GetDocument after protection: %v", err)
				}
			}
			before := doc
			path := filepath.Join(t.TempDir(), "shared."+string(format))
			result, err := runtime.Commands().Execute(ctx, CommandRequest{
				RequestID: "copy-" + string(format), DocumentID: doc.DocumentID, CommandID: "document.export_copy",
				ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
				Arguments: mustJSON(t, testDocumentexportPathArguments{Path: path}),
			})
			if err != nil {
				t.Fatalf("export copy: %v", err)
			}
			var output struct {
				Path         string `json:"path"`
				BytesWritten int64  `json:"bytes_written"`
			}
			if err := json.Unmarshal(result.Data, &output); err != nil {
				t.Fatalf("decode export copy result: %v", err)
			}
			if output.Path != canonicalPath(path) || output.BytesWritten <= 0 {
				t.Fatalf("copy result = %+v; want path %q and nonempty output", output, canonicalPath(path))
			}
			after, err := runtime.GetDocument(ctx, doc.DocumentID)
			if err != nil {
				t.Fatalf("GetDocument after copy: %v", err)
			}
			if after.SourcePath != before.SourcePath || after.Revision != before.Revision || after.Dirty != before.Dirty {
				t.Fatalf("copy changed live session: before=%+v after=%+v", before, after)
			}
			switch format {
			case FormatDocx:
				copyDoc, err := docx.Open(path)
				if err != nil {
					t.Fatalf("open exported DOCX: %v", err)
				}
				protected, err := copyDoc.GetWordDocumentProtection()
				if err != nil || !protected {
					t.Fatalf("exported protected DOCX state = %v, %v", protected, err)
				}
				_ = copyDoc.Close()
			case FormatXLSX:
				copyBook, err := xlsx.Open(path)
				if err != nil {
					t.Fatalf("open exported XLSX: %v", err)
				}
				_ = copyBook.Close()
			}
		})
	}

	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument for overwrite check: %v", err)
	}
	existing := filepath.Join(t.TempDir(), "existing.docx")
	if err := os.WriteFile(existing, []byte("preserve me"), 0o600); err != nil {
		t.Fatalf("create existing target: %v", err)
	}
	_, err = runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "copy-overwrite", DocumentID: doc.DocumentID, CommandID: "document.export_copy",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, testDocumentexportPathArguments{Path: existing}),
	})
	if !errors.Is(err, ErrTargetExists) {
		t.Fatalf("export copy overwrite error = %v; want ErrTargetExists", err)
	}
	contents, readErr := os.ReadFile(existing)
	if readErr != nil || string(contents) != "preserve me" {
		t.Fatalf("existing target changed: contents=%q error=%v", contents, readErr)
	}
}
