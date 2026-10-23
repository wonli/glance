package wordfont

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"

	"github.com/go-text/typesetting/fontscan"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"glance/internal/document"
	"glance/internal/fontdiag"
	"glance/internal/fontmanager/fontcatalog"
	"glance/internal/fontmanager/fontfallback"
	"glance/internal/wordlayout"
)

func FallbackFamilies() []string {
	return fontfallback.CJKFamilies()
}

func SplitFamilies(value string) []string {
	return splitWordFontFamilies(value)
}

type Shaper struct {
	mu sync.Mutex

	snapshot    *fontcatalog.Snapshot
	resourceIDs map[fontscan.Location]fontcatalog.ResourceID
	cacheDir    string
	loaded      bool
	loadErr     error
	fonts       []fontscan.Footprint

	facesByID         map[fontscan.Location]*font.Face
	idsByFace         map[*font.Face]fontscan.Location
	paths             map[string]*list.Element
	pathLRU           *list.List
	pathBytes         int64
	fontsByFamily     map[string][]fontscan.Footprint
	isLastResort      map[fontscan.Location]bool
	orderedFontsCache map[orderedFontsKey][]fontscan.Footprint
	coverage          map[fontscan.Location]font.Cmap
}

type orderedFontsKey struct {
	family string
	bold   bool
	italic bool
}

func NewSystem() (*Shaper, error) {
	if snapshot := fontcatalog.Default(); snapshot.Version > 0 {
		return NewWithSnapshot(snapshot)
	}
	if cache, objects, e := fontcatalog.DefaultPaths(); e == nil {
		if cached, e := fontcatalog.LoadCache(cache); e == nil {
			return NewWithSnapshot(cached)
		}
		if entries, e := os.ReadDir(objects); e == nil && len(entries) > 0 {
			if dirs, e := fontcatalog.SystemDirectories(); e == nil {
				snapshot, e := fontcatalog.LoadResources(context.Background(), cache, objects, dirs)
				if e == nil {
					return NewWithSnapshot(snapshot)
				}
			}
		}
	}
	return newWordTextShaperWithCacheDir("")
}

func NewWithCacheDir(cacheDir string) (*Shaper, error) {
	return newWordTextShaperWithCacheDir(cacheDir)
}

var processWordFontCache struct {
	once sync.Once
	dir  string
	err  error
}

func newWordTextShaperWithCacheDir(cacheDir string) (*Shaper, error) {
	var err error
	if cacheDir == "" {
		cacheDir, err = defaultWordFontCacheDir()
	} else {
		cacheDir, err = writableWordFontCacheDir(cacheDir)
	}
	if err != nil {
		return nil, fmt.Errorf("prepare Word font cache: %w", err)
	}
	return &Shaper{
		cacheDir:  cacheDir,
		facesByID: make(map[fontscan.Location]*font.Face),
		idsByFace: make(map[*font.Face]fontscan.Location),
		paths:     make(map[string]*list.Element),
		pathLRU:   list.New(),
	}, nil
}

