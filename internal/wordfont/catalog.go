package wordfont

import (
	"container/list"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/fontscan"
	"glance/internal/fontcatalog"
)

func NewWithSnapshot(snapshot fontcatalog.Snapshot) (*Shaper, error) {
	s := &Shaper{facesByID: make(map[fontscan.Location]*font.Face), idsByFace: make(map[*font.Face]fontscan.Location), dataByID: make(map[fontscan.Location][]byte), paths: make(map[string]*list.Element), pathLRU: list.New()}
	s.snapshot = &snapshot
	s.loaded = true
	s.resourceIDs = make(map[fontscan.Location]fontcatalog.ResourceID)
	for _, f := range snapshot.Faces() {
		if f.Unavailable != "" {
			continue
		}
		location := fontscan.Location{File: f.Path, Index: uint16(f.Index)}
		s.resourceIDs[location] = f.ID
		aspect := font.Aspect{Weight: font.Weight(f.Weight)}
		if f.Italic {
			aspect.Style = font.StyleItalic
		}
		names := append([]string{f.Family}, f.Aliases...)
		seen := map[string]bool{}
		for _, name := range names {
			name = font.NormalizeFamily(name)
			if seen[name] {
				continue
			}
			seen[name] = true
			s.fonts = append(s.fonts, fontscan.Footprint{Location: location, Family: name, Aspect: aspect})
		}
	}
	return s, nil
}
