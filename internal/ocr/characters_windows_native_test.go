//go:build windows

package ocr

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Opt in on Windows with an installed OCR language. This probes the actual
// PowerShell/WinRT bridge without requiring a PDF, network, or cache writes.
func TestWindowsNativeOCR(t *testing.T) {
	if os.Getenv("GLANCE_TEST_WINDOWS_OCR") != "1" {
		t.Skip("set GLANCE_TEST_WINDOWS_OCR=1 on Windows with an OCR language installed")
	}
	small := image.NewRGBA(image.Rect(0, 0, 240, 35))
	draw.Draw(small, small.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	d := font.Drawer{Dst: small, Src: image.NewUniform(color.Black), Face: basicfont.Face7x13, Dot: fixed.P(10, 23)}
	d.DrawString("HELLO WORLD 1234")
	large := image.NewRGBA(image.Rect(0, 0, 960, 140))
	xdraw.NearestNeighbor.Scale(large, large.Bounds(), small, small.Bounds(), draw.Src, nil)
	var data bytes.Buffer
	if err := png.Encode(&data, large); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lines, err := recognizeCharacterImage(ctx, data.Bytes())
	if err != nil || len(lines) == 0 {
		t.Fatalf("Windows OCR returned no lines: %v", err)
	}
	t.Logf("Windows OCR recognized %d lines", len(lines))
}
