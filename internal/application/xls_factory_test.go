package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type declaredSourceFormatAdapter struct {
	*testDocumentAdapter
	source Format
}

func (a *declaredSourceFormatAdapter) SourceFormat() Format { return a.source }

func TestOpenXLSFactoryReturnsEditableXLSXAdapter(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "format", "xls", "testdata", "formula-style-multisheet.xls"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "legacy.xls")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := (builtInFormatFactory{format: FormatXLS}).Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open XLS factory: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close(context.Background()) })
	if adapter.Format() != FormatXLSX || adapter.ReadOnly() {
		t.Fatalf("adapter = format %q, readOnly %v; want editable XLSX", adapter.Format(), adapter.ReadOnly())
	}
	if adapter.Path() != path {
		t.Fatalf("adapter source path = %q; want original XLS path %q", adapter.Path(), path)
	}
	declared, ok := adapter.(SourceFormatAdapter)
	if !ok || declared.SourceFormat() != FormatXLS {
		t.Fatalf("adapter does not declare XLS source format: %T", adapter)
	}
	if err := adapter.Save(context.Background()); err == nil || !strings.Contains(err.Error(), ".xlsx") {
		t.Fatalf("converted workbook Save error = %v; want XLSX-only Save As guidance", err)
	}
	output := filepath.Join(t.TempDir(), "converted.xlsx")
	if err := adapter.SaveAs(context.Background(), output); err != nil {
		t.Fatalf("save as XLSX: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("saving imported workbook changed the original XLS bytes")
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("XLSX output was not created: %v", err)
	}
}

func TestImportedXLSRuntimeSaveAsRejectsXLS(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join("..", "format", "xls", "testdata", "formula-style-multisheet.xls"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "legacy.xls")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{builtInFormatFactory{format: FormatXLS}}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument legacy XLS: %v", err)
	}
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, filepath.Join(t.TempDir(), "converted.xls")); err == nil {
		t.Fatal("Runtime.SaveDocumentAs accepted an XLS target for imported XLSX session")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("rejecting XLS Save As changed the source file")
	}
}
