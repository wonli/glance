package desktopui

import (
	"math"
	"testing"
)

func TestAxisGeometryBoundaryAndIndexRoundTrip(t *testing.T) {
	axis, err := NewAxisGeometry(5, 10)
	if err != nil {
		t.Fatal(err)
	}
	wants := []float64{0, 10, 20, 30, 40, 50}
	for boundary, want := range wants {
		got, err := axis.Boundary(boundary)
		if err != nil || got != want {
			t.Fatalf("Boundary(%d) = %v, %v; want %v, nil", boundary, got, err, want)
		}
	}
	for _, item := range []struct {
		position float64
		want     int
	}{{0, 0}, {9.5, 0}, {10, 1}, {29.9, 2}, {49.9, 4}, {50, 4}, {-5, 0}} {
		if got := axis.IndexAt(item.position); got != item.want {
			t.Errorf("IndexAt(%v) = %d; want %d", item.position, got, item.want)
		}
	}
}

func TestAxisGeometrySparseSizeOverridesAndHiddenDimensions(t *testing.T) {
	axis, err := NewAxisGeometry(5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := axis.SetSize(1, 15); err != nil {
		t.Fatal(err)
	}
	if err := axis.SetSize(3, 0); err != nil {
		t.Fatal(err)
	}
	wants := []float64{0, 10, 25, 35, 35, 45}
	for boundary, want := range wants {
		got, err := axis.Boundary(boundary)
		if err != nil || got != want {
			t.Fatalf("Boundary(%d) = %v, %v; want %v, nil", boundary, got, err, want)
		}
	}
	if got := axis.IndexAt(35); got != 4 {
		t.Fatalf("IndexAt(35) = %d; want row after hidden dimension (4)", got)
	}
	if got := axis.TotalSize(); got != 45 {
		t.Fatalf("TotalSize() = %v; want 45", got)
	}
	start, end := axis.VisibleRange(10, 20, 1)
	if start != 0 || end != 4 {
		t.Fatalf("VisibleRange(10, 20, 1) = [%d,%d); want [0,4)", start, end)
	}
	if start, end := axis.VisibleRange(0, 0, 2); start != 0 || end != 0 {
		t.Fatalf("zero-extent VisibleRange = [%d,%d); want empty range", start, end)
	}

	allHidden, err := NewAxisGeometry(2, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := allHidden.SetSize(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := allHidden.SetSize(1, 0); err != nil {
		t.Fatal(err)
	}
	if got := allHidden.IndexAt(0); got != -1 {
		t.Fatalf("IndexAt on all-hidden axis = %d; want -1", got)
	}
}

func TestAxisGeometryBoundsAndInvalidSizes(t *testing.T) {
	axis, err := NewAxisGeometry(4, 8)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := axis.Boundary(-1); err == nil {
		t.Fatal("Boundary(-1) should fail")
	}
	if _, err := axis.Boundary(5); err == nil {
		t.Fatal("Boundary(count+1) should fail")
	}
	for _, item := range []struct {
		index int
		size  float64
	}{{-1, 1}, {4, 1}, {1, -1}, {1, math.NaN()}, {1, math.Inf(1)}} {
		if err := axis.SetSize(item.index, item.size); err == nil {
			t.Errorf("SetSize(%d, %v) should fail", item.index, item.size)
		}
	}
	if got := axis.IndexAt(math.NaN()); got != -1 {
		t.Fatalf("IndexAt(NaN) = %d; want -1", got)
	}
	if start, end := axis.VisibleRange(0, math.Inf(1), 0); start != 0 || end != 0 {
		t.Fatalf("VisibleRange with infinite extent = [%d,%d); want empty range", start, end)
	}
	if start, end := axis.VisibleRange(math.NaN(), 10, 0); start != 0 || end != 0 {
		t.Fatalf("VisibleRange with NaN offset = [%d,%d); want empty range", start, end)
	}
}

func TestAxisGeometryRejectsInvalidConstruction(t *testing.T) {
	for _, item := range []struct {
		count int
		size  float64
	}{{0, 10}, {-1, 10}, {1, 0}, {1, -1}, {1, math.NaN()}, {1, math.Inf(1)}} {
		if _, err := NewAxisGeometry(item.count, item.size); err == nil {
			t.Errorf("NewAxisGeometry(%d, %v) should fail", item.count, item.size)
		}
	}
	axis, err := NewAxisGeometry(1_048_576, 20)
	if err != nil {
		t.Fatal(err)
	}
	last := 1_048_575
	got, err := axis.Boundary(last)
	if err != nil || got != float64(last)*20 {
		t.Fatalf("last row boundary = %v, %v; want %v, nil", got, err, float64(last)*20)
	}
	if got := axis.IndexAt(float64(last)*20 + 1); got != last {
		t.Fatalf("IndexAt near final row = %d; want %d", got, last)
	}
}