func (s *Shaper) Shape(value string, style document.WordTextStyle) (wordlayout.ShapedText, error) {
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

	fontMap := &wordTextFontMap{owner: s, candidates: s.orderedFonts(style), style: style}
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
	if fontMap.resourceErr != nil {
		return wordlayout.ShapedText{}, fontMap.resourceErr
	}
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
		shaped.LineGapPT = math.Max(shaped.LineGapPT, fixedToPoints(output.LineBounds.Gap))
		if ascent > shaped.AscentPT {
			shaped.AscentPT = ascent
		}
		if descent > shaped.DescentPT {
			shaped.DescentPT = descent
		}
		clusterGlyphCounts := map[int]int{}
		for _, glyph := range output.Glyphs {
			clusterGlyphCounts[glyph.TextIndex()]++
		}
		for _, glyph := range output.Glyphs {
			start, end := glyph.TextIndex(), glyph.TextIndex()+glyph.RunesCount()
			if start < 0 || end <= start || end > len(runes) {
				return wordlayout.ShapedText{}, fmt.Errorf("font shaper returned invalid rune cluster [%d,%d)", start, end)
			}
			advance := fixedToPoints(glyph.Advance)
			offset := fixedToPoints(glyph.XOffset)
			// Missing legacy bi-width fonts need compatible advances as well
			// as readable outlines. Installed originals always use their metrics.
			family := fontFamilyForRune(style, runes[start])
			if clusterGlyphCounts[start] == 1 && asciiCluster(runes[start:end]) && fontMap.missingBiWidthFamily(family) {
				width := 0.0
				for _, r := range runes[start:end] {
					if r >= 0x20 && r <= 0x7e {
						width += style.FontSize / 2
					} else {
						width += advance / float64(end-start)
					}
				}
				offset += (width - advance) / 2
				advance = width
			}
			shaped.Glyphs = append(shaped.Glyphs, wordlayout.ShapedGlyph{
				FontKey:    key,
				GlyphID:    uint32(glyph.GlyphID),
				RuneStart:  start,
				RuneEnd:    end,
				AdvanceXPT: advance,
				OffsetXPT:  offset,
				OffsetYPT:  fixedToPoints(glyph.YOffset),
			})
		}
	}
	if err := wordlayout.ValidateShapedText(value, shaped); err != nil {
		return wordlayout.ShapedText{}, fmt.Errorf("invalid shaped Word text: %w", err)
	}
	return shaped, nil
}

func (s *Shaper) readFontBytes(location fontscan.Location) ([]byte, error) {
	if s.snapshot != nil {
		return s.snapshot.ReadFace(s.resourceIDs[location])
	}
	return os.ReadFile(location.File)
}

func (s *Shaper) FontData(fontKey string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadSystemFonts(); err != nil {
		return nil, err
	}
	for location := range s.facesByID {
		if wordFontKey(location) == fontKey {
			return s.readFontBytes(location)
		}
	}
	for _, footprint := range s.fonts {
		if wordFontKey(footprint.Location) == fontKey {
			return s.readFontBytes(footprint.Location)
		}
	}
	return nil, fmt.Errorf("font key %q: %w", fontKey, wordlayout.ErrUnknownFont)
}

func (s *Shaper) IsItalicFont(fontKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadSystemFonts(); err != nil {
		return false
	}
	for _, footprint := range s.fonts {
		if wordFontKey(footprint.Location) == fontKey {
			return footprint.Aspect.Style == font.StyleItalic
		}
	}
	return false
}

func (s *Shaper) ResolveSpreadsheetFont(style document.CellStyle, text string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadSystemFonts(); err != nil {
		return "", err
	}
	wordStyle := document.WordTextStyle{
		FontFamily: style.FontFamily,
		Bold:       style.Bold,
		Italic:     style.Italic,
		FontSize:   style.FontSize,
	}
	if text != "" {
		fontMap := &wordTextFontMap{owner: s, candidates: s.orderedFonts(wordStyle)}
		for _, r := range text {
			face := fontMap.ResolveFace(r)
			if face != nil {
				if location, ok := s.idsByFace[face]; ok {
					return wordFontKey(location), nil
				}
			}
		}
	}
	candidates := s.orderedFonts(wordStyle)
	if len(candidates) > 0 {
		return wordFontKey(candidates[0].Location), nil
	}
	return "", wordlayout.ErrUnknownFont
}

