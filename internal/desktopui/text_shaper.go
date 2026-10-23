package desktopui

import (
	"fmt"
	"runtime"
	"strings"

	"gioui.org/font"
	"gioui.org/text"
	"glance/internal/fontmanager/emojifont"
	"glance/internal/fontmanager/fontfallback"
)

// Construct shapers during initialization or on managed workers, never layout.
// Each collection face produces its own mutable state over shared font bytes.
func newTextShaper(collection []font.FontFace) *text.Shaper {
	return newTextShaperForOS(runtime.GOOS, collection)
}
func newTextShaperForOS(goos string, collection []font.FontFace, options ...text.ShaperOption) *text.Shaper {
	if !hasTypeface(collection, emojifont.Family) {
		emoji, err := emojifont.Collection()
		if err != nil {
			panic(fmt.Errorf("initialize bundled emoji font: %w", err))
		}
		collection = append(append([]font.FontFace(nil), collection...), emoji...)
	}
	options = append(options, text.NoSystemFonts(), text.WithCollection(collection))
	return text.NewShaper(options...)
}

func hasTypeface(collection []font.FontFace, family string) bool {
	for _, face := range collection {
		if strings.EqualFold(strings.TrimSpace(string(face.Font.Typeface)), family) {
			return true
		}
	}
	return false
}

func desktopPreferredTypeface() font.Typeface {
	preferred := fontfallback.PreferredFamilies()
	families := make([]string, 0, len(preferred)+1)
	hasBundledEmoji := false
	for _, family := range preferred {
		families = append(families, family)
		if strings.EqualFold(strings.TrimSpace(family), emojifont.Family) {
			hasBundledEmoji = true
		}
		if !hasBundledEmoji && strings.EqualFold(strings.TrimSpace(family), "Apple Color Emoji") {
			families = append(families, emojifont.Family)
			hasBundledEmoji = true
		}
	}
	if !hasBundledEmoji {
		families = append(families, emojifont.Family)
	}
	return font.Typeface(strings.Join(families, ", "))
}

// newSystemTextShaper is reserved for windowless test and utility App values.
// Desktop windows use newTextShaper with an explicit manager collection.
func newSystemTextShaper() *text.Shaper { return text.NewShaper() }
