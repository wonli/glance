//go:build darwin && cgo

package printservice

import (
	"context"
	"testing"
)

func TestDarwinPrinterRejectsInvalidPDFBeforeShowingNativePanel(t *testing.T) {
	printer := New()
	if !printer.Available() {
		t.Fatal("macOS native printer should be available in a cgo build")
	}
	if err := printer.Print(context.Background(), []byte("not a PDF"), Options{}); err == nil {
		t.Fatal("invalid PDF unexpectedly reached the native print operation")
	}
}
