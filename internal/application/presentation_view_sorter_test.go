package application

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestPresentationViewSetAcceptsSorterMode(t *testing.T) {
	if err := ValidatePresentationViewArguments(json.RawMessage(`{"operation":"set_mode","mode":"sorter"}`)); err != nil {
		t.Fatalf("ValidatePresentationViewArguments(sorter): %v", err)
	}
}

func TestPresentationViewStateSchemaAdvertisesSorterMode(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(presentationViewStateSchema(), &schema); err != nil {
		t.Fatalf("decode view state schema: %v", err)
	}
	if !slices.Contains(schema.Properties["mode"].Enum, "sorter") {
		t.Fatalf("presentation view state mode enum = %v, want sorter", schema.Properties["mode"].Enum)
	}
}

func TestPresentationViewSetAcceptsRulerVisibility(t *testing.T) {
	if err := ValidatePresentationViewArguments(json.RawMessage(`{"operation":"set_ruler_visible","enabled":true}`)); err != nil {
		t.Fatalf("ValidatePresentationViewArguments(set_ruler_visible): %v", err)
	}
}

func TestPresentationViewStateSchemaAdvertisesRulerVisibility(t *testing.T) {
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(presentationViewStateSchema(), &schema); err != nil {
		t.Fatalf("decode view state schema: %v", err)
	}
	if _, ok := schema.Properties["ruler_visible"]; !ok {
		t.Fatal("presentation view state schema is missing ruler_visible")
	}
}
