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

type testSpreadsheetchartSheetKindArguments struct {
	Sheet  string             `json:"sheet"`
	Kind   string             `json:"kind"`
	Bounds document.CellRange `json:"bounds"`
	Cell   string             `json:"cell"`
	Title  string             `json:"title"`
}

type testSpreadsheetchartSheetEditsArguments struct {
	Sheet string                       `json:"sheet"`
	Edits []spreadsheetCellEditFixture `json:"edits"`
}

func TestSpreadsheetChartCapabilityUsesExplicitRangeAndRoundTrips(t *testing.T) {
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
	invoke := func(arguments any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		info := mustGetDocumentInfo(t, runtime, documentID)
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("xlsx-chart-%d", requestNumber), DocumentID: documentID,
			CapabilityID: "spreadsheet.chart.create", ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "chart-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}

	seed := []spreadsheetCellEditFixture{
		{Row: 1, Column: 1, Value: "季度"}, {Row: 1, Column: 2, Value: "营收"},
		{Row: 2, Column: 1, Value: "第一季度"}, {Row: 2, Column: 2, Value: "120"},
		{Row: 3, Column: 1, Value: "第二季度"}, {Row: 3, Column: 2, Value: "150"},
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "xlsx-chart-seed", DocumentID: documentID, CapabilityID: "spreadsheet.cells.set",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "chart-agent-test", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, testSpreadsheetchartSheetEditsArguments{Sheet: "Sheet1", Edits: seed}),
	}); err != nil {
		t.Fatalf("seed chart source: %v", err)
	}

	invalidTargetRevision := mustGetDocumentInfo(t, runtime, documentID)
	_, err = invoke(testSpreadsheetchartSheetKindArguments{Sheet: "Missing", Kind: "col", Bounds: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 2}, Cell: "E2", Title: "季度营收"})
	if err == nil {
		t.Fatal("chart creation accepted a missing source worksheet")
	}
	if after := mustGetDocumentInfo(t, runtime, documentID); after.Revision != invalidTargetRevision.Revision || after.Dirty != invalidTargetRevision.Dirty {
		t.Fatalf("invalid worksheet partially changed workbook: before=%+v after=%+v", invalidTargetRevision, after)
	}

	before := mustGetDocumentInfo(t, runtime, documentID)
	result, err := invoke(testSpreadsheetchartSheetKindArguments{Sheet: "Sheet1", Kind: "col", Bounds: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 2}, Cell: "E2", Title: "季度营收"})
	if err != nil {
		t.Fatalf("InvokeCapability(spreadsheet.chart.create): %v", err)
	}
	var output struct {
		Sheet   string `json:"sheet"`
		Kind    string `json:"kind"`
		Cell    string `json:"cell"`
		Title   string `json:"title"`
		Changed bool   `json:"changed"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode chart result: %v; data=%s", err, result.Data)
	}
	if output.Sheet != "Sheet1" || output.Kind != "col" || output.Cell != "E2" || output.Title != "季度营收" || !output.Changed {
		t.Fatalf("chart result = %+v; data=%s", output, result.Data)
	}
	after := mustGetDocumentInfo(t, runtime, documentID)
	if result.Revision != before.Revision+1 || after.Revision != result.Revision || !after.Dirty {
		t.Fatalf("chart command state: before=%+v result=%+v after=%+v", before, result, after)
	}

	path := filepath.Join(t.TempDir(), "quarterly-revenue.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after chart creation: %v", err)
	}
	if reopened.Dirty {
		t.Fatalf("reopened workbook is dirty: %+v", reopened)
	}

	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open saved XLSX: %v", err)
	}
	defer archive.Close()
	var chartXML string
	for _, file := range archive.File {
		if !strings.HasPrefix(file.Name, "xl/charts/chart") || !strings.HasSuffix(file.Name, ".xml") {
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
		chartXML += string(data)
	}
	for _, want := range []string{"季度营收", "Sheet1!$A$2:$A$3", "Sheet1!$B$2:$B$3"} {
		if !strings.Contains(chartXML, want) {
			t.Fatalf("saved chart is missing %q; chart parts=%s", want, chartXML)
		}
	}
}
