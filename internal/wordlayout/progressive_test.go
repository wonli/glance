package wordlayout

import (
	"context"
	"fmt"
	"testing"

	"glance/internal/document"
)

func TestProgressiveLayoutCallback(t *testing.T) {
	var blocks []document.WordBlock
	for i := 0; i < 30; i++ {
		blocks = append(blocks, paragraphBlock(fmt.Sprintf("p%d", i), fmt.Sprintf("Paragraph line content %d", i), document.WordTextStyle{FontSize: 12}))
	}

	input := layoutTestInput(200, 100, 10, blocks...)
	var callbackInvocations []int
	input.ProgressiveCallback = func(partial Result) {
		callbackInvocations = append(callbackInvocations, len(partial.Pages))
		if len(partial.Pages) < 2 {
			t.Errorf("expected at least 2 pages on initial callback, got %d", len(partial.Pages))
		}
		if !partial.CanPreview {
			t.Errorf("expected CanPreview to be true")
		}
	}

	engine := NewEngine(layoutFakeShaper{})
	result, err := engine.Layout(context.Background(), input)
	if err != nil {
		t.Fatalf("Layout error: %v", err)
	}

	if len(callbackInvocations) == 0 {
		t.Fatalf("expected progressive callback to be called, but was not called")
	}
	if callbackInvocations[0] < 2 {
		t.Errorf("first callback should deliver at least 2 pages, got %d", callbackInvocations[0])
	}
	if len(result.Pages) <= callbackInvocations[0] {
		t.Errorf("total pages %d should be greater than initial partial pages %d", len(result.Pages), callbackInvocations[0])
	}
}

func TestProgressiveLayoutCallbackNotInvokedWhenPreviousLayoutExists(t *testing.T) {
	var blocks []document.WordBlock
	for i := 0; i < 30; i++ {
		blocks = append(blocks, paragraphBlock(fmt.Sprintf("p%d", i), fmt.Sprintf("Paragraph line content %d", i), document.WordTextStyle{FontSize: 12}))
	}

	engine := NewEngine(layoutFakeShaper{})
	initialInput := layoutTestInput(200, 100, 10, blocks...)
	initialResult, err := engine.Layout(context.Background(), initialInput)
	if err != nil {
		t.Fatalf("Initial Layout error: %v", err)
	}

	// Change geometry to force a full reflow rather than the unchanged fast path.
	initialInput.DefaultPageSetup.HeightPT += 10
	// Now reflow with an edit (e.g. style change) where Previous is set
	editInput := layoutTestInput(200, 100, 10, blocks...)
	editInput.DefaultPageSetup = initialInput.DefaultPageSetup
	editInput.Previous = &initialResult
	callbackCalled := false
	editInput.ProgressiveCallback = func(partial Result) {
		callbackCalled = true
	}

	_, err = engine.Layout(context.Background(), editInput)
	if err != nil {
		t.Fatalf("Edit Layout error: %v", err)
	}

	if callbackCalled {
		t.Fatalf("ProgressiveCallback should NOT be invoked during reflow when input.Previous is non-nil")
	}
}
