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

type testSpreadsheetpivotSheetBoundsArguments struct {
	Sheet           string             `json:"sheet"`
	Bounds          document.CellRange `json:"bounds"`
	PivotTableRange string             `json:"pivot_table_range"`
	Rows            []string           `json:"rows"`
	Columns         []string           `json:"columns"`
	Data            []string           `json:"data"`
}

func TestSpreadsheetPivotCapabilityUsesNamedFieldsAndRoundTrips(t *testing.T) {
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
			RequestID: fmt.Sprintf("xlsx-pivot-%d", requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "pivot-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}
	seed := []spreadsheetCellEditFixture{
		{Row: 1, Column: 1, Value: "部门"}, {Row: 1, Column: 2, Value: "季度"}, {Row: 1, Column: 3, Value: "销售额"},
		{Row: 2, Column: 1, Value: "研发"}, {Row: 2, Column: 2, Value: "Q1"}, {Row: 2, Column: 3, Value: "100"},
		{Row: 3, Column: 1, Value: "市场"}, {Row: 3, Column: 2, Value: "Q1"}, {Row: 3, Column: 3, Value: "200"},
		{Row: 4, Column: 1, Value: "研发"}, {Row: 4, Column: 2, Value: "Q2"}, {Row: 4, Column: 3, Value: "150"},
		{Row: 5, Column: 1, Value: "市场"}, {Row: 5, Column: 2, Value: "Q2"}, {Row: 5, Column: 3, Value: "250"},
	}
	if _, err := invoke("spreadsheet.cells.set", spreadsheetCellsSetPayload{Sheet: "Sheet1", Edits: seed}); err != nil {
		t.Fatalf("seed pivot source: %v", err)
	}
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 5, EndColumn: 3}
	arguments := testSpreadsheetpivotSheetBoundsArguments{Sheet: "Sheet1", Bounds: bounds, PivotTableRange: "F2:J15", Rows: []string{"部门"}, Columns: []string{"季度"}, Data: []string{"销售额"}}
	beforeInvalid := mustGetDocumentInfo(t, runtime, documentID)
	invalid := testSpreadsheetpivotSheetBoundsArguments{Sheet: "Sheet1", Bounds: bounds, PivotTableRange: "F2:J15", Rows: []string{"不存在"}, Columns: []string{"季度"}, Data: []string{"销售额"}}
	if _, err := invoke("spreadsheet.pivot.create", invalid); err == nil {
		t.Fatal("pivot creation accepted a field missing from the source header")
	}
	if afterInvalid := mustGetDocumentInfo(t, runtime, documentID); afterInvalid.Revision != beforeInvalid.Revision || afterInvalid.Dirty != beforeInvalid.Dirty {
		t.Fatalf("invalid pivot fields partially changed workbook: before=%+v after=%+v", beforeInvalid, afterInvalid)
	}
	beforeCreate := mustGetDocumentInfo(t, runtime, documentID)
	result, err := invoke("spreadsheet.pivot.create", arguments)
	if err != nil {
		t.Fatalf("InvokeCapability(spreadsheet.pivot.create): %v", err)
	}
	var output struct {
		Sheet           string   `json:"sheet"`
		PivotTableRange string   `json:"pivot_table_range"`
		Rows            []string `json:"rows"`
		Columns         []string `json:"columns"`
		Data            []string `json:"data"`
		Changed         bool     `json:"changed"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode pivot result: %v; data=%s", err, result.Data)
	}
	if output.Sheet != "Sheet1" || output.PivotTableRange != "F2:J15" || strings.Join(output.Rows, ",") != "部门" || strings.Join(output.Columns, ",") != "季度" || strings.Join(output.Data, ",") != "销售额" || !output.Changed {
		t.Fatalf("pivot result = %+v; data=%s", output, result.Data)
	}
	afterCreate := mustGetDocumentInfo(t, runtime, documentID)
	if result.Revision != beforeCreate.Revision+1 || afterCreate.Revision != result.Revision || !afterCreate.Dirty {
		t.Fatalf("pivot creation state: before=%+v result=%+v after=%+v", beforeCreate, result, afterCreate)
	}

	path := filepath.Join(t.TempDir(), "department-sales-pivot.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after pivot creation: %v", err)
	}
	if reopened.Dirty {
		t.Fatalf("reopened workbook is dirty: %+v", reopened)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open saved XLSX: %v", err)
	}
	defer archive.Close()
	var pivotTableXML, pivotCacheXML string
	for _, file := range archive.File {
		if !strings.HasPrefix(file.Name, "xl/pivotTables/") && !strings.HasPrefix(file.Name, "xl/pivotCache/") {
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
		if strings.HasPrefix(file.Name, "xl/pivotTables/") {
			pivotTableXML += string(data)
		} else {
			pivotCacheXML += string(data)
		}
	}
	if !strings.Contains(pivotTableXML, "pivotTableDefinition") || !strings.Contains(pivotCacheXML, "pivotCacheDefinition") {
		t.Fatalf("pivot table/cache parts were not saved: table=%s cache=%s", pivotTableXML, pivotCacheXML)
	}
}
