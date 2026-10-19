package desktopui

import "testing"

func TestNativeMousePointerPosition(t *testing.T) {
	for _, p := range [][2]int32{{454, 788}, {-1200, -300}, {32767, -32768}} {
		packed := nativeMousePointerPosition(p[0], p[1])
		if x, y := int32(int16(packed)), int32(int16(packed>>16)); x != p[0] || y != p[1] {
			t.Fatalf("%v -> %d,%d", p, x, y)
		}
	}
}
