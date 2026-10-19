package desktopui

import (
	"image/color"
	"sync"

	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

const roundedBorderCacheLimit = 256

type roundedBorderKey struct {
	shape clip.RRect
	width float32
	color color.NRGBA
}

type roundedBorderEntry struct {
	ops  op.Ops
	call op.CallOp
}

type roundedBorderCache struct {
	mu    sync.Mutex
	items map[roundedBorderKey]*roundedBorderEntry
	fifo  []roundedBorderKey
	next  int
}

var shellRoundedBorders roundedBorderCache

// Keep recorded paths immutable so Gio can reuse their stroked geometry across
// frames. Recording a fresh path into the frame's Ops defeats that GPU cache.
// Bound retained geometries because resizing can produce many distinct sizes.
func (cache *roundedBorderCache) border(key roundedBorderKey) op.CallOp {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if entry := cache.items[key]; entry != nil {
		return entry.call
	}
	if cache.items == nil {
		cache.items = make(map[roundedBorderKey]*roundedBorderEntry)
	}
	entry := new(roundedBorderEntry)
	record := op.Record(&entry.ops)
	paint.FillShape(&entry.ops, key.color, clip.Stroke{Path: key.shape.Path(&entry.ops), Width: key.width}.Op())
	entry.call = record.Stop()
	if len(cache.fifo) < roundedBorderCacheLimit {
		cache.fifo = append(cache.fifo, key)
	} else {
		delete(cache.items, cache.fifo[cache.next])
		cache.fifo[cache.next] = key
		cache.next = (cache.next + 1) % roundedBorderCacheLimit
	}
	cache.items[key] = entry
	return entry.call
}

func paintRoundedBorder(ops *op.Ops, shape clip.RRect, width float32, border color.NRGBA) {
	if width <= 0 || border.A == 0 {
		return
	}
	shellRoundedBorders.border(roundedBorderKey{shape: shape, width: width, color: border}).Add(ops)
}
