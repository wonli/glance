package desktopui

import (
	"strings"

	giofont "gioui.org/font"
	"glance/internal/fontmanager/fontcatalog"
	"glance/internal/fontmanager/fontfallback"
)

// uiFontFaces selects the normal and semibold faces for the existing shell,
// code-editor, CJK, and emoji family chains. The family order matches the
// current theme; the builder below decides which faces to parse.
func uiFontFaces(snapshot fontcatalog.Snapshot, cache *fontCollectionCache) []fontcatalog.Face {
	families := uiTargetFamilies(snapshot, cache)
	var selected []fontcatalog.Face
	seen := map[fontcatalog.ResourceID]bool{}
	for _, weight := range []int{400, 600} {
		for _, family := range families {
			var best fontcatalog.Face
			found := false
			for _, face := range family.Faces {
				if face.Unavailable != "" || face.Italic {
					continue
				}
				if !found || uiWeightLess(face.Weight, best.Weight, weight) {
					best, found = face, true
				}
			}
			if found && !seen[best.ID] {
				selected = append(selected, best)
				seen[best.ID] = true
			}
		}
	}
	return selected
}

func uiTargetFamilies(snapshot fontcatalog.Snapshot, cache *fontCollectionCache) []fontcatalog.Family {
	// Pick the first available family for each functional typography role:
	// 1. UI Sans (Inter, Arial, Helvetica, sans-serif)
	// 2. CJK (PingFang SC, Hiragino Sans GB, Microsoft YaHei, Noto Sans CJK SC, ...)
	// 3. Monospace (Menlo, Monaco, Consolas, Courier New, ...)
	roles := [][]string{
		{"Inter", "Arial", "Helvetica", "sans-serif"},
		fontfallback.CJKFamilies(),
		fontfallback.MonospaceFamilies(),
	}
	var families []fontcatalog.Family
	seenFamilies := map[string]bool{}

	for _, role := range roles {
		for _, name := range role {
			matches := cache.familiesFor(snapshot, name)
			if len(matches) > 0 {
				for _, family := range matches {
					key := strings.ToLower(strings.TrimSpace(family.Name))
					if !seenFamilies[key] {
						seenFamilies[key] = true
						families = append(families, family)
					}
				}
				break
			}
		}
	}

	if len(families) == 0 {
		names := append(fontfallback.PreferredFamilies(), fontfallback.MonospaceFamilies()...)
		for _, name := range names {
			key := strings.ToLower(strings.TrimSpace(name))
			if key == "" || seenFamilies[key] {
				continue
			}
			seenFamilies[key] = true
			families = append(families, cache.familiesFor(snapshot, name)...)
		}
	}
	return families
}

// Use Gio/fontscan's CSS search direction, rather than absolute distance.
// In particular, 400 searches up to 500 first; 600 searches heavier first.
func uiWeightLess(a, b, query int) bool {
	score := func(weight int) (int, int) {
		if query >= 400 && query <= 500 {
			if weight >= query && weight <= 500 {
				return 0, weight - query
			}
			if weight < query {
				return 1, query - weight
			}
			return 2, weight - query
		}
		if query < 400 {
			if weight <= query {
				return 0, query - weight
			}
			return 1, weight - query
		}
		if weight >= query {
			return 0, weight - query
		}
		return 1, query - weight
	}
	ag, ad := score(a)
	bg, bd := score(b)
	return ag < bg || ag == bg && ad < bd
}

func fontCollectionForRendering(snapshot fontcatalog.Snapshot, names []string, cache *fontCollectionCache) []giofont.FontFace {
	builder := newFontCollectionBuilder(snapshot, cache)
	// Preserve requested document/private fonts before optional UI preloads.
	builder.addFamilies(names)
	for _, face := range uiFontFaces(snapshot, cache) {
		builder.add(face, append([]string{face.Family}, face.Aliases...))
	}
	cache.retainParsed(builder.loaded)
	return builder.collection
}

func hasUsableUIFontCollection(collection []giofont.FontFace) bool {
	preferred := fontfallback.PreferredFamilies()
	for _, face := range collection {
		family := strings.TrimSpace(string(face.Font.Typeface))
		if family == "" || strings.Contains(strings.ToLower(family), "emoji") {
			continue
		}
		for _, name := range preferred {
			if strings.EqualFold(family, name) {
				return true
			}
		}
	}
	return false
}
