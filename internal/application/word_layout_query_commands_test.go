package application

import (
	"context"
	"encoding/json"
	"testing"
)

func TestWordPageSetupQueryReturnsNeutralSectionsWithoutChangingRevision(t *testing.T) {
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

	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-layout-query", DocumentID: doc.DocumentID,
		CapabilityID: "word.page_setup.get", ExpectedRevision: doc.Revision,
		Operator: Operator{ID: "layout-reader", Kind: OperatorExternalAgent}, Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("InvokeCapability: %v", err)
	}
	var data struct {
		Sections []struct {
			StartBlock int `json:"start_block"`
			PageSetup  struct {
				WidthPoints  float64  `json:"width_points"`
				HeightPoints float64  `json:"height_points"`
				Orientation  string   `json:"orientation"`
				Explicit     []string `json:"explicit_fields"`
			} `json:"page_setup"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("decode page setup result: %v", err)
	}
	if len(data.Sections) == 0 || data.Sections[0].StartBlock != 0 {
		t.Fatalf("page setup sections = %+v; want a body-start section", data.Sections)
	}
	setup := data.Sections[0].PageSetup
	if setup.WidthPoints <= 0 || setup.HeightPoints <= 0 || !containsString(setup.Explicit, "width_points") {
		t.Fatalf("page setup geometry = %+v; want explicit page dimensions in points", setup)
	}
	current, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if current.Revision != doc.Revision || current.Dirty != doc.Dirty {
		t.Fatalf("page setup query changed document state: before=%+v after=%+v", doc, current)
	}
}
