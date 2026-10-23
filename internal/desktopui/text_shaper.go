package desktopui

import (
	"fmt"
	"runtime"

	"gioui.org/font"
	"gioui.org/text"
	"glance/internal/emojifont"
)

// Construct shapers during initialization or on managed workers, never layout.
// Each collection face produces its own mutable state over shared font bytes.
func newTextShaper(collection []font.FontFace) *text.Shaper {
	return newTextShaperForOS(runtime.GOOS, collection)
}
func newTextShaperForOS(goos string, collection []font.FontFace, options ...text.ShaperOption) *text.Shaper {
	if goos == "windows" {
		emoji, err := emojifont.Collection()
		if err != nil {
			panic(fmt.Errorf("initialize bundled emoji font: %w", err))
		}
		collection = append(append([]font.FontFace(nil), collection...), emoji...)
	}
	options = append(options, text.WithCollection(collection))
	return text.NewShaper(options...)
}
