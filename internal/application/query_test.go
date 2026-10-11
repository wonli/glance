package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
)

type inspectableTestAdapter struct {
	*testDocumentAdapter
	supportsInspection bool
	supportsFilters    bool
	payload            json.RawMessage
	diagnostics        []Diagnostic
	inspectErr         error
}

func (a *inspectableTestAdapter) Inspect(ctx context.Context, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if a.inspectErr != nil {
		return nil, nil, a.inspectErr
	}
	if !a.supportsInspection {
		return nil, nil, ErrQueryNotSupported
	}
	if !a.supportsFilters && (request.Sheet != "" || request.Range != nil) {
		return nil, nil, ErrQueryFilterNotSupported
	}
	return append(json.RawMessage(nil), a.payload...), append([]Diagnostic(nil), a.diagnostics...), nil
}

func TestQueryServiceInspectsLiveSessionAndReturnsCurrentMetadata(t *testing.T) {
	ctx := context.Background()
	adapter := &inspectableTestAdapter{
		testDocumentAdapter: &testDocumentAdapter{format: FormatJSON, dirty: true},
		supportsInspection:  true,
		payload:             json.RawMessage(`{"text":"live draft"}`),
		diagnostics:         []Diagnostic{{Code: "partial", Message: "some content is unsupported"}},
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{&testFormatFactory{format: FormatJSON, openDoc: adapter}}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "live.json"))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	if _, err := runtime.MarkDocumentChanged(ctx, doc.DocumentID); err != nil {
		t.Fatalf("MarkDocumentChanged: %v", err)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID})
	if err != nil {
		t.Fatalf("InspectDocument: %v", err)
	}
	if snapshot.Info.Revision != doc.Revision+1 || !snapshot.Info.Dirty {
		t.Fatalf("snapshot metadata is stale: %+v", snapshot.Info)
	}
	if string(snapshot.Content) != string(adapter.payload) || len(snapshot.Diagnostics) != 1 {
		t.Fatalf("unexpected snapshot content/diagnostics: %s %+v", snapshot.Content, snapshot.Diagnostics)
	}
}

func TestQueryServiceRejectsUnknownDocumentsAndUnsupportedFormats(t *testing.T) {
	ctx := context.Background()
	unsupported := &inspectableTestAdapter{testDocumentAdapter: &testDocumentAdapter{format: FormatJSON}}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{&testFormatFactory{format: FormatJSON, openDoc: unsupported}}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	if _, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: "unknown"}); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("unknown document error = %v, want ErrDocumentNotFound", err)
	}
	doc, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "unsupported.json"))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	if _, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID}); !errors.Is(err, ErrQueryNotSupported) {
		t.Fatalf("unsupported query error = %v, want ErrQueryNotSupported", err)
	}
}

func TestQueryServiceRejectsInapplicableFilters(t *testing.T) {
	ctx := context.Background()
	adapter := &inspectableTestAdapter{
		testDocumentAdapter: &testDocumentAdapter{format: FormatJSON},
		supportsInspection:  true,
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{&testFormatFactory{format: FormatJSON, openDoc: adapter}}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "filtered.json"))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	_, err = runtime.Queries().InspectDocument(ctx, InspectRequest{
		DocumentID: doc.DocumentID,
		Sheet:      "Sheet1",
	})
	if !errors.Is(err, ErrQueryFilterNotSupported) {
		t.Fatalf("InspectDocument error = %v, want ErrQueryFilterNotSupported", err)
	}
}

func TestQueryServiceHonorsCancellation(t *testing.T) {
	ctx := context.Background()
	adapter := &inspectableTestAdapter{
		testDocumentAdapter: &testDocumentAdapter{format: FormatJSON},
		supportsInspection:  true,
		payload:             json.RawMessage(`{}`),
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{&testFormatFactory{format: FormatJSON, openDoc: adapter}}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "cancel.json"))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := runtime.Queries().InspectDocument(cancelled, InspectRequest{DocumentID: doc.DocumentID}); !errors.Is(err, context.Canceled) {
		t.Fatalf("InspectDocument error = %v, want context.Canceled", err)
	}
}

