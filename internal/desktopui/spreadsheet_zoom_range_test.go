package desktopui

import (
	"math"
	"testing"
)

func TestSpreadsheetZoomRangeMatchesExcelLimitsAndSliderEndpoints(t *testing.T) {
	if got := spreadsheetZoomClamp(0.01); got != 0.1 {
		t.Fatalf("minimum zoom clamp = %.2f; want 0.10", got)
	}
	if got := spreadsheetZoomClamp(8); got != 4 {
		t.Fatalf("maximum zoom clamp = %.2f; want 4.00", got)
	}
	for _, test := range []struct {
		zoom float64
		frac float64
	}{
		{zoom: 0.1, frac: 0},
		{zoom: 1, frac: 0.5},
		{zoom: 4, frac: 1},
	} {
		if got := spreadsheetZoomFraction(test.zoom); got != test.frac {
			t.Errorf("zoom %.2f slider fraction = %.2f; want %.2f", test.zoom, got, test.frac)
		}
		if got := spreadsheetZoomFromFraction(test.frac); math.Abs(got-test.zoom) > 1e-9 {
			t.Errorf("slider fraction %.2f zoom = %.2f; want %.2f", test.frac, got, test.zoom)
		}
	}
}

func TestSpreadsheetZoomSliderMappingRoundTripsWithinExcelLimits(t *testing.T) {
	for _, want := range []float64{0.1, 0.25, 0.5, 1, 1.25, 2, 3, 4} {
		frac := spreadsheetZoomFraction(want)
		if got := spreadsheetZoomFromFraction(frac); math.Abs(got-want) > 1e-9 {
			t.Errorf("zoom %.2f maps through fraction %.6f to %.6f", want, frac, got)
		}
	}
}
