package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

type presentationBackgroundArguments struct {
	SlideID  uint32 `json:"slide_id"`
	ColorHex string `json:"color_hex"`
}

type presentationBackgroundCase struct {
	name      string
	arguments presentationBackgroundArguments
}

type testPresentationgeometrySlideIdColorHexArguments struct {
	SlideId  uint32 `json:"slide_id"`
	ColorHex string `json:"color_hex"`
}

func TestPresentationBackgroundAndCanvasSizeCapabilitiesRoundTrip(t *testing.T) {
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
	operator := Operator{ID: "pptx-geometry-test", Kind: OperatorHuman}

	inspect := func(documentID string) (presentationGeometryTestResult, error) {
		t.Helper()
		result, inspectErr := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: documentID})
		if inspectErr != nil {
			return presentationGeometryTestResult{}, inspectErr
		}
		var state presentationGeometryTestResult
		if err := json.Unmarshal(result.Content, &state); err != nil {
			return presentationGeometryTestResult{}, err
		}
		return state, nil
	}

	initial, err := inspect(doc.DocumentID)
	if err != nil || len(initial.Slides) != 1 || initial.Slides[0].SlideID == 0 || initial.SlideWidthPT <= 0 || initial.SlideHeightPT <= 0 {
		t.Fatalf("initial presentation geometry=%+v error=%v", initial, err)
	}
	slideID := initial.Slides[0].SlideID

	background, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-background-set", DocumentID: doc.DocumentID, CapabilityID: "presentation.slide.background.set",
		ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: mustJSON(t, testPresentationgeometrySlideIdColorHexArguments{SlideId: slideID, ColorHex: "#aBcDef"}),
	})
	if err != nil || background.Revision != doc.Revision+1 || !containsJSONText(background.Data, `"color_hex":"#ABCDEF"`) || !containsJSONText(background.Data, `"changed":true`) {
		t.Fatalf("set slide background: result=%s revision=%d error=%v", background.Data, background.Revision, err)
	}
	backgroundState, err := inspect(doc.DocumentID)
	if err != nil || len(backgroundState.Slides) != 1 || backgroundState.Slides[0].BackgroundColorHex != "#ABCDEF" {
		t.Fatalf("background not reflected in live inspect: state=%+v error=%v", backgroundState, err)
	}
	backgroundNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-background-noop", DocumentID: doc.DocumentID, CapabilityID: "presentation.slide.background.set",
		ExpectedRevision: background.Revision, Operator: operator,
		Arguments: mustJSON(t, testPresentationgeometrySlideIdColorHexArguments{SlideId: slideID, ColorHex: "#abcdef"}),
	})
	if err != nil || backgroundNoOp.Revision != background.Revision || !containsJSONText(backgroundNoOp.Data, `"changed":false`) {
		t.Fatalf("normalized background repeat should be a no-op: result=%s revision=%d error=%v", backgroundNoOp.Data, backgroundNoOp.Revision, err)
	}

	stateBeforeInvalidBackground, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid background: %v", err)
	}
	for _, testCase := range []presentationBackgroundCase{
		{name: "color", arguments: presentationBackgroundArguments{SlideID: slideID, ColorHex: "not-a-color"}},
		{name: "slide", arguments: presentationBackgroundArguments{SlideID: ^uint32(0), ColorHex: "#112233"}},
	} {
		if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "pptx-background-invalid-" + testCase.name, DocumentID: doc.DocumentID, CapabilityID: "presentation.slide.background.set",
			ExpectedRevision: background.Revision, Operator: operator, Arguments: mustJSON(t, testCase.arguments),
		}); err == nil {
			t.Fatalf("background set accepted invalid %s", testCase.name)
		}
	}
	stateAfterInvalidBackground, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterInvalidBackground.Revision != stateBeforeInvalidBackground.Revision || stateAfterInvalidBackground.Dirty != stateBeforeInvalidBackground.Dirty {
		t.Fatalf("invalid background request changed document state: before=%+v after=%+v error=%v", stateBeforeInvalidBackground, stateAfterInvalidBackground, err)
	}

	const width, height = 900.0, 506.25
	sizeSet, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-canvas-size-set", DocumentID: doc.DocumentID, CapabilityID: "presentation.slide.size.set",
		ExpectedRevision: background.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]float64{"width_pt": width, "height_pt": height}),
	})
	if err != nil || sizeSet.Revision != background.Revision+1 || !containsJSONText(sizeSet.Data, `"changed":true`) {
		t.Fatalf("set canvas size: result=%s revision=%d error=%v", sizeSet.Data, sizeSet.Revision, err)
	}
	sizeNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-canvas-size-noop", DocumentID: doc.DocumentID, CapabilityID: "presentation.slide.size.set",
		ExpectedRevision: sizeSet.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]float64{"width_pt": width, "height_pt": height}),
	})
	if err != nil || sizeNoOp.Revision != sizeSet.Revision || !containsJSONText(sizeNoOp.Data, `"changed":false`) {
		t.Fatalf("repeated canvas size should be a no-op: result=%s revision=%d error=%v", sizeNoOp.Data, sizeNoOp.Revision, err)
	}
	stateBeforeInvalidSize, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid canvas size: %v", err)
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-canvas-size-invalid", DocumentID: doc.DocumentID, CapabilityID: "presentation.slide.size.set",
		ExpectedRevision: sizeSet.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]float64{"width_pt": 5000, "height_pt": height}),
	}); err == nil {
		t.Fatal("canvas size accepted a width above the supported maximum")
	}
	stateAfterInvalidSize, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterInvalidSize.Revision != stateBeforeInvalidSize.Revision || stateAfterInvalidSize.Dirty != stateBeforeInvalidSize.Dirty {
		t.Fatalf("invalid canvas size changed document state: before=%+v after=%+v error=%v", stateBeforeInvalidSize, stateAfterInvalidSize, err)
	}

	path := filepath.Join(t.TempDir(), "presentation-geometry-roundtrip.pptx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("save presentation geometry: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close presentation before round trip: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen presentation: %v", err)
	}
	reopenedState, err := inspect(reopened.DocumentID)
	if err != nil || len(reopenedState.Slides) != 1 || reopenedState.SlideWidthPT != width || reopenedState.SlideHeightPT != height || reopenedState.Slides[0].SlideID != slideID || reopenedState.Slides[0].BackgroundColorHex != "#ABCDEF" {
		t.Fatalf("presentation geometry did not round-trip: state=%+v error=%v", reopenedState, err)
	}
	cleanBackgroundNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-background-reopened-noop", DocumentID: reopened.DocumentID, CapabilityID: "presentation.slide.background.set",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, testPresentationgeometrySlideIdColorHexArguments{SlideId: slideID, ColorHex: "#abcdef"}),
	})
	if err != nil || cleanBackgroundNoOp.Revision != reopened.Revision || !containsJSONText(cleanBackgroundNoOp.Data, `"changed":false`) {
		t.Fatalf("reopened background no-op: result=%s revision=%d error=%v", cleanBackgroundNoOp.Data, cleanBackgroundNoOp.Revision, err)
	}
	cleanSizeNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-size-reopened-noop", DocumentID: reopened.DocumentID, CapabilityID: "presentation.slide.size.set",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]float64{"width_pt": width, "height_pt": height}),
	})
	if err != nil || cleanSizeNoOp.Revision != reopened.Revision || !containsJSONText(cleanSizeNoOp.Data, `"changed":false`) {
		t.Fatalf("reopened canvas-size no-op: result=%s revision=%d error=%v", cleanSizeNoOp.Data, cleanSizeNoOp.Revision, err)
	}
	cleanState, err := runtime.GetDocument(ctx, reopened.DocumentID)
	if err != nil || cleanState.Revision != reopened.Revision || cleanState.Dirty {
		t.Fatalf("no-op on reopened clean presentation changed state: state=%+v error=%v", cleanState, err)
	}
}

type presentationGeometryTestResult struct {
	SlideWidthPT  float64 `json:"slide_width_pt"`
	SlideHeightPT float64 `json:"slide_height_pt"`
	Slides        []struct {
		SlideID            uint32 `json:"slide_id"`
		BackgroundColorHex string `json:"background_color_hex"`
	} `json:"slides"`
}
