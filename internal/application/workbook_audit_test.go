package application

import (
	"context"
	"encoding/json"
	"testing"

	"glance/internal/document"
)

func TestWorkbookAuditFindsHeaderKeyAndErrorLikeValueIssuesWithoutMutation(t *testing.T) {
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

	values := []struct {
		cell  string
		value string
	}{
		{"A1", "员工"}, {"B1", "工资"}, {"C1", "员工"},
		{"A2", "张三"}, {"B2", "1000"}, {"C2", "张三"},
		{"A3", "张三"}, {"B3", "#VALUE!"}, {"C3", "张三"},
		{"A4", "李四"}, {"B4", "2000"}, {"C4", "李四"},
	}
	for index, value := range values {
		result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "audit-fixture-" + value.cell, DocumentID: doc.DocumentID,
			CapabilityID: "spreadsheet.cell.set", ExpectedRevision: doc.Revision + uint64(index),
			Operator:  Operator{ID: "audit-fixture", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, map[string]any{"sheet": "Sheet1", "cell": value.cell, "value": value.value}),
		})
		if err != nil {
			t.Fatalf("set audit fixture cell %s: %v", value.cell, err)
		}
		if result.Revision != doc.Revision+uint64(index)+1 {
			t.Fatalf("fixture revision after %s = %d", value.cell, result.Revision)
		}
	}
	before, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before audit: %v", err)
	}
	audited, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "workbook-audit", DocumentID: doc.DocumentID, CapabilityID: "workbook.audit",
		ExpectedRevision: before.Revision,
		Operator:         Operator{ID: "payroll-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{
			"sheet": "Sheet1", "range": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 4, EndColumn: 3},
			"header_row": 1, "required_headers": []string{"员工", "工资", "部门"}, "key_columns": []int{1},
		}),
	})
	if err != nil {
		t.Fatalf("Invoke workbook.audit: %v", err)
	}
	var result struct {
		CellsScanned    int  `json:"cells_scanned"`
		NonEmptyCells   int  `json:"non_empty_cells"`
		FormulaCells    int  `json:"formula_cells"`
		HasMoreFindings bool `json:"has_more_findings"`
		Findings        []struct {
			Code  string   `json:"code"`
			Cells []string `json:"cells"`
			Value string   `json:"value"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(audited.Data, &result); err != nil {
		t.Fatalf("decode workbook audit: %v", err)
	}
	if result.CellsScanned != 12 || result.NonEmptyCells != 12 || result.HasMoreFindings {
		t.Fatalf("audit summary = %+v", result)
	}
	wantCodes := map[string]bool{"duplicate_header": false, "missing_required_header": false, "duplicate_key": false, "error_like_value": false}
	for _, finding := range result.Findings {
		if _, ok := wantCodes[finding.Code]; ok {
			wantCodes[finding.Code] = true
		}
	}
	for code, found := range wantCodes {
		if !found {
			t.Errorf("audit findings omitted %q: %+v", code, result.Findings)
		}
	}
	after, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument after audit: %v", err)
	}
	if after.Revision != before.Revision || after.Dirty != before.Dirty {
		t.Fatalf("audit changed workbook state: before=%+v after=%+v", before, after)
	}
}

func TestWorkbookAuditRejectsOversizedRanges(t *testing.T) {
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
	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "oversized-workbook-audit", DocumentID: doc.DocumentID,
		CapabilityID: "workbook.audit", ExpectedRevision: doc.Revision,
		Operator: Operator{ID: "audit-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{
			"sheet": "Sheet1", "range": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1000, EndColumn: 1000},
		}),
	})
	if err == nil || err != ErrInvalidCommandArguments {
		t.Fatalf("oversized workbook audit error = %v; want invalid arguments", err)
	}
}
