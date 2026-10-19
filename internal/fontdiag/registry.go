// Package fontdiag keeps opt-in, bounded, weak observations of application font
// parsing. Gio's private system font cache is not observed; use pprof for that.
package fontdiag

import (
	"container/list"
	"github.com/go-text/typesetting/font"
	"sync"
	"sync/atomic"
	"weak"
)

type Use struct {
	Consumer string `json:"consumer"`
	Path     string `json:"path"`
	Index    uint32 `json:"index"`
}
type Font struct {
	SourceBytes int64  `json:"source_bytes"`
	CacheHits   uint64 `json:"cache_hits"`
	Alive       bool   `json:"alive"`
	Uses        []Use  `json:"uses"`
}
type Snapshot struct {
	Enabled   bool   `json:"enabled"`
	Parses    uint64 `json:"parses"`
	CacheHits uint64 `json:"cache_hits"`
	Evicted   uint64 `json:"evicted"`
	Fonts     []Font `json:"fonts"`
}
type entry struct {
	pointer weak.Pointer[font.Font]
	info    Font
}
type registry struct {
	mu                    sync.Mutex
	limit                 int
	entries               map[weak.Pointer[font.Font]]*list.Element
	order                 list.List
	parses, hits, evicted uint64
}

func newRegistry(limit int) *registry {
	return &registry{limit: limit, entries: make(map[weak.Pointer[font.Font]]*list.Element)}
}
func (r *registry) get(f *font.Font) *entry {
	key := weak.Make(f)
	if el := r.entries[key]; el != nil {
		return el.Value.(*entry)
	}
	e := &entry{pointer: key}
	r.entries[key] = r.order.PushBack(e)
	if r.order.Len() > r.limit {
		old := r.order.Front()
		delete(r.entries, old.Value.(*entry).pointer)
		r.order.Remove(old)
		r.evicted++
	}
	return e
}
func (r *registry) recordParse(f *font.Font, size int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.parses++
	r.get(f).info.SourceBytes = size
}
func (r *registry) recordHit(f *font.Font) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hits++
	r.get(f).info.CacheHits++
}
func (r *registry) recordUse(f *font.Font, consumer, path string, index uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.get(f)
	use := Use{consumer, path, index}
	for _, old := range e.info.Uses {
		if old == use {
			return
		}
	}
	if len(e.info.Uses) < 16 {
		e.info.Uses = append(e.info.Uses, use)
	}
}
func (r *registry) snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := Snapshot{Enabled: true, Parses: r.parses, CacheHits: r.hits, Evicted: r.evicted, Fonts: make([]Font, 0, r.order.Len())}
	for el := r.order.Front(); el != nil; el = el.Next() {
		e := el.Value.(*entry)
		info := e.info
		info.Alive = e.pointer.Value() != nil
		info.Uses = append([]Use(nil), info.Uses...)
		out.Fonts = append(out.Fonts, info)
	}
	return out
}

var enabled atomic.Bool
var processRegistry = newRegistry(256)

// Enable is called only after the profiling listener is explicitly enabled.
func Enable() { enabled.Store(true) }
func RecordParse(f *font.Font, sourceBytes int64) {
	if enabled.Load() {
		processRegistry.recordParse(f, sourceBytes)
	}
}
func RecordHit(f *font.Font) {
	if enabled.Load() {
		processRegistry.recordHit(f)
	}
}
func RecordUse(f *font.Font, consumer, path string, index uint32) {
	if enabled.Load() {
		processRegistry.recordUse(f, consumer, path, index)
	}
}
func Read() Snapshot {
	if !enabled.Load() {
		return Snapshot{}
	}
	return processRegistry.snapshot()
}
