package application

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

type presentationTextProofreadPayload struct {
	Start int `json:"start"`
	Limit int `json:"limit"`
}

type presentationElementTextboxCreatePayload struct {
	SlideId uint32 `json:"slide_id"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Text    string `json:"text"`
}

type presentationSlideAddPayload struct {
}

func TestPresentationProofreadCapabilityPaginatesStableFindingsWithoutMutation(t *testing.T) {
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
	agent := Operator{ID: "proofread-agent", Kind: OperatorExternalAgent}
	revision := doc.Revision
	requestNumber := 0
	invoke := func(capabilityID string, args any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID:  fmt.Sprintf("presentation-proofread-%s-%d", capabilityID, requestNumber),
			DocumentID: doc.DocumentID, CapabilityID: capabilityID, ExpectedRevision: revision,
			Operator: agent, Arguments: mustJSON(t, args),
		})
		if invokeErr == nil {
			revision = result.Revision
		}
		return result, invokeErr
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 1000})
	if err != nil {
		t.Fatalf("inspect initial slide: %v", err)
	}
	var initial struct {
		Slides []struct {
			ID uint32 `json:"slide_id"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(snapshot.Content, &initial); err != nil || len(initial.Slides) != 1 || initial.Slides[0].ID == 0 {
		t.Fatalf("initial PPTX snapshot=%s error=%v", snapshot.Content, err)
	}
	firstSlideID := initial.Slides[0].ID
	added, err := invoke("presentation.slide.add", presentationSlideAddPayload{})
	if err != nil {
		t.Fatalf("add second slide: %v", err)
	}
	var addedSlide struct {
		SlideNumber int `json:"slide_number"`
	}
	if err := json.Unmarshal(added.Data, &addedSlide); err != nil || addedSlide.SlideNumber != 2 {
		t.Fatalf("added slide result=%s error=%v", added.Data, err)
	}
	snapshot, err = runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 1000})
	if err != nil {
		t.Fatalf("inspect added slide: %v", err)
	}
	initial.Slides = nil
	if err := json.Unmarshal(snapshot.Content, &initial); err != nil || len(initial.Slides) != 2 || initial.Slides[1].ID == 0 || initial.Slides[1].ID == firstSlideID {
		t.Fatalf("PPTX snapshot after add=%s error=%v", snapshot.Content, err)
	}
	secondSlideID := initial.Slides[1].ID
	textBox, err := invoke("presentation.element.textbox.create", presentationElementTextboxCreatePayload{SlideId: secondSlideID, X: 50, Y: 50, Width: 320, Height: 80, Text: "预算，，（待复核"})
	if err != nil {
		t.Fatalf("create proofread text: %v", err)
	}
	var textBoxResult struct {
		ElementID int64 `json:"element_id"`
	}
	if err := json.Unmarshal(textBox.Data, &textBoxResult); err != nil || textBoxResult.ElementID <= 0 {
		t.Fatalf("text box result=%s error=%v", textBox.Data, err)
	}
	before := mustGetDocumentInfo(t, runtime, doc.DocumentID)

	firstPage, err := invoke("presentation.text.proofread", presentationTextProofreadPayload{Start: 0, Limit: 1})
	if err != nil {
		t.Fatalf("proofread first page: %v", err)
	}
	var firstResult struct {
		Start         int  `json:"start"`
		ScannedSlides int  `json:"scanned_slides"`
		TotalSlides   int  `json:"total_slides"`
		Complete      bool `json:"complete"`
		HasMore       bool `json:"has_more"`
		Findings      []struct {
			Code        string `json:"code"`
			SlideID     uint32 `json:"slide_id"`
			SlideNumber int    `json:"slide_number"`
			ElementID   int64  `json:"element_id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(firstPage.Data, &firstResult); err != nil {
		t.Fatalf("decode first proofread page: %v data=%s", err, firstPage.Data)
	}
	if firstResult.Start != 0 || firstResult.ScannedSlides != 1 || firstResult.TotalSlides != 2 || firstResult.Complete || !firstResult.HasMore || len(firstResult.Findings) != 0 {
		t.Fatalf("first proofread page=%+v", firstResult)
	}

	secondPage, err := invoke("presentation.text.proofread", presentationTextProofreadPayload{Start: 1, Limit: 1})
	if err != nil {
		t.Fatalf("proofread second page: %v", err)
	}
	var secondResult struct {
		Start         int  `json:"start"`
		ScannedSlides int  `json:"scanned_slides"`
		TotalSlides   int  `json:"total_slides"`
		Complete      bool `json:"complete"`
		HasMore       bool `json:"has_more"`
		Findings      []struct {
			Code        string `json:"code"`
			SlideID     uint32 `json:"slide_id"`
			SlideNumber int    `json:"slide_number"`
			ElementID   int64  `json:"element_id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(secondPage.Data, &secondResult); err != nil {
		t.Fatalf("decode second proofread page: %v data=%s", err, secondPage.Data)
	}
	if secondResult.Start != 1 || secondResult.ScannedSlides != 1 || secondResult.TotalSlides != 2 || !secondResult.Complete || secondResult.HasMore || len(secondResult.Findings) != 2 {
		t.Fatalf("second proofread page=%+v", secondResult)
	}
	wantCodes := map[string]bool{"repeated_punctuation": true, "unbalanced_delimiters": true}
	for _, finding := range secondResult.Findings {
		if !wantCodes[finding.Code] || finding.SlideID != secondSlideID || finding.SlideNumber != 2 || finding.ElementID != textBoxResult.ElementID {
			t.Fatalf("proofread finding=%+v; want stable second-slide target", finding)
		}
		delete(wantCodes, finding.Code)
	}
	if len(wantCodes) != 0 {
		t.Fatalf("missing proofread findings: %v", wantCodes)
	}
	if _, err := invoke("presentation.text.proofread", presentationTextProofreadPayload{Start: 3, Limit: 1}); err == nil {
		t.Fatal("proofread accepted a page start beyond the slide count")
	}
	if after := mustGetDocumentInfo(t, runtime, doc.DocumentID); after.Revision != before.Revision || after.Dirty != before.Dirty {
		t.Fatalf("proofread changed document state: before=%+v after=%+v", before, after)
	}
}
