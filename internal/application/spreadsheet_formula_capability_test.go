package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"glance/internal/document"
)

type testSpreadsheetformulaSheetCellArguments struct {
	Sheet   string `json:"sheet"`
	Cell    string `json:"cell"`
	Formula string `json:"formula"`
}

func TestSpreadsheetFormulaCapabilityUsesExplicitCellAndRoundTrips(t *testing.T) {
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
	invoke := func(sheet, cell, formula string) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		info, getErr := runtime.GetDocument(ctx, documentID)
		if getErr != nil {
			t.Fatalf("GetDocument before formula request: %v", getErr)
		}
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("xlsx-formula-%d", requestNumber), DocumentID: documentID,
			CapabilityID: "spreadsheet.formula.set", ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "formula-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, testSpreadsheetformulaSheetCellArguments{Sheet: sheet, Cell: cell, Formula: formula}),
		})
	}
	inspectFormula := func() string {
		t.Helper()
		range1 := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}
		snapshot, inspectErr := runtime.Queries().InspectDocument(ctx, InspectRequest{
			DocumentID: documentID, Sheet: "Sheet1", Range: &range1,
		})
		if inspectErr != nil {
			t.Fatalf("InspectDocument(A1): %v", inspectErr)
		}
		var output struct {
			Rows [][]struct {
				Formula string `json:"formula"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(snapshot.Content, &output); err != nil {
			t.Fatalf("decode A1 query: %v; content=%s", err, snapshot.Content)
		}
		if len(output.Rows) != 1 || len(output.Rows[0]) != 1 {
			t.Fatalf("A1 query returned unexpected range: %s", snapshot.Content)
		}
		return output.Rows[0][0].Formula
	}
	assertFailureAtomic := func(sheet, cell, formula string) {
		t.Helper()
		before, beforeFormula := mustGetDocumentInfo(t, runtime, documentID), inspectFormula()
		if _, invokeErr := invoke(sheet, cell, formula); invokeErr == nil {
			t.Fatalf("formula request unexpectedly succeeded: sheet=%q cell=%q formula rune count=%d", sheet, cell, len([]rune(formula)))
		}
		after := mustGetDocumentInfo(t, runtime, documentID)
		if after.Revision != before.Revision || after.Dirty != before.Dirty || inspectFormula() != beforeFormula {
			t.Fatalf("failed formula request partially changed workbook: before=%+v after=%+v before formula=%q after formula=%q", before, after, beforeFormula, inspectFormula())
		}
	}
	assertFormula := func(formula string, wantChanged bool) CapabilityInvocationResult {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, documentID)
		result, invokeErr := invoke("Sheet1", "A1", formula)
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(spreadsheet.formula.set): %v", invokeErr)
		}
		var output struct {
			Changed bool   `json:"changed"`
			Sheet   string `json:"sheet"`
			Cell    string `json:"cell"`
			Formula string `json:"formula"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode formula result: %v; data=%s", err, result.Data)
		}
		if output.Changed != wantChanged || output.Sheet != "Sheet1" || output.Cell != "A1" || output.Formula != formula {
			t.Fatalf("formula result=%+v, want changed=%v formula=%q", output, wantChanged, formula)
		}
		after := mustGetDocumentInfo(t, runtime, documentID)
		wantRevision := before.Revision
		if wantChanged {
			wantRevision++
		}
		if after.Revision != wantRevision || after.Dirty != (before.Dirty || wantChanged) {
			t.Fatalf("formula state: before=%+v after=%+v changed=%v", before, after, wantChanged)
		}
		return result
	}

	if !doc.Dirty {
		t.Fatalf("new workbook should be dirty before its first save: %+v", doc)
	}
	assertFailureAtomic("Missing", "A1", "=SUM(1,2)")
	assertFailureAtomic("Sheet1", "A0", "=SUM(1,2)")
	assertFailureAtomic("Sheet1", "A1", "")
	set := assertFormula("=SUM(1,2)", true)
	if formula := inspectFormula(); formula != "SUM(1,2)" {
		t.Fatalf("A1 formula=%q, want SUM(1,2)", formula)
	}
	assertFormula("SUM(1,2)", false) // A leading equals sign is normalized for identity.
	boundaryFormula := `="` + strings.Repeat("薪", 8189) + `"`
	assertFormula(boundaryFormula, true) // 8192 UTF-16 units, including the formula marker and quotes.
	if formula := inspectFormula(); formula != strings.TrimPrefix(boundaryFormula, "=") {
		t.Fatalf("A1 boundary formula has %d runes; value was not preserved", len([]rune(formula)))
	}
	assertFailureAtomic("Sheet1", "A1", `="`+strings.Repeat("薪", 8190)+`"`) // 8193 BMP units.
	assertFailureAtomic("Sheet1", "A1", `="`+strings.Repeat("😀", 4095)+`"`) // 8193 UTF-16 units in fewer than 8192 runes.
	if _, err := runtime.SaveDocumentAs(ctx, documentID, t.TempDir()+"/formula.xlsx"); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if info := mustGetDocumentInfo(t, runtime, documentID); info.Dirty {
		t.Fatalf("saved workbook remains dirty: %+v", info)
	}
	path := mustGetDocumentInfo(t, runtime, documentID).SourcePath
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after formula save: %v", err)
	}
	documentID = reopened.DocumentID
	if reopened.Dirty || inspectFormula() != strings.TrimPrefix(boundaryFormula, "=") {
		t.Fatalf("reopened formula state=%+v formula=%q (initial result=%s)", reopened, inspectFormula(), set.Data)
	}
	assertFormula(boundaryFormula, false) // A no-op on the reopened workbook must remain clean.
}

func mustGetDocumentInfo(t *testing.T, runtime *Runtime, documentID string) DocumentInfo {
	t.Helper()
	info, err := runtime.GetDocument(context.Background(), documentID)
	if err != nil {
		t.Fatalf("GetDocument(%s): %v", documentID, err)
	}
	return info
}
