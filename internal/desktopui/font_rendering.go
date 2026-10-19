package desktopui

import (
	"container/list"
	"fmt"
	"glance/internal/document"
	"glance/internal/fontcatalog"
	"glance/internal/wordfont"
	"sort"
	"strings"
	"sync"

	giofont "gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
	fontapi "github.com/go-text/typesetting/font"
)

type fontRenderResult struct {
	key    string
	shaper *text.Shaper
}

func (a *App) prepareFontRendering() {
	p := &a.fontPicker
	if p.snapshot.Version == 0 || p.renderPending != "" {
		return
	}
	// Opening, closing, scrolling and searching the picker only change its UI;
	// they do not change the document's shaping requirements. Keep them out of
	// this key so a wheel event cannot trigger font reads and a new Shaper.
	inputKey := fmt.Sprintf("%d:%p", p.snapshot.Version, a.documentSessionState)
	if a.documentSessionState != nil {
		inputKey += fmt.Sprintf(":%d:%s:%d", a.wordRevisionAtomic.Load(), a.selectedWordRunID, a.documentGeneration)
		if a.virtualGrid != nil {
			v := a.virtualGrid
			inputKey += fmt.Sprintf("%v:%p:%p:%p:%p", v.LoadedWindows, v.Window.Cells, v.TopWindow.Cells, v.LeftWindow.Cells, v.CornerWindow.Cells)
		}
	}
	if inputKey == p.renderInputKey {
		return
	}
	p.renderInputKey = inputKey
	names := map[string]bool{}
	if a.documentSessionState != nil {
		a.wordBlocksMu.RLock()
		if run, ok := selectedWordRun(a.wordBlocks, a.selectedWordRunID); ok {
			names[run.Style.FontFamily] = true
		}
		a.wordBlocksMu.RUnlock()
		if a.virtualGrid != nil {
			for _, window := range []document.SheetWindow{a.virtualGrid.Window, a.virtualGrid.TopWindow, a.virtualGrid.LeftWindow, a.virtualGrid.CornerWindow} {
				for _, row := range window.Cells {
					for _, cell := range row {
						if cell.Style.FontFamily != "" {
							names[cell.Style.FontFamily] = true
						}
					}
				}
			}
		}
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		if name != "" {
			sorted = append(sorted, name)
		}
	}
	sort.Strings(sorted)
	key := fmt.Sprintf("%d:%s", p.snapshot.Version, strings.Join(sorted, "\x00"))
	if key == p.renderAccepted {
		return
	}
	p.renderPending = key
	snapshot := p.snapshot
	cache := p.renderCache
	if cache == nil {
		cache = newFontCollectionCache()
		p.renderCache = cache
	}
	if !a.launchManagedBackground(func() {
		collection := fontCollectionForRendering(snapshot, sorted, cache)
		result := fontRenderResult{key: key, shaper: text.NewShaper(text.WithCollection(collection))}
		select {
		case p.renderResults <- result:
		case <-a.shutdown:
			return
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}) {
		p.renderPending = ""
	}
}

const fontCollectionCacheLimit int64 = 128 << 20
const fontFamilyIndexCacheLimit = 2

type fontFamilyIndexEntry struct {
	version uint64
	byName  map[string][]fontcatalog.Family
}

type parsedFontEntry struct {
	key   string
	faces []giofont.FontFace
	bytes int64
}

type fontCollectionCache struct {
	mu          sync.Mutex
	familyItems map[uint64]*list.Element
	familyLRU   *list.List
	parsedItems map[string]*list.Element
	parsedLRU   *list.List
	parsedBytes int64
}

func newFontCollectionCache() *fontCollectionCache {
	return &fontCollectionCache{
		familyItems: make(map[uint64]*list.Element),
		familyLRU:   list.New(),
		parsedItems: make(map[string]*list.Element),
		parsedLRU:   list.New(),
	}
}

func buildFontFamilyIndex(snapshot fontcatalog.Snapshot) map[string][]fontcatalog.Family {
	index := make(map[string][]fontcatalog.Family)
	for _, family := range snapshot.Families() {
		seen := make(map[string]bool, len(family.Aliases)+1)
		for _, name := range append([]string{family.Name}, family.Aliases...) {
			key := strings.ToLower(name)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			index[key] = append(index[key], family)
		}
	}
	return index
}

func (cache *fontCollectionCache) familiesFor(snapshot fontcatalog.Snapshot, name string) []fontcatalog.Family {
	if cache == nil {
		return buildFontFamilyIndex(snapshot)[strings.ToLower(name)]
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.familyItems == nil {
		cache.familyItems = make(map[uint64]*list.Element)
		cache.familyLRU = list.New()
	}
	item := cache.familyItems[snapshot.Version]
	if item == nil {
		entry := fontFamilyIndexEntry{version: snapshot.Version, byName: buildFontFamilyIndex(snapshot)}
		item = cache.familyLRU.PushFront(entry)
		cache.familyItems[snapshot.Version] = item
		for cache.familyLRU.Len() > fontFamilyIndexCacheLimit {
			oldest := cache.familyLRU.Back()
			if oldest == nil {
				break
			}
			delete(cache.familyItems, oldest.Value.(fontFamilyIndexEntry).version)
			cache.familyLRU.Remove(oldest)
		}
	} else {
		cache.familyLRU.MoveToFront(item)
	}
	return item.Value.(fontFamilyIndexEntry).byName[strings.ToLower(name)]
}

func fontParseCacheKey(face fontcatalog.Face) string {
	key := fmt.Sprintf("%s\x00%s\x00%d", face.Path, face.Digest, face.Index)
	if face.Digest == "" {
		key += fmt.Sprintf("\x00%d:%d", face.Size, face.ModTime)
	}
	return key
}

type selectedCollectionFace struct{ font *fontapi.Font }

func (f selectedCollectionFace) Face() *fontapi.Face { return fontapi.NewFace(f.font) }

func parseFontCollection(snapshot fontcatalog.Snapshot, face fontcatalog.Face) ([]giofont.FontFace, int64, bool) {
	data, err := snapshot.ReadFace(face.ID)
	if err != nil {
		return nil, 0, false
	}
	parsed, err := wordfont.ParseFont(data, face.Index)
	if err != nil {
		return nil, 0, false
	}

	// Gio's ParseCollection eagerly parses every face, including large CJK
	// tables we never use. Wrap only this immutable font; each shaper gets its
	// own mutable Face through Face(), as in Gio's OpenType wrapper.
	selected := giofont.FontFace{Font: opentype.DescriptionToFont(parsed.Describe()), Face: selectedCollectionFace{parsed}}
	return []giofont.FontFace{selected}, int64(len(data)), true
}

func (cache *fontCollectionCache) parsedFaces(snapshot fontcatalog.Snapshot, face fontcatalog.Face) ([]giofont.FontFace, int64, bool) {
	if cache == nil {
		return parseFontCollection(snapshot, face)
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.parsedItems == nil {
		cache.parsedItems = make(map[string]*list.Element)
		cache.parsedLRU = list.New()
	}
	key := fontParseCacheKey(face)
	if item := cache.parsedItems[key]; item != nil {
		cache.parsedLRU.MoveToFront(item)
		entry := item.Value.(parsedFontEntry)
		return entry.faces, entry.bytes, true
	}
	fonts, size, ok := parseFontCollection(snapshot, face)
	if !ok || size > fontCollectionCacheLimit {
		return fonts, size, ok
	}
	for cache.parsedBytes+size > fontCollectionCacheLimit {
		oldest := cache.parsedLRU.Back()
		if oldest == nil {
			break
		}
		entry := oldest.Value.(parsedFontEntry)
		delete(cache.parsedItems, entry.key)
		cache.parsedBytes -= entry.bytes
		cache.parsedLRU.Remove(oldest)
	}
	item := cache.parsedLRU.PushFront(parsedFontEntry{key: key, faces: fonts, bytes: size})
	cache.parsedItems[key] = item
	cache.parsedBytes += size
	return fonts, size, true
}

func fontCollection(snapshot fontcatalog.Snapshot, names []string) []giofont.FontFace {
	return fontCollectionWithCache(snapshot, names, newFontCollectionCache())
}

func fontCollectionWithCache(snapshot fontcatalog.Snapshot, names []string, cache *fontCollectionCache) []giofont.FontFace {
	builder := newFontCollectionBuilder(snapshot, cache)
	builder.addFamilies(names)
	return builder.collection
}

// retainParsed drops strong references to fonts no longer needed by the current
// collection. Existing Shapers own their faces independently, so an old frame
// can finish safely even after its cache entry has been removed.
func (cache *fontCollectionCache) retainParsed(used map[string][]giofont.FontFace) {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for key, item := range cache.parsedItems {
		if _, ok := used[key]; ok {
			continue
		}
		cache.parsedBytes -= item.Value.(parsedFontEntry).bytes
		delete(cache.parsedItems, key)
		cache.parsedLRU.Remove(item)
	}
}
