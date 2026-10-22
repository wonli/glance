package desktopui

import (
	"context"
	"math"

	"gioui.org/font"
	"gioui.org/text"
	"glance/internal/application"
	"glance/internal/proofreading"
	"golang.org/x/image/math/fixed"
)

// Shape on a worker-owned Gio shaper, with the same document-coordinate
// parameters as the native editor. Sharing the UI shaper across goroutines
// would race its mutable glyph iterator and caches.
func reflowProofreadingForEditor(ctx context.Context, snapshot proofreading.Snapshot, geometry []application.ProofreadingPageGeometry, style application.ProofreadingTextStyle, initial bool) (application.ProofreadingLayoutSnapshot, error) {
	return application.ReflowProofreadingLines(ctx, snapshot, geometry, style, newProofreadingEditorLineShaper(), initial)
}

func newProofreadingEditorLineShaper() application.ProofreadingLineShaper {
	shaper := text.NewShaper()
	shape := func(ctx context.Context, value string, width float64, style application.ProofreadingTextStyle) ([]application.ProofreadingLine, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		shaper.LayoutString(text.Parameters{
			Font:       font.Font{Typeface: font.Typeface(style.Family)},
			PxPerEm:    fixed.I(int(math.Round(style.Size))),
			MaxWidth:   max(1, int(math.Round(width))),
			WrapPolicy: text.WrapWords, LineHeightScale: 1.28,
			DisableSpaceTrim: true,
		}, value)
		var lines []application.ProofreadingLine
		offset, start := 0, 0
		base, y := 0, 0
		for glyph, ok := shaper.NextGlyph(); ok; glyph, ok = shaper.NextGlyph() {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if len(lines) == 0 {
				base = int(glyph.Y)
			}
			y = int(glyph.Y) - base
			offset += int(glyph.Runes)
			if glyph.Flags&text.FlagLineBreak != 0 {
				if len(lines) > 0 {
					lines[len(lines)-1].Bottom = float64(y)
				}
				lines = append(lines, application.ProofreadingLine{Start: start, End: offset, Top: float64(y), Bottom: float64(y) + math.Round(style.Size)*1.28})
				start = offset
			}
		}
		return lines, nil
	}
	return shape
}
