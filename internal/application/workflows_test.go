package application

import (
	"context"
	"encoding/json"
	"testing"

	"glance/internal/document"
)

func TestDocumentCompareAndPlanPreviewAreReadOnly(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	left, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument left: %v", err)
	}
	right, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument right: %v", err)
	}
	changedRight, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "workflow-fixture-cell", DocumentID: right.DocumentID,
		CapabilityID: "spreadsheet.cell.set", ExpectedRevision: right.Revision,
		Operator:  Operator{ID: "workflow-fixture", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"sheet": "Sheet1", "cell": "A1", "value": "changed"}),
	})
	if err != nil {
		t.Fatalf("set comparison fixture: %v", err)
	}
	right.Revision = changedRight.Revision

	leftBefore, err := runtime.GetDocument(ctx, left.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument left before: %v", err)
	}
	rightBefore, err := runtime.GetDocument(ctx, right.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument right before: %v", err)
	}
	comparison, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "workflow-document-compare", CapabilityID: "document.compare",
		Operator: Operator{ID: "workflow-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{
			"left_document_id": left.DocumentID, "right_document_id": right.DocumentID,
			"left_expected_revision": leftBefore.Revision, "right_expected_revision": rightBefore.Revision,
			"sheet": "Sheet1", "range": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1},
		}),
	})
	if err != nil {
		t.Fatalf("Invoke document.compare: %v", err)
	}
	var compareResult struct {
		ChangedPaths []string `json:"changed_paths"`
	}
	if err := json.Unmarshal(comparison.Data, &compareResult); err != nil {
		t.Fatalf("decode document.compare: %v", err)
	}
	if len(compareResult.ChangedPaths) == 0 {
		t.Fatalf("document.compare missed fixture difference: %s", comparison.Data)
	}
	assertDocumentInfoUnchanged(t, runtime, leftBefore)
	assertDocumentInfoUnchanged(t, runtime, rightBefore)

	validated, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "workflow-plan-validate", DocumentID: left.DocumentID,
		CapabilityID: "document.plan.validate", ExpectedRevision: leftBefore.Revision,
		Operator: Operator{ID: "workflow-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"commands": []any{map[string]any{
			"capability_id": "spreadsheet.cell.set",
			"arguments":     map[string]any{"sheet": "Sheet1", "cell": "B1", "value": "would be written"},
		}}}),
	})
	if err != nil {
		t.Fatalf("Invoke document.plan.validate: %v", err)
	}
	var plan struct {
		PlanID string `json:"plan_id"`
		Valid  bool   `json:"valid"`
	}
	if err := json.Unmarshal(validated.Data, &plan); err != nil {
		t.Fatalf("decode validated plan: %v", err)
	}
	if !plan.Valid || plan.PlanID == "" {
		t.Fatalf("validated plan = %+v; want valid plan ID", plan)
	}
	assertDocumentInfoUnchanged(t, runtime, leftBefore)

	previewed, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "workflow-plan-preview", DocumentID: left.DocumentID,
		CapabilityID: "document.plan.preview", ExpectedRevision: leftBefore.Revision,
		Operator:  Operator{ID: "workflow-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]string{"plan_id": plan.PlanID}),
	})
	if err != nil {
		t.Fatalf("Invoke document.plan.preview: %v", err)
	}
	var preview struct {
		Valid    bool `json:"valid"`
		Commands []struct {
			CapabilityID string `json:"capability_id"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(previewed.Data, &preview); err != nil {
		t.Fatalf("decode plan preview: %v", err)
	}
	if !preview.Valid || len(preview.Commands) != 1 || preview.Commands[0].CapabilityID != "spreadsheet.cell.set" {
		t.Fatalf("plan preview = %+v; want valid one-command preview", preview)
	}
	assertDocumentInfoUnchanged(t, runtime, leftBefore)
}

func TestDocumentPlanApplyRequiresHumanAndReportsPartialFailure(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	agent := Operator{ID: "workflow-agent", Kind: OperatorExternalAgent}
	human := Operator{ID: "desktop-user", Kind: OperatorHuman}

	applyPlan := func(doc DocumentInfo, suffix string, commands []any) struct {
		StartRevision uint64 `json:"start_revision"`
		EndRevision   uint64 `json:"end_revision"`
		Status        string `json:"status"`
		Commands      []struct {
			CapabilityID string `json:"capability_id"`
			Status       string `json:"status"`
			Revision     uint64 `json:"revision"`
		} `json:"commands"`
	} {
		t.Helper()
		validated, validateErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "validate-plan-" + suffix, DocumentID: doc.DocumentID, CapabilityID: "document.plan.validate",
			ExpectedRevision: doc.Revision, Operator: agent,
			Arguments: mustJSON(t, map[string]any{"commands": commands}),
		})
		if validateErr != nil {
			t.Fatalf("validate plan %s: %v", suffix, validateErr)
		}
		var plan struct {
			PlanID string `json:"plan_id"`
			Valid  bool   `json:"valid"`
		}
		if err := json.Unmarshal(validated.Data, &plan); err != nil || !plan.Valid || plan.PlanID == "" {
			t.Fatalf("validated plan %s=%s error=%v", suffix, validated.Data, err)
		}
		pending, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "apply-plan-" + suffix, DocumentID: doc.DocumentID, CapabilityID: "document.plan.apply",
			ExpectedRevision: doc.Revision, Operator: agent,
			Arguments: mustJSON(t, map[string]string{"plan_id": plan.PlanID}),
		})
		if invokeErr != nil {
			t.Fatalf("stage plan %s: %v", suffix, invokeErr)
		}
		var staged struct {
			Status         string `json:"status"`
			ConfirmationID string `json:"confirmation_id"`
		}
		if err := json.Unmarshal(pending.Data, &staged); err != nil || staged.Status != "confirmation_required" || staged.ConfirmationID == "" {
			t.Fatalf("staged plan %s=%s error=%v", suffix, pending.Data, err)
		}
		if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, agent); err == nil {
			t.Fatalf("Agent approved its own plan %s", suffix)
		}
		if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, human); err != nil {
			t.Fatalf("human approval for plan %s: %v", suffix, err)
		}
		confirmation := waitForConfirmationStatus(t, runtime, staged.ConfirmationID, ConfirmationCompleted)
		if confirmation.Result == nil {
			t.Fatalf("completed plan %s has no result: %+v", suffix, confirmation)
		}
		var applied struct {
			StartRevision uint64 `json:"start_revision"`
			EndRevision   uint64 `json:"end_revision"`
			Status        string `json:"status"`
			Commands      []struct {
				CapabilityID string `json:"capability_id"`
				Status       string `json:"status"`
				Revision     uint64 `json:"revision"`
			} `json:"commands"`
		}
		if err := json.Unmarshal(confirmation.Result.Data, &applied); err != nil {
			t.Fatalf("decode applied plan %s: %v data=%s", suffix, err, confirmation.Result.Data)
		}
		return applied
	}

	t.Run("all commands complete in revision order", func(t *testing.T) {
		doc, err := runtime.CreateDocument(ctx, FormatXLSX)
		if err != nil {
			t.Fatalf("CreateDocument: %v", err)
		}
		result := applyPlan(doc, "success", []any{
			map[string]any{"capability_id": "spreadsheet.cell.set", "arguments": map[string]any{"sheet": "Sheet1", "cell": "A1", "value": "已核对"}},
			map[string]any{"capability_id": "spreadsheet.cell.set", "arguments": map[string]any{"sheet": "Sheet1", "cell": "B1", "value": "完成"}},
		})
		if result.Status != "applied" || result.StartRevision != doc.Revision || result.EndRevision != doc.Revision+2 || len(result.Commands) != 2 || result.Commands[0].Status != "completed" || result.Commands[1].Status != "completed" || result.Commands[0].Revision != doc.Revision+1 || result.Commands[1].Revision != doc.Revision+2 {
			t.Fatalf("applied plan result=%+v", result)
		}
		info := mustGetDocumentInfo(t, runtime, doc.DocumentID)
		if info.Revision != result.EndRevision || !info.Dirty {
			t.Fatalf("applied plan document state=%+v result=%+v", info, result)
		}
		snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{
			DocumentID: doc.DocumentID, Sheet: "Sheet1",
			Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2},
		})
		if err != nil {
			t.Fatalf("inspect applied plan cells: %v", err)
		}
		var values struct {
			Rows [][]struct {
				Value string `json:"value"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(snapshot.Content, &values); err != nil || len(values.Rows) != 1 || len(values.Rows[0]) != 2 || values.Rows[0][0].Value != "已核对" || values.Rows[0][1].Value != "完成" {
			t.Fatalf("applied plan cell values=%s error=%v", snapshot.Content, err)
		}
	})

	t.Run("stop on failure and report prior commits", func(t *testing.T) {
		doc, err := runtime.CreateDocument(ctx, FormatXLSX)
		if err != nil {
			t.Fatalf("CreateDocument: %v", err)
		}
		result := applyPlan(doc, "partial", []any{
			map[string]any{"capability_id": "spreadsheet.cell.set", "arguments": map[string]any{"sheet": "Sheet1", "cell": "A1", "value": "先完成"}},
			map[string]any{"capability_id": "spreadsheet.cell.set", "arguments": map[string]any{"sheet": "MissingSheet", "cell": "B1", "value": "应失败"}},
			map[string]any{"capability_id": "spreadsheet.cell.set", "arguments": map[string]any{"sheet": "Sheet1", "cell": "C1", "value": "不得执行"}},
		})
		if result.Status != "partial_failed" || result.StartRevision != doc.Revision || result.EndRevision != doc.Revision+1 || len(result.Commands) != 3 || result.Commands[0].Status != "completed" || result.Commands[1].Status != "failed" || result.Commands[2].Status != "not_run" {
			t.Fatalf("partial plan result=%+v", result)
		}
		info := mustGetDocumentInfo(t, runtime, doc.DocumentID)
		if info.Revision != result.EndRevision || !info.Dirty {
			t.Fatalf("partial plan document state=%+v result=%+v", info, result)
		}
		snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{
			DocumentID: doc.DocumentID, Sheet: "Sheet1",
			Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 3},
		})
		if err != nil {
			t.Fatalf("inspect partial plan cells: %v", err)
		}
		var values struct {
			Rows [][]struct {
				Value string `json:"value"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(snapshot.Content, &values); err != nil || len(values.Rows) != 1 || len(values.Rows[0]) != 3 || values.Rows[0][0].Value != "先完成" || values.Rows[0][2].Value != "" {
			t.Fatalf("partial plan cell values=%s error=%v", snapshot.Content, err)
		}
	})
}

func assertDocumentInfoUnchanged(t *testing.T, runtime *Runtime, before DocumentInfo) {
	t.Helper()
	after, err := runtime.GetDocument(context.Background(), before.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument after read-only workflow: %v", err)
	}
	if after.Revision != before.Revision || after.Dirty != before.Dirty {
		t.Fatalf("read-only workflow changed document state: before=%+v after=%+v", before, after)
	}
}
