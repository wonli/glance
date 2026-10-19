package fontdiag

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"golang.org/x/image/font/gofont/goregular"
)

func fixtureFont(t *testing.T) *font.Font {
	t.Helper()
	loaders, err := opentype.NewLoaders(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatal(err)
	}
	f, err := font.NewFont(loaders[0])
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestRegistryTracksSharedConsumersWithoutRetainingFont(t *testing.T) {
	r := newRegistry(2)
	func() {
		f := fixtureFont(t)
		r.recordParse(f, int64(len(goregular.TTF)))
		r.recordHit(f)
		r.recordUse(f, "word", "example.ttf", 0)
		r.recordUse(f, "ui", "example.ttf", 0)
		got := r.snapshot()
		if got.Parses != 1 || got.CacheHits != 1 || len(got.Fonts) != 1 || !got.Fonts[0].Alive || len(got.Fonts[0].Uses) != 2 {
			t.Fatalf("bad shared-font inventory: %+v", got)
		}
		runtime.KeepAlive(f)
	}()
	runtime.GC()
	if got := r.snapshot(); got.Fonts[0].Alive {
		t.Fatal("diagnostic inventory retained font")
	}
}

func TestRegistryBoundsHistoricalEntries(t *testing.T) {
	r := newRegistry(2)
	var alive []*font.Font
	for i := 0; i < 3; i++ {
		f := fixtureFont(t)
		alive = append(alive, f)
		r.recordParse(f, 1)
	}
	got := r.snapshot()
	if len(got.Fonts) != 2 || got.Evicted != 1 || got.Parses != 3 {
		t.Fatalf("unbounded inventory: %+v", got)
	}
	runtime.KeepAlive(alive)
}
