package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestPresentationTransitionCapabilitiesApplyToAllAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatPPTX)
	if err != nil {
		t.Fatalf("CreateDocument(PPTX): %v", err)
	}
	operator := Operator{ID: "pptx-transition-test", Kind: OperatorHuman}

	inspection, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID})
	if err != nil {
		t.Fatalf("inspect new presentation: %v", err)
	}
	var inspected struct {
		Slides []struct {
			SlideID uint32 `json:"slide_id"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(inspection.Content, &inspected); err != nil || len(inspected.Slides) != 1 || inspected.Slides[0].SlideID == 0 {
		t.Fatalf("new presentation inspection=%s error=%v", inspection.Content, err)
	}
	sourceSlideID := inspected.Slides[0].SlideID
	duplicate, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-transition-duplicate-slide", DocumentID: doc.DocumentID, CapabilityID: "presentation.slide.duplicate",
		ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]uint32{"slide_id": sourceSlideID}),
	})
	if err != nil {
		t.Fatalf("duplicate slide for apply-to-all coverage: %v", err)
	}
	var duplicateData struct {
		SlideID uint32 `json:"slide_id"`
	}
	if err := json.Unmarshal(duplicate.Data, &duplicateData); err != nil || duplicateData.SlideID == 0 || duplicateData.SlideID == sourceSlideID {
		t.Fatalf("duplicate result=%s error=%v", duplicate.Data, err)
	}
	slideIDs := []uint32{sourceSlideID, duplicateData.SlideID}

	get := func(documentID string, slideID uint32, requestID string) (presentationTransitionTestResult, error) {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: documentID, CapabilityID: "presentation.transition.get",
			Operator: operator, Arguments: mustJSON(t, map[string]uint32{"slide_id": slideID}),
		})
		if invokeErr != nil {
			return presentationTransitionTestResult{}, invokeErr
		}
		var state presentationTransitionTestResult
		if err := json.Unmarshal(result.Data, &state); err != nil {
			return presentationTransitionTestResult{}, err
		}
		return state, nil
	}

	for index, slideID := range slideIDs {
		initial, invokeErr := get(doc.DocumentID, slideID, "pptx-transition-get-initial-"+string(rune('a'+index)))
		if invokeErr != nil || initial.SlideID != slideID || initial.Transition.Type != "none" {
			t.Fatalf("initial transition for slide %d = %+v error=%v", slideID, initial, invokeErr)
		}
	}
	stateBeforeInvalidGet, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid transition query: %v", err)
	}
	if _, err := get(doc.DocumentID, ^uint32(0), "pptx-transition-invalid-get"); err == nil {
		t.Fatal("transition query accepted a nonexistent slide_id")
	}
	stateAfterInvalidGet, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterInvalidGet.Revision != stateBeforeInvalidGet.Revision || stateAfterInvalidGet.Dirty != stateBeforeInvalidGet.Dirty {
		t.Fatalf("invalid transition query changed document state: before=%+v after=%+v error=%v", stateBeforeInvalidGet, stateAfterInvalidGet, err)
	}

	setArgs := map[string]any{
		"slide_id": sourceSlideID, "transition_type": "fade", "speed": "med",
		"through_black": true, "advance_on_click": false, "apply_to_all": true,
	}
	set, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-transition-apply-to-all", DocumentID: doc.DocumentID, CapabilityID: "presentation.transition.set",
		ExpectedRevision: duplicate.Revision, Operator: operator, Arguments: mustJSON(t, setArgs),
	})
	if err != nil || set.Revision != duplicate.Revision+1 || !containsJSONText(set.Data, `"changed":true`) {
		t.Fatalf("apply transition to all slides: result=%s revision=%d error=%v", set.Data, set.Revision, err)
	}
	var setResult struct {
		AffectedSlideIDs []uint32 `json:"affected_slide_ids"`
	}
	if err := json.Unmarshal(set.Data, &setResult); err != nil || len(setResult.AffectedSlideIDs) != 2 {
		t.Fatalf("apply-to-all result=%s error=%v", set.Data, err)
	}
	for index, slideID := range slideIDs {
		state, invokeErr := get(doc.DocumentID, slideID, "pptx-transition-get-set-"+string(rune('a'+index)))
		if invokeErr != nil || state.Transition.Type != "fade" || state.Transition.Speed != "med" || !state.Transition.ThroughBlack || state.Transition.AdvanceOnClick {
			t.Fatalf("transition on slide %d = %+v error=%v", slideID, state, invokeErr)
		}
	}

	stateBeforeInvalidSet, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid transition set: %v", err)
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-transition-invalid-combination", DocumentID: doc.DocumentID, CapabilityID: "presentation.transition.set",
		ExpectedRevision: set.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"slide_id": sourceSlideID, "transition_type": "push", "through_black": true, "apply_to_all": true}),
	}); err == nil {
		t.Fatal("transition accepted through_black for push")
	}
	stateAfterInvalidSet, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterInvalidSet.Revision != stateBeforeInvalidSet.Revision || stateAfterInvalidSet.Dirty != stateBeforeInvalidSet.Dirty {
		t.Fatalf("invalid transition patch changed document state: before=%+v after=%+v error=%v", stateBeforeInvalidSet, stateAfterInvalidSet, err)
	}

	noOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-transition-noop", DocumentID: doc.DocumentID, CapabilityID: "presentation.transition.set",
		ExpectedRevision: set.Revision, Operator: operator, Arguments: mustJSON(t, setArgs),
	})
	if err != nil || noOp.Revision != set.Revision || !containsJSONText(noOp.Data, `"changed":false`) {
		t.Fatalf("repeating transition patch should be a no-op: result=%s revision=%d error=%v", noOp.Data, noOp.Revision, err)
	}
	stateAfterNoOp, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterNoOp.Revision != set.Revision || stateAfterNoOp.Dirty != stateBeforeInvalidSet.Dirty {
		t.Fatalf("transition no-op changed revision/dirty state: before=%+v after=%+v error=%v", stateBeforeInvalidSet, stateAfterNoOp, err)
	}

	path := filepath.Join(t.TempDir(), "presentation-transition-roundtrip.pptx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("save presentation transitions: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close presentation before round trip: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen presentation: %v", err)
	}
	for index, slideID := range slideIDs {
		state, invokeErr := get(reopened.DocumentID, slideID, "pptx-transition-get-reopened-"+string(rune('a'+index)))
		if invokeErr != nil || state.Transition.Type != "fade" || state.Transition.Speed != "med" || !state.Transition.ThroughBlack || state.Transition.AdvanceOnClick {
			t.Fatalf("reopened transition on slide %d = %+v error=%v", slideID, state, invokeErr)
		}
	}
	cleanNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-transition-reopened-noop", DocumentID: reopened.DocumentID, CapabilityID: "presentation.transition.set",
		ExpectedRevision: reopened.Revision, Operator: operator, Arguments: mustJSON(t, setArgs),
	})
	if err != nil || cleanNoOp.Revision != reopened.Revision || !containsJSONText(cleanNoOp.Data, `"changed":false`) {
		t.Fatalf("transition no-op after reopen: result=%s revision=%d error=%v", cleanNoOp.Data, cleanNoOp.Revision, err)
	}
	cleanState, err := runtime.GetDocument(ctx, reopened.DocumentID)
	if err != nil || cleanState.Revision != reopened.Revision || cleanState.Dirty {
		t.Fatalf("no-op on reopened clean presentation changed state: state=%+v error=%v", cleanState, err)
	}
}

type presentationTransitionTestResult struct {
	SlideID    uint32 `json:"slide_id"`
	Transition struct {
		Type           string `json:"type"`
		Speed          string `json:"speed"`
		ThroughBlack   bool   `json:"through_black"`
		AdvanceOnClick bool   `json:"advance_on_click"`
	} `json:"transition"`
}
