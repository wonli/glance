package desktopui

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/fontscan"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"glance/internal/document"
	"glance/internal/wordlayout"
)

var wordFontFallbackFamilies = []string{
	"Noto Sans CJK SC",
	"PingFang SC",
	"Hiragino Sans GB",
	"Microsoft YaHei",
	"WenQuanYi Micro Hei",
	"Noto Sans",
	"Arial",
	"Helvetica",
	"sans-serif",
}

type wordTextShaper struct {
	mu sync.Mutex

	cacheDir string
	loaded   bool
	loadErr  error
	fonts    []fontscan.Footprint

	facesByID map[fontscan.Location]*font.Face
	idsByFace map[*font.Face]fontscan.Location
	dataByID  map[fontscan.Location][]byte
	paths     map[string]wordlayout.GlyphOutline
}

func newWordTextShaper() (*wordTextShaper, error) {
	return newWordTextShaperWithCacheDir("")
}

var processWordFontCache struct {
	once sync.Once
	dir  string
	err  error
}

func defaultWordFontCacheDir() (string, error) {
	processWordFontCache.once.Do(func() {
		preferred := ""
		if userCache, err := os.UserCacheDir(); err == nil {
			preferred = filepath.Join(userCache, "glance", "fonts")
		}
		processWordFontCache.dir, processWordFontCache.err = writableWordFontCacheDir(preferred)
	})
	return processWordFontCache.dir, processWordFontCache.err
}

func prepareWordFontScanner() error {
	cacheDir, err := defaultWordFontCacheDir()
	if err != nil {
		return fmt.Errorf("prepare Word font cache: %w", err)
	}
	if _, err := fontscan.SystemFonts(nil, cacheDir); err != nil {
		return fmt.Errorf("initialize system font scanner: %w", err)
	}
	return nil
}

func newWordTextShaperWithCacheDir(cacheDir string) (*wordTextShaper, error) {
	var err error
	if cacheDir == "" {
		cacheDir, err = defaultWordFontCacheDir()
	} else {
		cacheDir, err = writableWordFontCacheDir(cacheDir)
	}
	if err != nil {
		return nil, fmt.Errorf("prepare Word font cache: %w", err)
	}
	return &wordTextShaper{
		cacheDir:  cacheDir,
		facesByID: make(map[fontscan.Location]*font.Face),
		idsByFace: make(map[*font.Face]fontscan.Location),
		dataByID:  make(map[fontscan.Location][]byte),
		paths:     make(map[string]wordlayout.GlyphOutline),
	}, nil
}

func writableWordFontCacheDir(preferred string) (string, error) {
	if preferred != "" {
		if err := ensureWordFontCacheWritable(preferred); err == nil {
			return preferred, nil
		}
	}
	return os.MkdirTemp("", "glance-word-fonts-")
}

