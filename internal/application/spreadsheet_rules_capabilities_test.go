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

func TestSpreadsheetConditionalFormatAndValidationCapabilitiesRoundTrip(t *testing.T) {
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
			RequestID: fmt.Sprintf("xlsx-rules-%d", requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "rules-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}
	assertRuleCommand := func(capabilityID string, arguments any, wantChanged bool) {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, documentID)
		result, invokeErr := invoke(capabilityID, arguments)
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(%s): %v", capabilityID, invokeErr)
		}
		var output struct {
			Changed bool `json:"changed"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode %s result: %v; data=%s", capabilityID, err, result.Data)
		}
		if output.Changed != wantChanged {
			t.Fatalf("%s changed=%v, want %v; data=%s", capabilityID, output.Changed, wantChanged, result.Data)
		}
		after := mustGetDocumentInfo(t, runtime, documentID)
		wantRevision := before.Revision
		if wantChanged {
			wantRevision++
		}
		if result.Revision != wantRevision || after.Revision != wantRevision || after.Dirty != (before.Dirty || wantChanged) {
			t.Fatalf("%s state: before=%+v result=%+v after=%+v", capabilityID, before, result, after)
		}
	}
	assertInvalidTarget := func(capabilityID string, arguments any) {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, documentID)
		_, invokeErr := invoke(capabilityID, arguments)
		if invokeErr == nil {
			t.Fatalf("%s accepted a missing worksheet", capabilityID)
		}
		after := mustGetDocumentInfo(t, runtime, documentID)
		if after.Revision != before.Revision || after.Dirty != before.Dirty {
			t.Fatalf("%s partially changed workbook for invalid target: before=%+v after=%+v", capabilityID, before, after)
		}
	}
	readSheetXML := func(path string) string {
		t.Helper()
		archive, err := zip.OpenReader(path)
		if err != nil {
			t.Fatalf("open XLSX %s: %v", path, err)
		}
		defer archive.Close()
		for _, file := range archive.File {
			if file.Name != "xl/worksheets/sheet1.xml" {
				continue
			}
			reader, err := file.Open()
			if err != nil {
				t.Fatalf("open worksheet XML: %v", err)
			}
			data, readErr := io.ReadAll(reader)
			closeErr := reader.Close()
			if readErr != nil || closeErr != nil {
				t.Fatalf("read worksheet XML: read=%v close=%v", readErr, closeErr)
			}
			return string(data)
		}
		t.Fatal("saved XLSX has no first worksheet XML")
		return ""
	}

	conditionalRules := []struct {
		bounds document.CellRange
		typeID string
	}{
		{document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 1}, "data_bar"},
		{document.CellRange{StartRow: 1, StartColumn: 3, EndRow: 3, EndColumn: 3}, "3_color_scale"},
		{document.CellRange{StartRow: 1, StartColumn: 4, EndRow: 3, EndColumn: 4}, "duplicate"},
		{document.CellRange{StartRow: 1, StartColumn: 5, EndRow: 3, EndColumn: 5}, "above_average"},
	}
	validations := []map[string]any{
		{
			"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 1, StartColumn: 2, EndRow: 3, EndColumn: 2},
			"operation": "list", "items": []string{"北区", "南区"},
		},
		{
			"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 1, StartColumn: 6, EndRow: 3, EndColumn: 6},
			"operation": "number", "minimum": 0, "maximum": 100,
		},
	}
	invalidConditional := map[string]any{"sheet": "Missing", "bounds": conditionalRules[0].bounds, "operation": "set", "type": conditionalRules[0].typeID}
	invalidValidation := map[string]any{"sheet": "Missing", "bounds": validations[0]["bounds"], "operation": "list", "items": []string{"北区", "南区"}}
	assertInvalidTarget("spreadsheet.conditional_format.set", invalidConditional)
	assertInvalidTarget("spreadsheet.data_validation.set", invalidValidation)
	for _, rule := range conditionalRules {
		arguments := map[string]any{"sheet": "Sheet1", "bounds": rule.bounds, "operation": "set", "type": rule.typeID}
		assertRuleCommand("spreadsheet.conditional_format.set", arguments, true)
		assertRuleCommand("spreadsheet.conditional_format.set", arguments, false)
	}
	for _, validation := range validations {
		assertRuleCommand("spreadsheet.data_validation.set", validation, true)
		assertRuleCommand("spreadsheet.data_validation.set", validation, false)
	}

	path := filepath.Join(t.TempDir(), "rules.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs with rules: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after rules: %v", err)
	}
	documentID = reopened.DocumentID
	if reopened.Dirty {
		t.Fatalf("reopened workbook with rules is dirty: %+v", reopened)
	}
	for _, rule := range conditionalRules {
		arguments := map[string]any{"sheet": "Sheet1", "bounds": rule.bounds, "operation": "set", "type": rule.typeID}
		assertRuleCommand("spreadsheet.conditional_format.set", arguments, false)
	}
	for _, validation := range validations {
		assertRuleCommand("spreadsheet.data_validation.set", validation, false)
	}
	sheetXML := readSheetXML(path)
	for _, ref := range []string{`sqref="A1:A3"`, `sqref="C1:C3"`, `sqref="D1:D3"`, `sqref="E1:E3"`} {
		if !strings.Contains(sheetXML, "<conditionalFormatting") || !strings.Contains(sheetXML, ref) {
			t.Fatalf("conditional format did not round-trip on %s: %s", ref, sheetXML)
		}
	}
	for _, ref := range []string{`sqref="B1:B3"`, `sqref="F1:F3"`} {
		if !strings.Contains(sheetXML, "<dataValidations") || !strings.Contains(sheetXML, ref) {
			t.Fatalf("data validation did not round-trip on %s: %s", ref, sheetXML)
		}
	}

	for _, rule := range conditionalRules {
		arguments := map[string]any{"sheet": "Sheet1", "bounds": rule.bounds, "operation": "clear"}
		assertRuleCommand("spreadsheet.conditional_format.set", arguments, true)
		assertRuleCommand("spreadsheet.conditional_format.set", arguments, false)
	}
	for _, validation := range validations {
		arguments := map[string]any{"sheet": "Sheet1", "bounds": validation["bounds"], "operation": "clear"}
		assertRuleCommand("spreadsheet.data_validation.set", arguments, true)
		assertRuleCommand("spreadsheet.data_validation.set", arguments, false)
	}
	if _, err := runtime.SaveDocument(ctx, documentID); err != nil {
		t.Fatalf("SaveDocument after clear: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after clear: %v", err)
	}
	cleared, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after clear: %v", err)
	}
	if cleared.Dirty {
		t.Fatalf("reopened cleared workbook is dirty: %+v", cleared)
	}
	sheetXML = readSheetXML(path)
	if strings.Contains(sheetXML, "<conditionalFormatting") || strings.Contains(sheetXML, "<dataValidations") {
		t.Fatalf("cleared rules remain in saved worksheet XML: %s", sheetXML)
	}
}
