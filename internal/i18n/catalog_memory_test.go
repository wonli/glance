package i18n

import "testing"

var catalogTextSink string
var catalogSink *Catalog

func TestCatalogPlainTextLookupDoesNotAllocate(t *testing.T) {
	c, err := newCatalog(LocaleEnglish, map[string]any{"label": "Saved }} exactly"}, map[string]any{"label": "Saved }} exactly"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Text(Message{ID: "label"}); got != "Saved }} exactly" {
		t.Fatalf("literal changed: %q", got)
	}
	if got := testing.AllocsPerRun(100, func() { catalogTextSink = c.Text(Message{ID: "label"}) }); got != 0 {
		t.Fatalf("plain text lookup allocated %g objects", got)
	}
}

func BenchmarkEmbeddedCatalogConstruction(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var err error
		catalogSink, err = NewCatalog(LocaleChinese, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