func ensureWordFontCacheWritable(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	probe, err := os.CreateTemp(directory, ".write-probe-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	closeErr := probe.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func (s *wordTextShaper) Shape(value string, style document.WordTextStyle) (wordlayout.ShapedText, error) {
	if value == "" {
		return wordlayout.ShapedText{}, nil
	}
	if math.IsNaN(style.FontSize) || math.IsInf(style.FontSize, 0) || style.FontSize < 0 {
		return wordlayout.ShapedText{}, errors.New("Word font size must be a finite non-negative point value")
	}
	if style.FontSize == 0 {
		style.FontSize = 11
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadSystemFonts(); err != nil {
		return wordlayout.ShapedText{}, err
	}

	fontMap := &wordTextFontMap{owner: s, candidates: s.orderedFonts(style)}
	runes := []rune(value)
	input := shaping.Input{
		Text:      runes,
		RunStart:  0,
		RunEnd:    len(runes),
		Direction: di.DirectionLTR,
		Language:  language.NewLanguage("und"),
		Size:      fixed.Int26_6(math.Round(style.FontSize * 64)),
	}
	var segmenter shaping.Segmenter
	segments := segmenter.Split(input, fontMap)
	var harfbuzz shaping.HarfbuzzShaper
	shaped := wordlayout.ShapedText{}
	for _, segment := range segments {
		if segment.Face == nil {
			return wordlayout.ShapedText{}, errors.New("no system font is available to shape Word text")
		}
		output := harfbuzz.Shape(segment)
		location, ok := s.idsByFace[segment.Face]
		if !ok {
			return wordlayout.ShapedText{}, errors.New("selected system font has no registered font identity")
		}
		key := wordFontKey(location)
		ascent := fixedToPoints(output.LineBounds.Ascent)
		descent := math.Abs(fixedToPoints(output.LineBounds.Descent))
		if ascent > shaped.AscentPT {
			shaped.AscentPT = ascent
		}
		if descent > shaped.DescentPT {
			shaped.DescentPT = descent
		}
		for _, glyph := range output.Glyphs {
			start, end := glyph.TextIndex(), glyph.TextIndex()+glyph.RunesCount()
			if start < 0 || end <= start || end > len(runes) {
				return wordlayout.ShapedText{}, fmt.Errorf("font shaper returned invalid rune cluster [%d,%d)", start, end)
			}
			shaped.Glyphs = append(shaped.Glyphs, wordlayout.ShapedGlyph{
				FontKey:    key,
				GlyphID:    uint32(glyph.GlyphID),
				RuneStart:  start,
				RuneEnd:    end,
				AdvanceXPT: fixedToPoints(glyph.Advance),
				OffsetXPT:  fixedToPoints(glyph.XOffset),
				OffsetYPT:  fixedToPoints(glyph.YOffset),
			})
		}
	}
	if err := wordlayout.ValidateShapedText(value, shaped); err != nil {
		return wordlayout.ShapedText{}, fmt.Errorf("invalid shaped Word text: %w", err)
	}
	return shaped, nil
}

func (s *wordTextShaper) FontData(fontKey string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadSystemFonts(); err != nil {
		return nil, err
	}
	for location := range s.dataByID {
		if wordFontKey(location) == fontKey {
			return bytes.Clone(s.dataByID[location]), nil
		}
	}
	for _, footprint := range s.fonts {
		if wordFontKey(footprint.Location) == fontKey {
			data, err := os.ReadFile(footprint.Location.File)
			if err != nil {
				return nil, fmt.Errorf("read font source %q: %w", footprint.Location.File, err)
			}
			s.dataByID[footprint.Location] = data
			return bytes.Clone(data), nil
		}
	}
	return nil, fmt.Errorf("font key %q: %w", fontKey, wordlayout.ErrUnknownFont)
}

func (s *wordTextShaper) GlyphOutline(fontKey string, glyphID uint32) (wordlayout.GlyphOutline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadSystemFonts(); err != nil {
		return wordlayout.GlyphOutline{}, err
	}
	cacheKey := fontKey + "#glyph=" + strconv.FormatUint(uint64(glyphID), 10)
	if outline, ok := s.paths[cacheKey]; ok {
		return cloneGlyphOutline(outline), nil
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
			s.paths[cacheKey] = outline
			return cloneGlyphOutline(outline), nil
		}
		return wordlayout.GlyphOutline{}, fmt.Errorf("glyph %d in %q has no vector outline", glyphID, fontKey)
	default:
		return wordlayout.GlyphOutline{}, fmt.Errorf("glyph %d in %q uses an unsupported color or bitmap outline", glyphID, fontKey)
	}
	outline := wordlayout.GlyphOutline{UnitsPerEm: unitsPerEm, Commands: fontOutlineCommands(source.Segments)}
	s.paths[cacheKey] = outline
	return cloneGlyphOutline(outline), nil
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

func (s *wordTextShaper) loadSystemFonts() error {
	if s.loaded {
		return s.loadErr
	}
	s.loaded = true
	s.fonts, s.loadErr = fontscan.SystemFonts(nil, s.cacheDir)
	if s.loadErr == nil && len(s.fonts) == 0 {
		s.loadErr = errors.New("no system fonts were found")
	}
	return s.loadErr
}

func (s *wordTextShaper) orderedFonts(style document.WordTextStyle) []fontscan.Footprint {
	families := splitWordFontFamilies(style.FontFamily)
	families = append(families, wordFontFallbackFamilies...)
	seen := make(map[fontscan.Location]bool)
	ordered := make([]fontscan.Footprint, 0, len(s.fonts))
	for _, family := range families {
		name := font.NormalizeFamily(strings.Trim(family, "\\\"' "))
		if name == "" {
			continue
		}
		matches := make([]fontscan.Footprint, 0)
		for _, footprint := range s.fonts {
			if footprint.Family == name && !seen[footprint.Location] {
				matches = append(matches, footprint)
			}
		}
		sort.SliceStable(matches, func(i, j int) bool {
			return wordFontAspectScore(matches[i].Aspect, style) < wordFontAspectScore(matches[j].Aspect, style)
		})
		for _, match := range matches {
			seen[match.Location] = true
			ordered = append(ordered, match)
		}
	}
	remaining := make([]fontscan.Footprint, 0, len(s.fonts)-len(ordered))
	for _, footprint := range s.fonts {
		if !seen[footprint.Location] {
			remaining = append(remaining, footprint)
		}
	}
	sort.SliceStable(remaining, func(i, j int) bool {
		if remaining[i].Family != remaining[j].Family {
			return remaining[i].Family < remaining[j].Family
		}
		left, right := wordFontAspectScore(remaining[i].Aspect, style), wordFontAspectScore(remaining[j].Aspect, style)
		if left != right {
			return left < right
		}
		if remaining[i].Location.File != remaining[j].Location.File {
			return remaining[i].Location.File < remaining[j].Location.File
		}
		return remaining[i].Location.Index < remaining[j].Location.Index
	})
	return append(ordered, remaining...)
}

func splitWordFontFamilies(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	families := make([]string, 0, len(parts))
	for _, part := range parts {
		if family := strings.TrimSpace(strings.Trim(part, "\\\"' ")); family != "" {
			families = append(families, family)
		}
	}
	return families
}

func wordFontAspectScore(aspect font.Aspect, style document.WordTextStyle) float64 {
	score := 0.0
	if style.Italic != (aspect.Style == font.StyleItalic) {
		score += 1000
	}
	weight := float64(font.WeightNormal)
	if style.Bold {
		weight = float64(font.WeightBold)
	}
	delta := math.Abs(float64(aspect.Weight) - weight)
	if math.IsNaN(delta) || math.IsInf(delta, 0) {
		delta = 1000
	}
	return score + delta
}

func fixedToPoints(value fixed.Int26_6) float64 {
	return float64(value) / 64
}

type wordTextFontMap struct {
	owner      *wordTextShaper
	candidates []fontscan.Footprint
	resolved   map[rune]*font.Face
}

func (m *wordTextFontMap) ResolveFace(r rune) *font.Face {
	if m.resolved == nil {
		m.resolved = make(map[rune]*font.Face)
	}
	if face, ok := m.resolved[r]; ok {
		return face
	}
	for _, candidate := range m.candidates {
		if !candidate.Runes.Contains(r) {
			continue
		}
		face := m.owner.loadFace(candidate.Location)
		if face != nil {
			m.resolved[r] = face
			return face
		}
	}
	return nil
}

func (s *wordTextShaper) loadFace(location fontscan.Location) *font.Face {
	if face := s.facesByID[location]; face != nil {
		return face
	}
	data, err := os.ReadFile(location.File)
	if err != nil {
		return nil
	}
	faces, err := font.ParseTTC(bytes.NewReader(data))
	if err != nil || int(location.Index) >= len(faces) {
		return nil
	}
	face := faces[location.Index]
	s.facesByID[location] = face
	s.idsByFace[face] = location
	s.dataByID[location] = data
	return face
}

func wordFontKey(location fontscan.Location) string {
	return location.File + "#face=" + strconv.Itoa(int(location.Index)) + "#instance=" + strconv.Itoa(int(location.Instance))
}