func TestQueryServiceEnforcesBoundsAndSerializedByteLimit(t *testing.T) {
	ctx := context.Background()
	adapter := &inspectableTestAdapter{
		testDocumentAdapter: &testDocumentAdapter{format: FormatJSON},
		supportsInspection:  true,
		payload:             json.RawMessage(`{"text":"0123456789"}`),
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{&testFormatFactory{format: FormatJSON, openDoc: adapter}}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "bounded.json"))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	query := runtime.Queries()
	for name, request := range map[string]InspectRequest{
		"negative start":       {DocumentID: doc.DocumentID, Start: -1},
		"oversized limit":      {DocumentID: doc.DocumentID, Limit: MaxQueryItems + 1},
		"oversized bytes":      {DocumentID: doc.DocumentID, MaxBytes: MaxQueryResultBytes + 1},
		"oversized offset":     {DocumentID: doc.DocumentID, Start: MaxQueryOffset + 1},
		"invalid range":        {DocumentID: doc.DocumentID, Range: &document.CellRange{StartRow: 0, StartColumn: 1, EndRow: 1, EndColumn: 1}},
		"large range":          {DocumentID: doc.DocumentID, Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1000, EndColumn: 1000}},
		"range and pagination": {DocumentID: doc.DocumentID, Start: 1, Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := query.InspectDocument(ctx, request); !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("InspectDocument error = %v, want ErrInvalidQuery", err)
			}
		})
	}
	if _, err := query.InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, MaxBytes: 4}); !errors.Is(err, ErrQueryResultTooLarge) {
		t.Fatalf("oversized result error = %v, want ErrQueryResultTooLarge", err)
	}
}

func TestQueryServiceReadsBuiltInJSONDocumentWithPagination(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte(`{"a":1,"b":{"value":2},"c":3}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Start: 1, Limit: 1})
	if err != nil {
		t.Fatalf("InspectDocument: %v", err)
	}
	var page jsonObjectPage
	if err := json.Unmarshal(snapshot.Content, &page); err != nil {
		t.Fatalf("unmarshal query content: %v", err)
	}
	if page.Total != 3 || page.Start != 1 || len(page.Items) != 1 || page.Items[0].Key != "b" || string(page.Items[0].Value) != `{"value":2}` {
		t.Fatalf("unexpected paged object result: %+v", page)
	}
	if snapshot.Info.DocumentID != doc.DocumentID || snapshot.Info.Format != FormatJSON {
		t.Fatalf("unexpected metadata: %+v", snapshot.Info)
	}
	if _, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Sheet: "Sheet1"}); !errors.Is(err, ErrQueryFilterNotSupported) {
		t.Fatalf("JSON sheet filter error = %v, want ErrQueryFilterNotSupported", err)
	}
}

func TestQueryServiceReadsBoundedSpreadsheetRange(t *testing.T) {
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
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{
		DocumentID: doc.DocumentID,
		Sheet:      "Sheet1",
		Range:      &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2},
	})
	if err != nil {
		t.Fatalf("InspectDocument: %v", err)
	}
	if len(snapshot.Content) == 0 || !strings.Contains(string(snapshot.Content), `"start_row":1`) || !strings.Contains(string(snapshot.Content), `"start_column":1`) {
		t.Fatalf("unexpected spreadsheet query content: %s", snapshot.Content)
	}
}

func TestBuiltInFormatInspectorsReturnValidBoundedContent(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	for _, format := range []Format{FormatDocx, FormatXLSX, FormatPPTX, FormatPDF, FormatJSON, FormatMarkdown} {
		t.Run(string(format), func(t *testing.T) {
			doc, err := runtime.CreateDocument(ctx, format)
			if err != nil {
				t.Fatalf("CreateDocument(%s): %v", format, err)
			}
			snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 2})
			if err != nil {
				t.Fatalf("InspectDocument(%s): %v", format, err)
			}
			if !json.Valid(snapshot.Content) || len(snapshot.Content) > MaxQueryResultBytes {
				t.Fatalf("invalid or unbounded %s query content (%d bytes): %s", format, len(snapshot.Content), snapshot.Content)
			}
		})
	}
}
