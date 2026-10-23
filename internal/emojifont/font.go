// Package emojifont owns the application-private bitmap emoji fallback.
package emojifont

import (
	"embed"
	"sync"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"github.com/go-text/typesetting/di"
	fontapi "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

const Family = "Glance Color Emoji"

//go:embed assets/NotoColorEmoji.ttf assets/OFL.txt assets/manifest.json
var assets embed.FS
var once sync.Once
var face opentype.Face
var loadErr error

// Preload parses immutable font data once. Call before constructing UI shapers,
// outside layout/frame callbacks. Each Face() call retains independent state.
func Preload() error {
	once.Do(func() {
		data, err := assets.ReadFile("assets/NotoColorEmoji.ttf")
		if err != nil {
			loadErr = err
			return
		}
		face, loadErr = opentype.Parse(data)
	})
	return loadErr
}
func Collection() ([]font.FontFace, error) {
	if err := Preload(); err != nil {
		return nil, err
	}
	return []font.FontFace{{Font: font.Font{Typeface: Family}, Face: face}}, nil
}

// BitmapPNG returns an embedded color bitmap glyph for a scalar the bundled
// font covers. The returned bytes are owned by the caller.
func BitmapPNG(r rune) ([]byte, int, int, bool) {
	if err := Preload(); err != nil {
		return nil, 0, 0, false
	}
	return bitmapPNGForGlyph(face.Face(), r)
}

// BitmapPNGForText shapes one grapheme with the bundled color font and returns
// its bitmap when OpenType resolves it to a single embedded PNG glyph. This
// preserves ZWJ, flag, variation-selector and keycap sequences for static
// renderers such as PDF that cannot embed bitmap font tables.
func BitmapPNGForText(value string) ([]byte, int, int, bool) {
	if value == "" || Preload() != nil {
		return nil, 0, 0, false
	}
	fontFace := face.Face()
	fontFace.SetPpem(109, 109)
	runes := []rune(value)
	var hb shaping.HarfbuzzShaper
	shaped := hb.Shape(shaping.Input{
		Text: runes, RunStart: 0, RunEnd: len(runes), Direction: di.DirectionLTR,
		Face: fontFace, Size: fixed.I(109), Script: language.Common, Language: language.NewLanguage("und"),
	})
	if len(shaped.Glyphs) != 1 {
		return nil, 0, 0, false
	}
	bitmap, ok := fontFace.GlyphData(shaped.Glyphs[0].GlyphID).(fontapi.GlyphBitmap)
	if !ok || bitmap.Format != fontapi.PNG || bitmap.Width <= 0 || bitmap.Height <= 0 || len(bitmap.Data) == 0 {
		return nil, 0, 0, false
	}
	return append([]byte(nil), bitmap.Data...), bitmap.Width, bitmap.Height, true
}

func bitmapPNGForGlyph(fontFace *fontapi.Face, r rune) ([]byte, int, int, bool) {
	glyph, ok := fontFace.NominalGlyph(r)
	if !ok || glyph == 0 {
		return nil, 0, 0, false
	}
	bitmap, ok := fontFace.GlyphData(glyph).(fontapi.GlyphBitmap)
	if !ok || bitmap.Format != fontapi.PNG || bitmap.Width <= 0 || bitmap.Height <= 0 || len(bitmap.Data) == 0 {
		return nil, 0, 0, false
	}
	return append([]byte(nil), bitmap.Data...), bitmap.Width, bitmap.Height, true
}
