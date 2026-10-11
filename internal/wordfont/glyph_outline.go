package wordfont

import (
	"container/list"
	"fmt"
	"strconv"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"

	"glance/internal/wordlayout"
)

const (
	wordGlyphOutlineCacheLimit = int64(16 << 20)
	wordGlyphOutlineCacheItems = 128
)

type cachedWordGlyphOutline struct {
	key     string
	outline wordlayout.GlyphOutline
	weight  int64
}

func (s *Shaper) GlyphOutline(fontKey string, glyphID uint32) (wordlayout.GlyphOutline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadSystemFonts(); err != nil {
		return wordlayout.GlyphOutline{}, err
	}
	cacheKey := fontKey + "#glyph=" + strconv.FormatUint(uint64(glyphID), 10)
	if outline, ok := s.cachedGlyphOutline(cacheKey); ok {
		return outline, nil
	}
	var face *font.Face
	for location, candidate := range s.facesByID {
		if wordFontKey(location) == fontKey {
			face = candidate
			break
		}
	}
	if face == nil {
		for _, footprint := range s.fonts {
			if wordFontKey(footprint.Location) == fontKey {
				face = s.loadFace(footprint.Location)
				break
			}
		}
	}
	if face == nil {
		return wordlayout.GlyphOutline{}, fmt.Errorf("font key %q: %w", fontKey, wordlayout.ErrUnknownFont)
	}
	unitsPerEm := float64(face.Upem())
	data := face.GlyphData(font.GID(glyphID))
	var source font.GlyphOutline
	switch value := data.(type) {
	case font.GlyphOutline:
		source = value
	case font.GlyphSVG:
		source = value.Outline
	case nil:
		if extents, ok := face.GlyphExtents(font.GID(glyphID)); ok && extents.Width == 0 && extents.Height == 0 {
			outline := wordlayout.GlyphOutline{UnitsPerEm: unitsPerEm}
			s.cacheGlyphOutline(cacheKey, outline)
			return cloneGlyphOutline(outline), nil
		}
		return wordlayout.GlyphOutline{}, fmt.Errorf("glyph %d in %q has no vector outline", glyphID, fontKey)
	default:
		return wordlayout.GlyphOutline{}, fmt.Errorf("glyph %d in %q uses an unsupported color or bitmap outline", glyphID, fontKey)
	}
	outline := wordlayout.GlyphOutline{UnitsPerEm: unitsPerEm, Commands: fontOutlineCommands(source.Segments)}
	s.cacheGlyphOutline(cacheKey, outline)
	return cloneGlyphOutline(outline), nil
}

func (s *Shaper) cachedGlyphOutline(cacheKey string) (wordlayout.GlyphOutline, bool) {
	if s == nil {
		return wordlayout.GlyphOutline{}, false
	}
	item := s.paths[cacheKey]
	if item == nil {
		return wordlayout.GlyphOutline{}, false
	}
	s.pathLRU.MoveToFront(item)
	entry := item.Value.(cachedWordGlyphOutline)
	return cloneGlyphOutline(entry.outline), true
}

func (s *Shaper) cacheGlyphOutline(cacheKey string, outline wordlayout.GlyphOutline) {
	if s == nil {
		return
	}
	if s.paths == nil {
		s.paths = make(map[string]*list.Element)
	}
	if s.pathLRU == nil {
		s.pathLRU = list.New()
	}
	if current := s.paths[cacheKey]; current != nil {
		entry := current.Value.(cachedWordGlyphOutline)
		s.pathBytes -= entry.weight
		delete(s.paths, cacheKey)
		s.pathLRU.Remove(current)
	}

	weight := int64(len(cacheKey)) + int64(len(outline.Commands))*64
	if weight > wordGlyphOutlineCacheLimit {
		return
	}
	for len(s.paths) >= wordGlyphOutlineCacheItems || s.pathBytes+weight > wordGlyphOutlineCacheLimit {
		oldest := s.pathLRU.Back()
		if oldest == nil {
			break
		}
		entry := oldest.Value.(cachedWordGlyphOutline)
		delete(s.paths, entry.key)
		s.pathBytes -= entry.weight
		s.pathLRU.Remove(oldest)
	}
	entry := cachedWordGlyphOutline{key: cacheKey, outline: cloneGlyphOutline(outline), weight: weight}
	s.paths[cacheKey] = s.pathLRU.PushFront(entry)
	s.pathBytes += weight
}

func fontOutlineCommands(segments []font.Segment) []wordlayout.GlyphOutlineCommand {
	commands := make([]wordlayout.GlyphOutlineCommand, 0, len(segments))
	var current wordlayout.GlyphOutlinePoint
	point := func(value font.SegmentPoint) wordlayout.GlyphOutlinePoint {
		return wordlayout.GlyphOutlinePoint{X: float64(value.X), Y: float64(value.Y)}
	}
	for _, segment := range segments {
		switch segment.Op {
		case ot.SegmentOpMoveTo:
			current = point(segment.Args[0])
			commands = append(commands, wordlayout.GlyphOutlineCommand{Op: wordlayout.GlyphPathMoveTo, Points: [3]wordlayout.GlyphOutlinePoint{current}})
		case ot.SegmentOpLineTo:
			current = point(segment.Args[0])
			commands = append(commands, wordlayout.GlyphOutlineCommand{Op: wordlayout.GlyphPathLineTo, Points: [3]wordlayout.GlyphOutlinePoint{current}})
		case ot.SegmentOpQuadTo:
			control, end := point(segment.Args[0]), point(segment.Args[1])
			c1 := wordlayout.GlyphOutlinePoint{X: current.X + 2.0/3.0*(control.X-current.X), Y: current.Y + 2.0/3.0*(control.Y-current.Y)}
			c2 := wordlayout.GlyphOutlinePoint{X: end.X + 2.0/3.0*(control.X-end.X), Y: end.Y + 2.0/3.0*(control.Y-end.Y)}
			commands = append(commands, wordlayout.GlyphOutlineCommand{Op: wordlayout.GlyphPathCurveTo, Points: [3]wordlayout.GlyphOutlinePoint{c1, c2, end}})
			current = end
		case ot.SegmentOpCubeTo:
			points := [3]wordlayout.GlyphOutlinePoint{point(segment.Args[0]), point(segment.Args[1]), point(segment.Args[2])}
			commands = append(commands, wordlayout.GlyphOutlineCommand{Op: wordlayout.GlyphPathCurveTo, Points: points})
			current = points[2]
		}
	}
	return commands
}

func cloneGlyphOutline(outline wordlayout.GlyphOutline) wordlayout.GlyphOutline {
	outline.Commands = append([]wordlayout.GlyphOutlineCommand(nil), outline.Commands...)
	return outline
}
