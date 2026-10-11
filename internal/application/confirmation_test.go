package application

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestHighRiskCapabilityWaitsForOneTimeHumanApproval(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	var executed atomic.Bool
	registerHighRiskTestCommand(t, runtime, &executed)
	request := CapabilityRequest{
		RequestID: "high-risk-request", DocumentID: doc.DocumentID, CapabilityID: "test.high_risk.command",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: json.RawMessage(`{"target_id":"image-7"}`),
	}
	pending, err := runtime.InvokeCapability(ctx, request)
	if err != nil {
		t.Fatalf("InvokeCapability should stage confirmation: %v", err)
	}
	var staged struct {
		Status           string `json:"status"`
		ConfirmationID   string `json:"confirmation_id"`
		CapabilityID     string `json:"capability_id"`
		DocumentID       string `json:"document_id"`
		ExpectedRevision uint64 `json:"expected_revision"`
		ImpactSummary    string `json:"impact_summary"`
	}
	if err := json.Unmarshal(pending.Data, &staged); err != nil {
		t.Fatalf("decode staged confirmation: %v", err)
	}
	if staged.Status != "confirmation_required" || staged.ConfirmationID == "" || staged.CapabilityID != request.CapabilityID || staged.DocumentID != doc.DocumentID || staged.ExpectedRevision != doc.Revision || staged.ImpactSummary == "" {
		t.Fatalf("staged confirmation summary = %+v", staged)
	}
	if executed.Load() {
		t.Fatal("high-risk operation executed before approval")
	}
	if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, Operator{ID: "agent", Kind: OperatorExternalAgent}); !errors.Is(err, ErrHumanApprovalRequired) {
		t.Fatalf("Agent approval error = %v; want ErrHumanApprovalRequired", err)
	}
	if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, Operator{ID: "operator-1", Kind: OperatorHuman}); err != nil {
		t.Fatalf("human approval: %v", err)
	}
	info := waitForConfirmationStatus(t, runtime, staged.ConfirmationID, ConfirmationCompleted)
	if !executed.Load() || info.Result == nil || info.Result.Revision != doc.Revision+1 {
		t.Fatalf("approved command result = %+v, executed=%v", info, executed.Load())
	}
	if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, Operator{ID: "operator-1", Kind: OperatorHuman}); !errors.Is(err, ErrConfirmationNotPending) {
		t.Fatalf("repeated approval error = %v; want ErrConfirmationNotPending", err)
	}
}

func TestHighRiskCapabilityRejectsAgentConfirmationAndHumanCanReject(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	var executed atomic.Bool
	registerHighRiskTestCommand(t, runtime, &executed)
	request := CapabilityRequest{
		RequestID: "high-risk-rejected", DocumentID: doc.DocumentID, CapabilityID: "test.high_risk.command",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: json.RawMessage(`{"target_id":"image-7","confirmed":true}`),
	}
	if _, err := runtime.InvokeCapability(ctx, request); !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("Agent confirmed=true error = %v; want invalid arguments", err)
	}
	request.Arguments = json.RawMessage(`{"target_id":"image-7"}`)
	pending, err := runtime.InvokeCapability(ctx, request)
	if err != nil {
		t.Fatalf("InvokeCapability: %v", err)
	}
	var staged struct {
		ConfirmationID string `json:"confirmation_id"`
	}
	if err := json.Unmarshal(pending.Data, &staged); err != nil || staged.ConfirmationID == "" {
		t.Fatalf("confirmation response = %s, decode error %v", pending.Data, err)
	}
	if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, false, Operator{ID: "operator-1", Kind: OperatorHuman}); err != nil {
		t.Fatalf("human rejection: %v", err)
	}
	info := waitForConfirmationStatus(t, runtime, staged.ConfirmationID, ConfirmationRejected)
	if executed.Load() || info.ResolvedBy != "operator-1" {
		t.Fatalf("rejected operation state = %+v, executed=%v", info, executed.Load())
	}
}

