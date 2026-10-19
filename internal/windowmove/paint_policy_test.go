package windowmove

import "testing"

func TestDeferMovePaint(t *testing.T) {
	normal := geometry{width: 800, height: 600, dpi: 96, normal: true, valid: true}
	changed := normal
	changed.dpi = 144
	for _, tc := range []struct {
		name               string
		dragging, resizing bool
		before, after      geometry
		want               bool
	}{
		{"pure move pause", true, false, normal, normal, true},
		{"outside gesture", false, false, normal, normal, false},
		{"resize gesture", true, true, normal, normal, false},
		{"DPI changed", true, false, normal, changed, false},
		{"missing baseline", true, false, geometry{}, normal, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := deferMovePaint(tc.dragging, tc.resizing, tc.before, tc.after); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
