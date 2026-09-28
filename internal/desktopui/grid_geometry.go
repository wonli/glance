package desktopui

import (
	"errors"
	"fmt"
	"math"
)

// AxisGeometry maps dimension indexes to content coordinates without storing
// a widget or a size value for every row or column. The Fenwick tree stores
// deltas from the default size; overrides contains only non-default sizes.
type AxisGeometry struct {
	count       int
	defaultSize float64
	deltaTree   []float64
	overrides   map[int]float64
	maxVisible  int
}

func NewAxisGeometry(count int, defaultSize float64) (*AxisGeometry, error) {
	if count <= 0 {
		return nil, fmt.Errorf("dimension count must be positive: %d", count)
	}
	if !finite(defaultSize) || defaultSize <= 0 {
		return nil, fmt.Errorf("default dimension size must be finite and positive: %v", defaultSize)
	}
	return &AxisGeometry{
		count:       count,
		defaultSize: defaultSize,
		deltaTree:   make([]float64, count+1),
		overrides:   make(map[int]float64),
		maxVisible:  count - 1,
	}, nil
}

// Boundary returns the content coordinate at the start of index. The final
// valid boundary is count and denotes the end of the axis.
func (a *AxisGeometry) Boundary(index int) (float64, error) {
	if index < 0 || index > a.count {
		return 0, fmt.Errorf("dimension boundary %d outside [0,%d]", index, a.count)
	}
	return float64(index)*a.defaultSize + a.prefixDelta(index), nil
}

// IndexAt returns the dimension containing position. Zero-sized dimensions
// are skipped. Coordinates outside the axis clamp to the first or last visible
// dimension; non-finite positions and an entirely hidden axis return -1.
func (a *AxisGeometry) IndexAt(position float64) int {
	if a == nil || a.count == 0 || !finite(position) {
		return -1
	}
	total := a.TotalSize()
	if total <= 0 {
		return -1
	}
	if position < 0 {
		position = 0
	} else if position >= total {
		position = math.Nextafter(total, math.Inf(-1))
	}

	// Fenwick lower-bound search. At every step compare the candidate prefix
	// against its default-size contribution plus accumulated size deltas.
	index, delta := 0, float64(0)
	for bit := highestPowerOfTwo(a.count); bit != 0; bit >>= 1 {
		next := index + bit
		if next > a.count {
			continue
		}
		candidateDelta := delta + a.deltaTree[next]
		candidatePrefix := float64(next)*a.defaultSize + candidateDelta
		if candidatePrefix <= position {
			index = next
			delta = candidateDelta
		}
	}
	if index >= a.count {
		return a.lastVisibleIndex()
	}
	return index
}

// VisibleRange returns the half-open dimension interval intersecting the
// viewport, expanded by overscan and clamped to the axis.
func (a *AxisGeometry) VisibleRange(offset, extent float64, overscan int) (start, end int) {
	if a == nil || a.count == 0 || !finite(offset) || !finite(extent) || extent <= 0 || a.TotalSize() <= 0 {
		return 0, 0
	}
	if offset < 0 {
		offset = 0
	}
	if overscan < 0 {
		overscan = 0
	}
	if overscan > a.count {
		overscan = a.count
	}
	if offset >= a.TotalSize() {
		offset = math.Nextafter(a.TotalSize(), math.Inf(-1))
	}
	first := a.IndexAt(offset)
	endPosition := offset + extent
	if !finite(endPosition) || endPosition >= a.TotalSize() {
		endPosition = math.Nextafter(a.TotalSize(), math.Inf(-1))
	} else if endPosition > offset {
		endPosition = math.Nextafter(endPosition, math.Inf(-1))
	}
	last := a.IndexAt(endPosition)
	if first < 0 || last < first {
		return 0, 0
	}
	start = first - overscan
	if start < 0 {
		start = 0
	}
	end = last + 1 + overscan
	if end > a.count {
		end = a.count
	}
	return start, end
}

func (a *AxisGeometry) SetSize(index int, size float64) error {
	if a == nil || index < 0 || index >= a.count {
		return fmt.Errorf("dimension index %d outside [0,%d)", index, axisCount(a))
	}
	if !finite(size) || size < 0 {
		return fmt.Errorf("dimension size must be finite and non-negative: %v", size)
	}
	old := a.sizeAt(index)
	if old == size {
		return nil
	}
	newTotal := a.TotalSize() + size - old
	if !finite(newTotal) || newTotal < 0 {
		return errors.New("dimension update would produce an invalid axis extent")
	}
	if size == a.defaultSize {
		delete(a.overrides, index)
	} else {
		a.overrides[index] = size
	}
	a.addDelta(index+1, size-old)
	if size > 0 && index > a.maxVisible {
		a.maxVisible = index
	} else if size == 0 && index == a.maxVisible {
		a.maxVisible = -1
		for candidate := index - 1; candidate >= 0; candidate-- {
			if a.sizeAt(candidate) > 0 {
				a.maxVisible = candidate
				break
			}
		}
	}
	return nil
}

func (a *AxisGeometry) TotalSize() float64 {
	if a == nil {
		return 0
	}
	return float64(a.count)*a.defaultSize + a.prefixDelta(a.count)
}

func (a *AxisGeometry) Clone() *AxisGeometry {
	if a == nil {
		return nil
	}
	clone := &AxisGeometry{count: a.count, defaultSize: a.defaultSize, deltaTree: append([]float64(nil), a.deltaTree...), overrides: make(map[int]float64, len(a.overrides)), maxVisible: a.maxVisible}
	for i, size := range a.overrides {
		clone.overrides[i] = size
	}
	return clone
}

// Scale converts all stored geometry to a different pixel density while
// preserving the sparse overrides and their relative dimensions.
func (a *AxisGeometry) Scale(factor float64) error {
	if a == nil || !finite(factor) || factor <= 0 {
		return fmt.Errorf("geometry scale must be finite and positive: %v", factor)
	}
	if factor == 1 {
		return nil
	}
	a.defaultSize *= factor
	for i := range a.deltaTree {
		a.deltaTree[i] *= factor
	}
	for i, size := range a.overrides {
		a.overrides[i] = size * factor
	}
	return nil
}

func (a *AxisGeometry) sizeAt(index int) float64 {
	if size, ok := a.overrides[index]; ok {
		return size
	}
	return a.defaultSize
}

func (a *AxisGeometry) prefixDelta(index int) float64 {
	var delta float64
	for i := index; i > 0; i -= i & -i {
		delta += a.deltaTree[i]
	}
	return delta
}

func (a *AxisGeometry) addDelta(index int, delta float64) {
	for i := index; i <= a.count; i += i & -i {
		a.deltaTree[i] += delta
	}
}

func (a *AxisGeometry) lastVisibleIndex() int {
	if a == nil {
		return -1
	}
	return a.maxVisible
}

func highestPowerOfTwo(value int) int {
	bit := 1
	for bit <= value/2 {
		bit <<= 1
	}
	return bit
}

func axisCount(a *AxisGeometry) int {
	if a == nil {
		return 0
	}
	return a.count
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
