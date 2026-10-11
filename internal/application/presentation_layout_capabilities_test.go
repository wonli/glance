package application

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

func TestPresentationLayoutCapabilitiesUseLiveCatalogAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	fixturePath := filepath.Join("..", "..", "testdata", "office-corpus", "pptx", "layouts.pptx")
	doc, err := runtime.OpenDocument(ctx, fixturePath)
	if err != nil {
		t.Fatalf("OpenDocument(layout fixture): %v", err)
	}
	documentID := doc.DocumentID
	requestNumber := 0
	invoke := func(capabilityID string, args any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		info := mustGetDocumentInfo(t, runtime, documentID)
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("pptx-layout-%d", requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "layout-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, args),
		})
	}
	inspectSlide := func() (uint32, string) {
		t.Helper()
		snapshot, inspectErr := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: documentID})
		if inspectErr != nil {
			t.Fatalf("InspectDocument: %v", inspectErr)
		}
		var output struct {
			Slides []struct {
				ID    uint32 `json:"slide_id"`
				Title string `json:"title"`
			} `json:"slides"`
		}
		if err := json.Unmarshal(snapshot.Content, &output); err != nil {
			t.Fatalf("decode presentation inspection: %v; content=%s", err, snapshot.Content)
		}
		if len(output.Slides) == 0 || output.Slides[0].ID == 0 {
			t.Fatalf("unexpected blank presentation: %+v", output)
		}
		return output.Slides[0].ID, output.Slides[0].Title
	}
	listLayouts := func() (uint64, []struct {
		Index int    `json:"index"`
		Name  string `json:"name"`
	}) {
		t.Helper()
		result, invokeErr := invoke("presentation.layouts.list", map[string]any{})
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(presentation.layouts.list): %v", invokeErr)
		}
		var output struct {
			Revision uint64 `json:"revision"`
			Total    int    `json:"total"`
			Layouts  []struct {
				Index int    `json:"index"`
				Name  string `json:"name"`
			} `json:"layouts"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode layout catalog: %v; data=%s", err, result.Data)
		}
		if output.Revision != result.Revision || output.Total < 2 || len(output.Layouts) < 2 || output.Layouts[0].Index != 0 || output.Layouts[1].Index != 1 {
			t.Fatalf("layout catalog is incomplete or stale: %+v invocation=%+v", output, result)
		}
		return output.Revision, output.Layouts
	}
	assertInvalidTargetAtomic := func(slideID uint32, layoutIndex int) {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, documentID)
		beforeSlideID, beforeTitle := inspectSlide()
		if _, invokeErr := invoke("presentation.slide.layout.set", map[string]any{"slide_id": slideID, "layout_index": layoutIndex}); invokeErr == nil {
			t.Fatalf("layout set accepted invalid target slide=%d layout=%d", slideID, layoutIndex)
		}
		after := mustGetDocumentInfo(t, runtime, documentID)
		afterSlideID, afterTitle := inspectSlide()
		if after.Revision != before.Revision || after.Dirty != before.Dirty || afterSlideID != beforeSlideID || afterTitle != beforeTitle {
			t.Fatalf("invalid layout request partially changed presentation: before=%+v after=%+v before slide=%d/%q after=%d/%q", before, after, beforeSlideID, beforeTitle, afterSlideID, afterTitle)
		}
	}

	slideID, title := inspectSlide()
	initialInfo := mustGetDocumentInfo(t, runtime, documentID)
	catalogRevision, layouts := listLayouts()
	if catalogRevision != initialInfo.Revision {
		t.Fatalf("initial layout read did not bind to current document: revision=%d info=%+v title=%q", catalogRevision, initialInfo, title)
	}
	assertInvalidTargetAtomic(slideID+100000, layouts[1].Index)
	assertInvalidTargetAtomic(slideID, len(layouts))

	setLayout := func(index int, wantChanged *bool) CapabilityInvocationResult {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, documentID)
		result, invokeErr := invoke("presentation.slide.layout.set", map[string]any{"slide_id": slideID, "layout_index": index})
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(presentation.slide.layout.set): %v", invokeErr)
		}
		var output struct {
			Changed     bool   `json:"changed"`
			SlideID     uint32 `json:"slide_id"`
			LayoutIndex int    `json:"layout_index"`
			LayoutName  string `json:"layout_name"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode layout result: %v; data=%s", err, result.Data)
		}
		if wantChanged != nil && output.Changed != *wantChanged || output.SlideID != slideID || output.LayoutIndex != index || output.LayoutName == "" {
			t.Fatalf("layout result=%+v want changed=%v slide=%d index=%d", output, wantChanged, slideID, index)
		}
		after := mustGetDocumentInfo(t, runtime, documentID)
		wantRevision := before.Revision
		if output.Changed {
			wantRevision++
		}
		if after.Revision != wantRevision || after.Dirty != (before.Dirty || output.Changed) {
			t.Fatalf("layout state: before=%+v after=%+v changed=%v", before, after, output.Changed)
		}
		return result
	}
	chosenLayout := -1
	for _, layout := range layouts {
		result := setLayout(layout.Index, nil)
		var output struct {
			Changed bool `json:"changed"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode layout probe: %v", err)
		}
		if output.Changed {
			chosenLayout = layout.Index
			break
		}
	}
	if chosenLayout < 0 {
		t.Fatal("layout catalog did not offer a change for the first slide")
	}
	wantChanged := false
	setLayout(chosenLayout, &wantChanged)
	path := filepath.Join(t.TempDir(), "layout.pptx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if info := mustGetDocumentInfo(t, runtime, documentID); info.Dirty {
		t.Fatalf("saved presentation remains dirty: %+v", info)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after save: %v", err)
	}
	documentID = reopened.DocumentID
	reopenedSlideID, reopenedTitle := inspectSlide()
	if reopened.Dirty || reopenedSlideID != slideID || reopenedTitle != title {
		t.Fatalf("reopened slide state: info=%+v slide=%d/%q want %d/%q", reopened, reopenedSlideID, reopenedTitle, slideID, title)
	}
	setLayout(chosenLayout, &wantChanged) // The selected layout must remain attached after reopen.
}
