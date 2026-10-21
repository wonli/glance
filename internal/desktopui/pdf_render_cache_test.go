package desktopui

import "testing"

func TestPDFCanvasDPITracksPhysicalOutputDensity(t *testing.T) {
	tests := []struct {
		name  string
		scale float64
		want  float64
	}{
		{name: "one-times display at 100 percent", scale: 4.0 / 3.0, want: 96},
		{name: "retina display at 100 percent", scale: 8.0 / 3.0, want: 192},
		{name: "retina display at 150 percent", scale: 4, want: 288},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pdfCanvasDPI(tt.scale); got != tt.want {
				t.Fatalf("pdfCanvasDPI(%v) = %v, want %v", tt.scale, got, tt.want)
			}
		})
	}
}
