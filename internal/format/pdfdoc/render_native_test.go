package pdfdoc

import "testing"

func TestNativeRenderGeometry(t *testing.T) {
	for _, scale := range []int{1, 2} {
		img := renderNative(fixture(t), 0, 596*scale, 842*scale)
		if img == nil {
			t.Skip("native unavailable")
		}
		minX, minY := 596*scale, 842*scale
		for y := 0; y < 842*scale; y++ {
			for x := 0; x < 596*scale; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r < 40000 && g < 40000 && b < 40000 {
					minX = min(minX, x)
					minY = min(minY, y)
				}
			}
		}
		if minX < 38*scale || minX > 43*scale || minY < 45*scale || minY > 55*scale {
			t.Fatalf("scale %d: text origin %d,%d", scale, minX, minY)
		}
	}
}
