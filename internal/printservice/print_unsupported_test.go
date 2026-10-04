//go:build !darwin || !cgo

package printservice

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedPlatformPrintIsUnavailable(t *testing.T) {
	printer := New()
	if printer.Available() {
		t.Fatal("unsupported platform unexpectedly reports native printing")
	}
	if err := printer.Print(context.Background(), []byte("pdf"), Options{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Print error = %v, want ErrUnavailable", err)
	}
}