func TestEveryHighRiskCommandRequiresHumanConfirmation(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	documents := make(map[Format]DocumentInfo)
	checked := 0
	for _, descriptor := range runtime.Catalog().Descriptors() {
		if descriptor.Category != CapabilityCommand || !highRisk(descriptor.Risk) || descriptor.RequiresView {
			continue
		}
		format := FormatXLSX
		if len(descriptor.Formats) > 0 {
			format = descriptor.Formats[0]
		}
		if format == FormatXLS {
			t.Errorf("high-risk command %s only targets read-only XLS", descriptor.ID)
			continue
		}
		doc, exists := documents[format]
		if !exists {
			doc, err = runtime.CreateDocument(ctx, format)
			if err != nil {
				t.Fatalf("CreateDocument(%s) for %s: %v", format, descriptor.ID, err)
			}
			documents[format] = doc
		}
		arguments, err := exampleArguments(descriptor.InputSchema)
		if err != nil {
			t.Errorf("%s: build schema-valid arguments: %v", descriptor.ID, err)
			continue
		}
		arguments = isolateGeneratedPaths(t, arguments, descriptor.ID)
		if err := validateArguments(descriptor.InputSchema, arguments); err != nil {
			t.Errorf("%s: generated arguments do not satisfy schema: %v", descriptor.ID, err)
			continue
		}
		before, err := runtime.GetDocument(ctx, doc.DocumentID)
		if err != nil {
			t.Fatalf("GetDocument before %s: %v", descriptor.ID, err)
		}
		pending, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "confirm-high-risk-" + descriptor.ID, DocumentID: doc.DocumentID,
			CapabilityID: descriptor.ID, ExpectedRevision: before.Revision,
			Operator: Operator{ID: "high-risk-agent", Kind: OperatorExternalAgent}, Arguments: arguments,
		})
		if err != nil {
			t.Errorf("%s did not stage confirmation: %v", descriptor.ID, err)
			continue
		}
		var staged struct {
			Status           string `json:"status"`
			ConfirmationID   string `json:"confirmation_id"`
			CapabilityID     string `json:"capability_id"`
			ExpectedRevision uint64 `json:"expected_revision"`
		}
		if err := json.Unmarshal(pending.Data, &staged); err != nil || staged.Status != "confirmation_required" || staged.ConfirmationID == "" || staged.CapabilityID != descriptor.ID || staged.ExpectedRevision != before.Revision {
			t.Errorf("%s confirmation response = %+v, data=%s, error=%v", descriptor.ID, staged, pending.Data, err)
			continue
		}
		afterStage, err := runtime.GetDocument(ctx, doc.DocumentID)
		if err != nil || afterStage.Revision != before.Revision || afterStage.Dirty != before.Dirty {
			t.Errorf("%s changed document before human confirmation: before=%+v after=%+v error=%v", descriptor.ID, before, afterStage, err)
		}
		if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, false, Operator{ID: "high-risk-reviewer", Kind: OperatorHuman}); err != nil {
			t.Errorf("reject %s confirmation: %v", descriptor.ID, err)
			continue
		}
		if rejected := waitForConfirmationStatus(t, runtime, staged.ConfirmationID, ConfirmationRejected); rejected.Result != nil {
			t.Errorf("rejected %s confirmation returned a result: %+v", descriptor.ID, rejected)
		}
		afterReject, err := runtime.GetDocument(ctx, doc.DocumentID)
		if err != nil || afterReject.Revision != before.Revision || afterReject.Dirty != before.Dirty {
			t.Errorf("rejected %s confirmation changed document: before=%+v after=%+v error=%v", descriptor.ID, before, afterReject, err)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no high-risk command confirmation paths were checked")
	}
	t.Logf("verified human confirmation and rejection for %d high-risk commands", checked)
}

func registerHighRiskTestCommand(t *testing.T, runtime *Runtime, executed *atomic.Bool) {
	t.Helper()
	err := runtime.Catalog().Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "test.high_risk.command", Name: "删除 Word 图片",
			Description: "删除指定图片；此操作会改变文档内容。",
			Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskHigh,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"target_id":{"type":"string"}},"required":["target_id"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"}},"required":["changed"],"additionalProperties":false}`),
		},
		Handler: func(context.Context, CapabilityContext, json.RawMessage) (CapabilityResult, error) {
			executed.Store(true)
			return CapabilityResult{Data: json.RawMessage(`{"changed":true}`), Changed: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("register high-risk test command: %v", err)
	}
}

func waitForConfirmationStatus(t *testing.T, runtime *Runtime, id string, status ConfirmationStatus) ConfirmationInfo {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		info, err := runtime.GetConfirmation(context.Background(), id)
		if err != nil {
			t.Fatalf("GetConfirmation: %v", err)
		}
		if info.Status == status {
			return info
		}
		time.Sleep(time.Millisecond)
	}
	info, err := runtime.GetConfirmation(context.Background(), id)
	if err != nil {
		t.Fatalf("GetConfirmation after deadline: %v", err)
	}
	t.Fatalf("confirmation status = %q; want %q", info.Status, status)
	return ConfirmationInfo{}
}
