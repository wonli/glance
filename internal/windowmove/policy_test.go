package windowmove

import "testing"

func TestMovePolicy(t *testing.T) {
	const move = 0x0001 | 0x0004
	base := geometry{width: 1280, height: 820, dpi: 96, normal: true, valid: true}
	for _, tc := range []struct {
		name     string
		flags    uint32
		dragging bool
		change   func(*geometry)
		want     bool
	}{
		{"pure move", move, true, nil, true},
		{"outside native drag", move, false, nil, false},
		{"resize flags", 0x0004, true, nil, false},
		{"z order", 0x0001, true, nil, false},
		{"no move", move | 0x0002, true, nil, false},
		{"frame", move | 0x0020, true, nil, false},
		{"show", move | 0x0040, true, nil, false},
		{"hide", move | 0x0080, true, nil, false},
		{"changed size", move, true, func(g *geometry) { g.width++ }, false},
		{"changed height", move, true, func(g *geometry) { g.height++ }, false},
		{"changed DPI", move, true, func(g *geometry) { g.dpi = 144 }, false},
		{"maximized or minimized", move, true, func(g *geometry) { g.normal = false }, false},
		{"failed geometry query", move, true, func(g *geometry) { g.valid = false }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := base
			if tc.change != nil {
				tc.change(&next)
			}
			if got := skipPositionRedraw(tc.flags, tc.dragging, base, next); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	if skipPositionRedraw(move, true, geometry{}, base) {
		t.Fatal("missing baseline allowed")
	}
}

func TestEnabled(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{{"", true}, {"1", true}, {"0", false}, {"false", false}, {"off", false}, {"FALSE", false}} {
		t.Setenv("GLANCE_WINDOWS_MOVE_REDRAW", tc.value)
		if got := Enabled(); got != tc.want {
			t.Fatalf("%q: got %v", tc.value, got)
		}
	}
}
