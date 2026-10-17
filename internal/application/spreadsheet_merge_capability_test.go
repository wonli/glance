package application

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"glance/internal/document"
)

func TestSpreadsheetMergeCapabilityConfirmsDataLossAndTogglesUnmerge(t *testing.T) {
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
	operator := Operator{ID: "spreadsheet-merge-test", Kind: OperatorHuman}
	requestNumber := 0
	invoke := func(capabilityID string, arguments any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		info, err := runtime.GetDocument(ctx, doc.DocumentID)
		if err != nil {
			t.Fatalf("GetDocument: %v", err)
		}
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("merge-%d", requestNumber), DocumentID: doc.DocumentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision, Operator: operator,
			Arguments: mustJSON(t, arguments),
		})
	}
	setCell := func(cell, value string) {
		t.Helper()
		if _, err := invoke("spreadsheet.cell.set", spreadsheetCellSetPayload{Sheet: "Sheet1", Cell: cell, Value: value}); err != nil {
			t.Fatalf("set %s: %v", cell, err)
		}
	}
	type mergeArguments struct {
		Sheet               string             `json:"sheet"`
		Bounds              document.CellRange `json:"bounds"`
		ConfirmDiscardToken string             `json:"confirm_discard_token,omitempty"`
	}
	type mergeResult struct {
		Changed              bool   `json:"changed"`
		ConfirmationRequired bool   `json:"confirmation_required"`
		DiscardedCellCount   int    `json:"discarded_cell_count"`
		ConfirmDiscardToken  string `json:"confirm_discard_token"`
		Unmerged             bool   `json:"unmerged"`
	}
	decode := func(result CapabilityInvocationResult) mergeResult {
		t.Helper()
		var output mergeResult
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode merge result: %v; data=%s", err, result.Data)
		}
		return output
	}
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2}
	setCell("A1", "标题")
	setCell("B1", "保留候选")

	preview, err := invoke("spreadsheet.cells.merge", mergeArguments{Sheet: "Sheet1", Bounds: bounds})
	if err != nil {
		t.Fatalf("preview merge: %v", err)
	}
	previewData := decode(preview)
	if !previewData.ConfirmationRequired || previewData.Changed || previewData.DiscardedCellCount != 1 || previewData.ConfirmDiscardToken == "" {
		t.Fatalf("merge preview = %+v; want a non-mutating confirmation for B1", previewData)
	}
	if afterPreview := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterPreview.Revision != preview.Revision {
		t.Fatalf("preview changed workbook revision to %d", afterPreview.Revision)
	}

	setCell("B1", "新内容")
	stale, err := invoke("spreadsheet.cells.merge", mergeArguments{Sheet: "Sheet1", Bounds: bounds, ConfirmDiscardToken: previewData.ConfirmDiscardToken})
	if err != nil {
		t.Fatalf("stale-token merge: %v", err)
	}
	staleData := decode(stale)
	if !staleData.ConfirmationRequired || staleData.Changed || staleData.ConfirmDiscardToken == previewData.ConfirmDiscardToken {
		t.Fatalf("stale confirmation = %+v; want refreshed confirmation token", staleData)
	}

	merged, err := invoke("spreadsheet.cells.merge", mergeArguments{Sheet: "Sheet1", Bounds: bounds, ConfirmDiscardToken: staleData.ConfirmDiscardToken})
	if err != nil {
		t.Fatalf("confirmed merge: %v", err)
	}
	if output := decode(merged); !output.Changed || output.ConfirmationRequired || output.Unmerged {
		t.Fatalf("confirmed merge = %+v", output)
	}

	undo, err := invoke("spreadsheet.history.apply", map[string]string{"action": "undo"})
	if err != nil {
		t.Fatalf("undo merge: %v", err)
	}
	var undoData struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(undo.Data, &undoData); err != nil || !undoData.Changed {
		t.Fatalf("undo merge result = %+v err=%v data=%s", undoData, err, undo.Data)
	}
	query, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Sheet: "Sheet1", Range: &bounds})
	if err != nil {
		t.Fatalf("InspectDocument after undo: %v", err)
	}
	var sheet struct {
		Rows [][]struct {
			Value string `json:"value"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(query.Content, &sheet); err != nil || len(sheet.Rows) != 1 || len(sheet.Rows[0]) != 2 || sheet.Rows[0][0].Value != "标题" || sheet.Rows[0][1].Value != "新内容" {
		t.Fatalf("undo failed to restore the pre-merge cells: %+v err=%v content=%s", sheet, err, query.Content)
	}

	setCell("B1", "")
	cleanMerge, err := invoke("spreadsheet.cells.merge", mergeArguments{Sheet: "Sheet1", Bounds: bounds})
	if err != nil {
		t.Fatalf("merge empty secondary cell: %v", err)
	}
	if output := decode(cleanMerge); !output.Changed || output.ConfirmationRequired {
		t.Fatalf("merge without data loss = %+v; want immediate merge", output)
	}
	unmerge, err := invoke("spreadsheet.cells.merge", mergeArguments{Sheet: "Sheet1", Bounds: bounds})
	if err != nil {
		t.Fatalf("toggle unmerge: %v", err)
	}
	if output := decode(unmerge); !output.Changed || !output.Unmerged {
		t.Fatalf("merge command on an existing merged range = %+v; want unmerge", output)
	}
}
