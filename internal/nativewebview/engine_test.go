package nativewebview

import (
	"math"
	"testing"
)

func TestValidatePointerEvent(t *testing.T) {
	if err := validatePointerEvent(PointerEvent{Kind: PointerWheel, X: 12.5, Y: 4, DeltaY: -120}); err != nil {
		t.Fatalf("valid wheel event rejected: %v", err)
	}
	for _, event := range []PointerEvent{
		{Kind: "unknown"},
		{Kind: PointerMove, X: math.NaN()},
		{Kind: PointerWheel, DeltaY: math.Inf(1)},
		{Kind: PointerDown, Button: 8},
	} {
		if err := validatePointerEvent(event); err == nil {
			t.Fatalf("invalid event accepted: %+v", event)
		}
	}
}

func TestBlockedExternalRequestPatterns(t *testing.T) {
	want := []string{"http://*/*", "https://*/*", "file://*/*", "ftp://*/*", "ws://*/*", "wss://*/*"}
	got := blockedExternalRequestPatterns()
	if len(got) != len(want) {
		t.Fatalf("blocked URL patterns = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("blocked URL patterns = %v, want %v", got, want)
		}
	}
}
