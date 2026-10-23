package wordfont

import (
	"bytes"
	"context"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/fontscan"
	"glance/internal/document"
	"glance/internal/fontmanager/fontcatalog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"weak"

	"github.com/go-text/typesetting/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func TestParsedFontSharesImmutableData(t *testing.T) {
	first, err := ParseFont(goregular.TTF, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseFont(append([]byte(nil), goregular.TTF...), 0)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("identical font contents were parsed twice")
	}
	other, err := ParseFont(gomono.TTF, 0)
	if err != nil {
		t.Fatal(err)
	}
	if other == first {
		t.Fatal("different font contents share parsed data")
	}
	if _, err := ParseFont(goregular.TTF, 1); err == nil {
		t.Fatal("invalid collection index was accepted")
	}
	runtime.KeepAlive(first)
}

func TestParsedFontConcurrentReuse(t *testing.T) {
	data := append(append([]byte(nil), goregular.TTF...), 1, 2, 3)
	results := make(chan *font.Font, 16)
	var wg sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, err := ParseFont(data, 0)
			if err != nil {
				t.Error(err)
			}
			results <- f
		}()
	}
	wg.Wait()
	first := <-results
	for i := 1; i < cap(results); i++ {
		if f := <-results; f != first {
			t.Fatal("concurrent font consumers parsed independent tables")
		}
	}
}

func TestParsedFontCacheDoesNotKeepUnusedFontAlive(t *testing.T) {
	weakFont := func() weak.Pointer[font.Font] {
		data := append(append([]byte(nil), goregular.TTF...), 7, 8, 9)
		f, err := ParseFont(data, 0)
		if err != nil {
			t.Fatal(err)
		}
		return weak.Make(f)
	}()
	runtime.GC()
	if weakFont.Value() != nil {
		t.Fatal("shared parse index retains an otherwise unused font")
	}
}

func TestWordShapersShareTablesButKeepFaceCachesIndependent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "font.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	faces, err := fontcatalog.Inspect(context.Background(), path, fontcatalog.System)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := fontcatalog.NewSnapshot(1, faces)
	first, _ := NewWithSnapshot(snapshot)
	second, _ := NewWithSnapshot(snapshot)
	for _, shaper := range []*Shaper{first, second} {
		if _, err := shaper.Shape("Hello", document.WordTextStyle{FontFamily: "Go", FontSize: 12}); err != nil {
			t.Fatal(err)
		}
	}
	location := fontscan.Location{File: path, Index: 0}
	a, b := first.facesByID[location], second.facesByID[location]
	if a == nil || b == nil {
		t.Fatal("fixture was not loaded")
	}
	if a.Font != b.Font {
		t.Fatal("Word sessions duplicated immutable font tables")
	}
	if a == b {
		t.Fatal("Word sessions share mutable face caches")
	}
}

var benchmarkParsedFont *font.Font

func BenchmarkParsedFontReuse(b *testing.B) {
	for _, shared := range []bool{false, true} {
		name := "direct"
		if shared {
			name = "shared"
		}
		b.Run(name, func(b *testing.B) {
			var err error
			benchmarkParsedFont, err = ParseFont(goregular.TTF, 0)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if shared {
					benchmarkParsedFont, err = ParseFont(goregular.TTF, 0)
				} else {
					var loaders []*opentype.Loader
					loaders, err = opentype.NewLoaders(bytes.NewReader(goregular.TTF))
					if err == nil {
						benchmarkParsedFont, err = font.NewFont(loaders[0])
					}
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
