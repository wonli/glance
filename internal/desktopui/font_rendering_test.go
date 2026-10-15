package desktopui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	giofont "gioui.org/font"
	"glance/internal/fontcatalog"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func TestFontRenderingUsesOneConsistentSourcePerDescriptor(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "system.ttf"), filepath.Join(dir, "private.ttf")
	os.WriteFile(a, goregular.TTF, 0600)
	os.WriteFile(b, gomono.TTF, 0600)
	sys, _ := fontcatalog.Inspect(context.Background(), a, fontcatalog.System)
	private, _ := fontcatalog.Inspect(context.Background(), b, fontcatalog.Private)
	private[0].Family = "Go"
	private[0].Aliases = []string{"Go"}
	snapshot := fontcatalog.NewSnapshot(1, append(sys, private...))
	var regularAdvance float32
	for _, prefer := range []bool{false, true} {
		if prefer {
			snapshot, _ = snapshot.Prefer(private[0].ID)
		}
		collection := fontCollection(snapshot, []string{"Go"})
		count := 0
		for _, f := range collection {
			if f.Font.Typeface == giofont.Typeface("Go") && f.Font.Weight == giofont.Normal && f.Font.Style == giofont.Regular {
				count++
				face := f.Face.Face()
				glyph, ok := face.NominalGlyph('i')
				if !ok {
					t.Fatal("fixture has no i glyph")
				}
				advance := face.HorizontalAdvance(glyph) / float32(face.Upem())
				if prefer && advance <= regularAdvance {
					t.Fatalf("private monospace selection did not change actual glyph metrics: %v <= %v", advance, regularAdvance)
				}
				if !prefer {
					regularAdvance = advance
				}
			}
		}
		if count != 1 {
			t.Fatalf("descriptor has %d competing faces", count)
		}
	}
}