func (s *Shaper) loadSystemFonts() error {
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

func (s *Shaper) buildFontIndex() {
	if s.fontsByFamily != nil {
		return
	}
	s.fontsByFamily = make(map[string][]fontscan.Footprint, len(s.fonts))
	s.isLastResort = make(map[fontscan.Location]bool)
	for _, footprint := range s.fonts {
		s.fontsByFamily[footprint.Family] = append(s.fontsByFamily[footprint.Family], footprint)
		lower := strings.ToLower(footprint.Location.File)
		if strings.Contains(lower, "lastresort") {
			s.isLastResort[footprint.Location] = true
		}
	}
}

func (s *Shaper) orderedFonts(style document.WordTextStyle) []fontscan.Footprint {
	key := orderedFontsKey{
		family: style.FontFamily,
		bold:   style.Bold,
		italic: style.Italic,
	}
	if s.orderedFontsCache == nil {
		s.orderedFontsCache = make(map[orderedFontsKey][]fontscan.Footprint)
	}
	if cached, ok := s.orderedFontsCache[key]; ok {
		return cached
	}
	s.buildFontIndex()

	families := wordFontFamilyCandidates(style.FontFamily)
	// Keep the renderer's fallback order aligned with the process-wide Gio
	// collection so Markdown, Word, spreadsheet, and presentation text resolve
	// the same family from one captured catalog.
	families = append(families, fontfallback.PreferredFamilies()...)
	seen := make(map[fontscan.Location]bool)
	ordered := make([]fontscan.Footprint, 0, len(s.fonts))
	for _, family := range families {
		name := font.NormalizeFamily(strings.Trim(family, "\\\"' "))
		if name == "" {
			continue
		}
		var matches []fontscan.Footprint
		for _, footprint := range s.fontsByFamily[name] {
			if !seen[footprint.Location] {
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
		leftLast := s.isLastResort[remaining[i].Location]
		rightLast := s.isLastResort[remaining[j].Location]
		if leftLast != rightLast {
			return !leftLast
		}
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
	if len(remaining) > 8 {
		remaining = remaining[:8]
	}
	// Keep only the candidates, not the full-catalog backing array. Bound
	// family/style variants so arbitrary document font names cannot grow this
	// optimization without limit.
	result := append([]fontscan.Footprint(nil), append(ordered, remaining...)...)
	if len(s.orderedFontsCache) >= 64 {
		clear(s.orderedFontsCache)
	}
	s.orderedFontsCache[key] = result
	return result
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
	resourceErr    error
	owner          *Shaper
	candidates     []fontscan.Footprint
	resolved       map[rune]*font.Face
	style          document.WordTextStyle
	byFamily       map[string][]fontscan.Footprint
	biWidthMissing map[string]bool
}

func (m *wordTextFontMap) ResolveFace(r rune) *font.Face {
	if m.resolved == nil {
		m.resolved = make(map[rune]*font.Face)
	}
	if face, ok := m.resolved[r]; ok {
		return face
	}
	candidates := m.candidates
	family := fontFamilyForRune(m.style, r)
	if family != "" && family != m.style.FontFamily {
		if m.byFamily == nil {
			m.byFamily = map[string][]fontscan.Footprint{}
		}
		var ok bool
		candidates, ok = m.byFamily[family]
		if !ok {
			style := m.style
			style.FontFamily = family
			candidates = m.owner.orderedFonts(style)
			m.byFamily[family] = candidates
		}
	}
	for _, candidate := range candidates {
		if candidate.Runes.Len() > 0 && !candidate.Runes.Contains(r) {
			continue
		}
		if candidate.Runes.Len() == 0 && m.owner.snapshot != nil {
			covered, err := m.owner.catalogFontContains(candidate.Location, r)
			if err != nil {
				m.resourceErr = fmt.Errorf("font resource unavailable or changed: %s; refresh fonts: %w", candidate.Location.File, err)
				return nil
			}
			if !covered {
				continue
			}
		}
		face := m.owner.loadFace(candidate.Location)
		if face == nil && m.owner.snapshot != nil {
			m.resourceErr = fmt.Errorf("font resource unavailable or changed: %s; refresh fonts", candidate.Location.File)
			return nil
		}
		if face != nil {
			if _, ok := face.NominalGlyph(r); !ok {
				continue
			}
			m.resolved[r] = face
			return face
		}
	}
	return nil
}

func (s *Shaper) loadFace(location fontscan.Location) *font.Face {
	if face := s.facesByID[location]; face != nil {
		return face
	}
	data, err := s.readFontBytes(location)
	if err != nil {
		return nil
	}
	ft, err := ParseFont(data, uint32(location.Index))
	if err != nil {
		return nil
	}

	fontdiag.RecordUse(ft, "word", location.File, uint32(location.Index))
	face := font.NewFace(ft)
	s.facesByID[location] = face
	s.idsByFace[face] = location
	return face
}

func wordFontKey(location fontscan.Location) string {
	return location.File + "#face=" + strconv.Itoa(int(location.Index)) + "#instance=" + strconv.Itoa(int(location.Instance))
}
