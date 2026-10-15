package desktopui

import (
	"container/list"
	"fmt"
	"glance/internal/fontcatalog"
	"io"
	"sort"
	"strings"
	"sync"

	giofont "gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
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
	// Scrolling and searching the picker only change which rows are visible;
	// they do not change the document's shaping requirements. Keep them out of
	// this key so a wheel event cannot trigger font reads and a new Shaper.
	inputKey := fmt.Sprintf("%d:%p:%t", p.snapshot.Version, a.documentSessionState, p.open)
	if a.documentSessionState != nil {
		inputKey += fmt.Sprintf(":%d:%s:%d", a.wordRevisionAtomic.Load(), a.selectedWordRunID, a.documentGeneration)
		if a.virtualGrid != nil {
			inputKey += fmt.Sprintf("%v:%p", a.virtualGrid.WindowRange, a.virtualGrid.Window.Cells)
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
			for _, row := range a.virtualGrid.Window.Cells {
				for _, cell := range row {
					if cell.Style.FontFamily != "" {
						names[cell.Style.FontFamily] = true
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
	key := fmt.Sprintf("%d:%t:%s", p.snapshot.Version, p.open, strings.Join(sorted, "\x00"))
	if key == p.renderAccepted {
		return
	}
	p.renderPending = key
	snapshot := p.snapshot
	includePrivate := p.open
	cache := p.renderCache
	if cache == nil {
		cache = newFontCollectionCache()
		p.renderCache = cache
	}
	if !a.launchManagedBackground(func() {
		if includePrivate {
			sorted = append(sorted, privateFontFamilyNames(snapshot)...)
			sort.Strings(sorted)
			unique := sorted[:0]
			for _, name := range sorted {
				if len(unique) == 0 || unique[len(unique)-1] != name {
					unique = append(unique, name)
				}
			}
			sorted = unique
		}
		collection := fontCollectionWithCache(snapshot, sorted, cache)
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

func privateFontFamilyNames(snapshot fontcatalog.Snapshot) []string {
	var names []string
	for _, family := range snapshot.Families() {
		for _, face := range family.Faces {
			if face.Source == fontcatalog.Private && face.Unavailable == "" {
				names = append(names, family.Name)
				break
			}
		}
	}
	return names
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
	key := face.Path + "\x00" + face.Digest
	if face.Digest == "" {
		key += fmt.Sprintf("\x00%d:%d", face.Size, face.ModTime)
	}
	return key
}

func parseFontCollection(snapshot fontcatalog.Snapshot, face fontcatalog.Face) ([]giofont.FontFace, int64, bool) {
	reader, err := snapshot.OpenFace(face.ID)
	if err != nil {
		return nil, 0, false
	}
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		return nil, 0, false
	}
	fonts, err := opentype.ParseCollection(data)
	if err != nil {
		return nil, 0, false
	}
	return fonts, int64(len(data)), true
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
	var collection []giofont.FontFace
	loaded := map[string][]giofont.FontFace{}
	descriptors := map[giofont.Font]bool{}
	var loadedBytes int64
	for _, name := range names {
		for _, family := range cache.familiesFor(snapshot, name) {
			for _, face := range family.Faces {
				if face.Unavailable != "" {
					continue
				}
				if loadedBytes+face.Size > 128<<20 {
					continue
				}
				parseKey := fontParseCacheKey(face)
				fonts, ok := loaded[parseKey]
				if !ok {
					var size int64
					var loadedOK bool
					fonts, size, loadedOK = cache.parsedFaces(snapshot, face)
					if !loadedOK {
						continue
					}
					loaded[parseKey] = fonts
					loadedBytes += size
				}
				if int(face.Index) >= len(fonts) {
					continue
				}
				f := fonts[face.Index]
				for _, alias := range append([]string{family.Name}, family.Aliases...) {
					copy := f
					copy.Font.Typeface = giofont.Typeface(alias)
					if !descriptors[copy.Font] {
						descriptors[copy.Font] = true
						collection = append(collection, copy)
					}
				}
			}
		}
	}
	return collection
}
