package wordfont

import (
	"fmt"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/fontscan"
)

// Catalog footprints have no RuneSet. Probe their small cmap before NewFont
// parses and retains every glyph/table for a font that cannot render the rune.
// The shaper mutex protects this cache together with the loaded faces.
func (s *Shaper) catalogFontContains(location fontscan.Location, r rune) (bool, error) {
	cmap := s.coverage[location]
	if cmap == nil {
		resource, err := s.snapshot.OpenFaceResource(s.resourceIDs[location])
		if err != nil {
			return false, err
		}
		defer resource.Close()
		loaders, err := opentype.NewLoaders(resource)
		if err != nil {
			return false, err
		}
		if int(location.Index) >= len(loaders) {
			return false, fmt.Errorf("invalid font face index %d", location.Index)
		}
		loader := loaders[location.Index]
		raw, _ := loader.RawTable(opentype.MustNewTag("OS/2"))
		os2, _, _ := tables.ParseOs2(raw)
		raw, err = loader.RawTable(opentype.MustNewTag("cmap"))
		if err != nil {
			return false, err
		}
		table, _, err := tables.ParseCmap(raw)
		if err != nil {
			return false, err
		}
		cmap, _, err = font.ProcessCmap(table, os2.FontPage())
		if err != nil {
			return false, err
		}
		if s.coverage == nil {
			s.coverage = make(map[fontscan.Location]font.Cmap)
		}
		s.coverage[location] = cmap
	}
	_, ok := cmap.Lookup(r)
	return ok, nil
}
