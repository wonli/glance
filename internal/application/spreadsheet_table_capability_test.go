package application

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
)

func TestSpreadsheetTableCapabilityCreatesExplicitTableAndRoundTrips(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument(XLSX): %v", err)
	}
	documentID := doc.DocumentID
	requestNumber := 0
	invoke := func(capabilityID string, arguments any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		info := mustGetDocumentInfo(t, runtime, documentID)
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("xlsx-table-%d", requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "table-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 2}
	seed := []map[string]any{
		{"row": 1, "column": 1, "value": "季度"}, {"row": 1, "column": 2, "value": "营收"},
		{"row": 2, "column": 1, "value": "第一季度"}, {"row": 2, "column": 2, "value": "120"},
		{"row": 3, "column": 1, "value": "第二季度"}, {"row": 3, "column": 2, "value": "150"},
	}
	if _, err := invoke("spreadsheet.cells.set", map[string]any{"sheet": "Sheet1", "edits": seed}); err != nil {
		t.Fatalf("seed table source: %v", err)
	}
	arguments := map[string]any{"sheet": "Sheet1", "bounds": bounds, "name": "RevenueTable", "style_name": "TableStyleMedium2"}
	invalid := map[string]any{"sheet": "Missing", "bounds": bounds, "name": "RevenueTable", "style_name": "TableStyleMedium2"}
	beforeInvalid := mustGetDocumentInfo(t, runtime, documentID)
	if _, err := invoke("spreadsheet.table.create", invalid); err == nil {
		t.Fatal("table creation accepted a missing worksheet")
	}
	if afterInvalid := mustGetDocumentInfo(t, runtime, documentID); afterInvalid.Revision != beforeInvalid.Revision || afterInvalid.Dirty != beforeInvalid.Dirty {
		t.Fatalf("invalid worksheet partially changed workbook: before=%+v after=%+v", beforeInvalid, afterInvalid)
	}
	beforeCreate := mustGetDocumentInfo(t, runtime, documentID)
	result, err := invoke("spreadsheet.table.create", arguments)
	if err != nil {
		t.Fatalf("InvokeCapability(spreadsheet.table.create): %v", err)
	}
	var output struct {
		Sheet  string `json:"sheet"`
		Bounds struct {
			StartRow    int `json:"start_row"`
			StartColumn int `json:"start_column"`
			EndRow      int `json:"end_row"`
			EndColumn   int `json:"end_column"`
		} `json:"bounds"`
		Name      string `json:"name"`
		StyleName string `json:"style_name"`
		Changed   bool   `json:"changed"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode table result: %v; data=%s", err, result.Data)
	}
	if output.Sheet != "Sheet1" || output.Bounds.StartRow != 1 || output.Bounds.StartColumn != 1 || output.Bounds.EndRow != 3 || output.Bounds.EndColumn != 2 || output.Name != "RevenueTable" || output.StyleName != "TableStyleMedium2" || !output.Changed {
		t.Fatalf("table result = %+v; data=%s", output, result.Data)
	}
	afterCreate := mustGetDocumentInfo(t, runtime, documentID)
	if result.Revision != beforeCreate.Revision+1 || afterCreate.Revision != result.Revision || !afterCreate.Dirty {
		t.Fatalf("table creation state: before=%+v result=%+v after=%+v", beforeCreate, result, afterCreate)
	}
	beforeDuplicate := mustGetDocumentInfo(t, runtime, documentID)
	if _, err := invoke("spreadsheet.table.create", arguments); err == nil {
		t.Fatal("duplicate table name unexpectedly created another table")
	}
	if afterDuplicate := mustGetDocumentInfo(t, runtime, documentID); afterDuplicate.Revision != beforeDuplicate.Revision || afterDuplicate.Dirty != beforeDuplicate.Dirty {
		t.Fatalf("duplicate table request partially changed workbook: before=%+v after=%+v", beforeDuplicate, afterDuplicate)
	}

	path := filepath.Join(t.TempDir(), "revenue-table.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after table creation: %v", err)
	}
	if reopened.Dirty {
		t.Fatalf("reopened workbook is dirty: %+v", reopened)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open saved XLSX: %v", err)
	}
	defer archive.Close()
	var tableXML string
	for _, file := range archive.File {
		if !strings.HasPrefix(file.Name, "xl/tables/table") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read %s: read=%v close=%v", file.Name, readErr, closeErr)
		}
		tableXML += string(data)
	}
	for _, want := range []string{`name="RevenueTable"`, `displayName="RevenueTable"`, `ref="A1:B3"`} {
		if !strings.Contains(tableXML, want) {
			t.Fatalf("saved structured table is missing %q; table parts=%s", want, tableXML)
		}
	}
}
