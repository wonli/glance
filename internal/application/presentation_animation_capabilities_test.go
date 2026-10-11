package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestPresentationAnimationCapabilitiesUseStableTargetsAndRoundTrip(t *testing.T) {
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
	operator := Operator{ID: "pptx-animation-test", Kind: OperatorHuman}

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
	slideID := inspected.Slides[0].SlideID

	created, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-animation-create-target", DocumentID: doc.DocumentID,
		CapabilityID: "presentation.element.textbox.create", ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "x": 80, "y": 100, "width": 180, "height": 64, "text": "季度经营摘要"}),
	})
	if err != nil {
		t.Fatalf("create animation target: %v", err)
	}
	var createdTarget struct {
		ElementID uint32 `json:"element_id"`
	}
	if err := json.Unmarshal(created.Data, &createdTarget); err != nil || createdTarget.ElementID == 0 {
		t.Fatalf("created target=%s error=%v", created.Data, err)
	}
	elementID := createdTarget.ElementID

	list := func(documentID string, targetSlideID uint32, requestID string) (presentationAnimationTestSummary, error) {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: documentID, CapabilityID: "presentation.animations.list",
			Operator: operator, Arguments: mustJSON(t, map[string]uint32{"slide_id": targetSlideID}),
		})
		if invokeErr != nil {
			return presentationAnimationTestSummary{}, invokeErr
		}
		var summary presentationAnimationTestSummary
		if err := json.Unmarshal(result.Data, &summary); err != nil {
			return presentationAnimationTestSummary{}, err
		}
		return summary, nil
	}

	initialAnimations, err := list(doc.DocumentID, slideID, "pptx-animation-list-initial")
	if err != nil || initialAnimations.HasTiming || len(initialAnimations.Animations) != 0 || initialAnimations.Revision != created.Revision {
		t.Fatalf("initial animations=%+v error=%v", initialAnimations, err)
	}
	beforeInvalidList, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid animation list: %v", err)
	}
	if _, err := list(doc.DocumentID, ^uint32(0), "pptx-animation-list-invalid-slide"); err == nil {
		t.Fatal("animation list accepted a nonexistent slide_id")
	}
	afterInvalidList, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || afterInvalidList.Revision != beforeInvalidList.Revision || afterInvalidList.Dirty != beforeInvalidList.Dirty {
		t.Fatalf("invalid animation query changed document state: before=%+v after=%+v error=%v", beforeInvalidList, afterInvalidList, err)
	}

	added, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-animation-add", DocumentID: doc.DocumentID, CapabilityID: "presentation.element.animation.add",
		ExpectedRevision: created.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "element_id": elementID, "effect": "fade", "duration_ms": 800}),
	})
	if err != nil || added.Revision != created.Revision+1 || !containsJSONText(added.Data, `"changed":true`) {
		t.Fatalf("add fade animation: result=%s revision=%d error=%v", added.Data, added.Revision, err)
	}
	stateAfterAdd, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterAdd.Revision != added.Revision || !stateAfterAdd.Dirty {
		t.Fatalf("animation add state=%+v error=%v", stateAfterAdd, err)
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-animation-invalid-target", DocumentID: doc.DocumentID, CapabilityID: "presentation.element.animation.add",
		ExpectedRevision: added.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "element_id": ^uint32(0), "effect": "fade", "duration_ms": 800}),
	}); err == nil {
		t.Fatal("animation add accepted a nonexistent element_id")
	}
	stateAfterInvalidTarget, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterInvalidTarget.Revision != added.Revision || stateAfterInvalidTarget.Dirty != stateAfterAdd.Dirty {
		t.Fatalf("invalid animation target changed document state: before=%+v after=%+v error=%v", stateAfterAdd, stateAfterInvalidTarget, err)
	}

	animations, err := list(doc.DocumentID, slideID, "pptx-animation-list-after-add")
	if err != nil || !animations.HasTiming || animations.Truncated || len(animations.Animations) < 2 {
		t.Fatalf("animations after add=%+v error=%v", animations, err)
	}
	var animationIndex int
	for _, animation := range animations.Animations {
		if animation.Kind == "animEffect" && animation.TargetElementID == elementID {
			animationIndex = animation.Index
			if animation.Duration != "800" {
				t.Fatalf("new animation duration=%q; want 800", animation.Duration)
			}
		}
	}
	if animationIndex == 0 {
		t.Fatalf("animation list does not include an animEffect for element %d: %+v", elementID, animations.Animations)
	}

	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-animation-duration-invalid-target", DocumentID: doc.DocumentID, CapabilityID: "presentation.element.animation.duration.set",
		ExpectedRevision: added.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "element_id": elementID + 1, "animation_index": animationIndex, "duration_ms": 1200}),
	}); err == nil {
		t.Fatal("animation duration accepted an index belonging to another target")
	}
	stateBeforeDuration, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateBeforeDuration.Revision != added.Revision || stateBeforeDuration.Dirty != stateAfterAdd.Dirty {
		t.Fatalf("invalid animation duration changed document state: before=%+v after=%+v error=%v", stateAfterAdd, stateBeforeDuration, err)
	}

	durationSet, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-animation-duration-set", DocumentID: doc.DocumentID, CapabilityID: "presentation.element.animation.duration.set",
		ExpectedRevision: added.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "element_id": elementID, "animation_index": animationIndex, "duration_ms": 1200}),
	})
	if err != nil || durationSet.Revision != added.Revision+1 || !containsJSONText(durationSet.Data, `"changed":true`) {
		t.Fatalf("set animation duration: result=%s revision=%d error=%v", durationSet.Data, durationSet.Revision, err)
	}
	durationNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-animation-duration-noop", DocumentID: doc.DocumentID, CapabilityID: "presentation.element.animation.duration.set",
		ExpectedRevision: durationSet.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "element_id": elementID, "animation_index": animationIndex, "duration_ms": 1200}),
	})
	if err != nil || durationNoOp.Revision != durationSet.Revision || !containsJSONText(durationNoOp.Data, `"changed":false`) {
		t.Fatalf("repeat duration should be a no-op: result=%s revision=%d error=%v", durationNoOp.Data, durationNoOp.Revision, err)
	}
	stateAfterNoOp, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterNoOp.Revision != durationSet.Revision || stateAfterNoOp.Dirty != stateAfterAdd.Dirty {
		t.Fatalf("duration no-op changed revision/dirty state: before=%+v after=%+v error=%v", stateAfterAdd, stateAfterNoOp, err)
	}

	path := filepath.Join(t.TempDir(), "presentation-animation-roundtrip.pptx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("save presentation with animation: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close presentation before round trip: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen presentation: %v", err)
	}
	reopenedAnimations, err := list(reopened.DocumentID, slideID, "pptx-animation-list-after-reopen")
	if err != nil || !reopenedAnimations.HasTiming || reopenedAnimations.Revision != reopened.Revision {
		t.Fatalf("reopened animations=%+v error=%v", reopenedAnimations, err)
	}
	var persisted bool
	for _, animation := range reopenedAnimations.Animations {
		if animation.Kind == "animEffect" && animation.TargetElementID == elementID && animation.Duration == "1200" {
			persisted = true
		}
	}
	if !persisted {
		t.Fatalf("saved animation target/duration not preserved after reopen: %+v", reopenedAnimations.Animations)
	}
	cleanNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-animation-duration-reopened-noop", DocumentID: reopened.DocumentID, CapabilityID: "presentation.element.animation.duration.set",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "element_id": elementID, "animation_index": animationIndex, "duration_ms": 1200}),
	})
	if err != nil || cleanNoOp.Revision != reopened.Revision || !containsJSONText(cleanNoOp.Data, `"changed":false`) {
		t.Fatalf("reopened duration should remain an idempotent no-op: result=%s revision=%d error=%v", cleanNoOp.Data, cleanNoOp.Revision, err)
	}
	cleanState, err := runtime.GetDocument(ctx, reopened.DocumentID)
	if err != nil || cleanState.Revision != reopened.Revision || cleanState.Dirty {
		t.Fatalf("no-op on reopened clean presentation changed state: state=%+v error=%v", cleanState, err)
	}

	var deleteAnimationIndex int
	for _, animation := range reopenedAnimations.Animations {
		if animation.Kind == "animEffect" && animation.TargetElementID == elementID {
			deleteAnimationIndex = animation.Index
			break
		}
	}
	if deleteAnimationIndex == 0 {
		t.Fatalf("reopened animations do not contain the target effect: %+v", reopenedAnimations.Animations)
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-animation-delete-invalid-target", DocumentID: reopened.DocumentID, CapabilityID: "presentation.element.animation.delete",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "element_id": elementID + 1, "animation_index": deleteAnimationIndex}),
	}); err == nil {
		t.Fatal("animation delete accepted an index belonging to another target")
	}
	stateAfterInvalidDelete, err := runtime.GetDocument(ctx, reopened.DocumentID)
	if err != nil || stateAfterInvalidDelete.Revision != reopened.Revision || stateAfterInvalidDelete.Dirty {
		t.Fatalf("invalid animation delete changed document state: state=%+v error=%v", stateAfterInvalidDelete, err)
	}

	deleted, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-animation-delete", DocumentID: reopened.DocumentID, CapabilityID: "presentation.element.animation.delete",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"slide_id": slideID, "element_id": elementID, "animation_index": deleteAnimationIndex}),
	})
	var deleteResult struct {
		Changed               bool `json:"changed"`
		RemovedAnimationNodes int  `json:"removed_animation_nodes"`
	}
	if err == nil {
		err = json.Unmarshal(deleted.Data, &deleteResult)
	}
	if err != nil || deleted.Revision != reopened.Revision+1 || !deleteResult.Changed || deleteResult.RemovedAnimationNodes != 2 {
		t.Fatalf("delete animation: result=%s revision=%d error=%v", deleted.Data, deleted.Revision, err)
	}
	afterDelete, err := list(reopened.DocumentID, slideID, "pptx-animation-list-after-delete")
	if err != nil || len(afterDelete.Animations) != 0 {
		t.Fatalf("animation group remains after deletion: animations=%+v error=%v", afterDelete.Animations, err)
	}
	stateAfterDelete, err := runtime.GetDocument(ctx, reopened.DocumentID)
	if err != nil || stateAfterDelete.Revision != deleted.Revision || !stateAfterDelete.Dirty {
		t.Fatalf("animation delete state=%+v error=%v", stateAfterDelete, err)
	}

	deletedPath := filepath.Join(t.TempDir(), "presentation-animation-deleted.pptx")
	if _, err := runtime.SaveDocumentAs(ctx, reopened.DocumentID, deletedPath); err != nil {
		t.Fatalf("save presentation after animation deletion: %v", err)
	}
	if err := runtime.CloseDocument(ctx, reopened.DocumentID); err != nil {
		t.Fatalf("close presentation after animation deletion: %v", err)
	}
	deletedReopened, err := runtime.OpenDocument(ctx, deletedPath)
	if err != nil {
		t.Fatalf("reopen presentation after animation deletion: %v", err)
	}
	persistedDelete, err := list(deletedReopened.DocumentID, slideID, "pptx-animation-list-after-delete-reopen")
	if err != nil || len(persistedDelete.Animations) != 0 {
		t.Fatalf("deleted animation returned after reopen: animations=%+v error=%v", persistedDelete.Animations, err)
	}
}

type presentationAnimationTestSummary struct {
	Revision   uint64 `json:"revision"`
	HasTiming  bool   `json:"has_timing"`
	Truncated  bool   `json:"truncated"`
	Animations []struct {
		Index           int    `json:"index"`
		Kind            string `json:"kind"`
		TargetElementID uint32 `json:"target_element_id"`
		Duration        string `json:"duration"`
	} `json:"animations"`
}
