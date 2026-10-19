package desktopui

import (
	giofont "gioui.org/font"
	"glance/internal/fontcatalog"
	"glance/internal/fontfallback"
)

// uiFontFaces selects the normal and semibold faces used by shell text. Loading
// every weight/italic of a CJK family would cost more than the sharing saves.
// This only selects metadata; the builder below decides which faces to parse.
func uiFontFaces(snapshot fontcatalog.Snapshot, cache *fontCollectionCache) []fontcatalog.Face {
	var families []fontcatalog.Family
	if matches := cache.familiesFor(snapshot, "Inter"); len(matches) > 0 {
		families = append(families, matches...)
	}
	for _, name := range fontfallback.CJKFamilies() {
		matches := cache.familiesFor(snapshot, name)
		usable := false
		for _, family := range matches {
			for _, face := range family.Faces {
				if face.Unavailable == "" && !face.Italic {
					usable = true
					break
				}
			}
		}
		if usable {
			families = append(families, matches...)
			break
		}
	}
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
		// Gio registers collection fonts with stretch zero, but system faces use
		// normal stretch. Its matcher therefore loads the system face anyway.
		// Avoid a redundant UI parse until that upstream mismatch is fixed;
		// keep private/online UI sources and explicit document requests intact.
		if face.Source != fontcatalog.System {
			builder.add(face, append([]string{face.Family}, face.Aliases...))
		}
	}
	cache.retainParsed(builder.loaded)
	return builder.collection
}
