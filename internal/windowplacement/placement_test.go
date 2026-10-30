package windowplacement

import (
	"image"
	"testing"
)

func TestFitToWorkArea(t *testing.T) {
	tests := []struct {
		name               string
		window, work, want image.Rectangle
	}{
		{"already visible", image.Rect(100, 100, 900, 700), image.Rect(0, 0, 1920, 1040), image.Rect(100, 100, 900, 700)},
		{"off right and bottom", image.Rect(1500, 700, 2300, 1300), image.Rect(0, 0, 1920, 1040), image.Rect(1120, 440, 1920, 1040)},
		{"oversized high dpi", image.Rect(100, 100, 2020, 1330), image.Rect(0, 0, 1366, 728), image.Rect(68, 36, 1297, 691)},
		{"only height oversized", image.Rect(10, 10, 1290, 830), image.Rect(0, 0, 1366, 768), image.Rect(68, 38, 1297, 729)},
		{"negative monitor origin", image.Rect(-2200, -200, -1200, 500), image.Rect(-1920, 0, 0, 1040), image.Rect(-1920, 0, -920, 700)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitToWorkArea(tt.window, tt.work); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
