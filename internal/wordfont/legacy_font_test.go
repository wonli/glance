package wordfont

import (
	"github.com/go-text/typesetting/font"
	"glance/internal/document"
	"math"
	"testing"
)

func TestMissingSimHeiPreservesHalfWidthSpaceAlignment(t *testing.T) {
	s := missingSimHeiShaper(t)
	// The three form labels are padded with ordinary spaces in a bi-width font.
	var widths []float64
	for _, text := range []string{"纳税人（姓名/名称）：            ", "法定代表人姓名：                ", "地址：                          "} {
		out, err := s.Shape(text, document.WordTextStyle{FontFamily: "黑体", FontSize: 12})
		if err != nil {
			t.Fatal(err)
		}
		width := 0.
		for _, g := range out.Glyphs {
			width += g.AdvanceXPT
		}
		widths = append(widths, width)
	}
	if math.Abs(widths[1]-widths[2]) > .1 || math.Abs(widths[0]-widths[1]) > 6.1 {
		t.Fatalf("right-label offsets=%v; want the two lower labels aligned and only source's half-character offset on first", widths)
	}
}

func TestWordShaperUsesSeparateLatinAndEastAsianFonts(t *testing.T) {
	s, err := NewWithCacheDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	latin, err := s.Shape("A ", document.WordTextStyle{FontFamily: "Times New Roman", FontSize: 12})
	if err != nil {
		t.Fatal(err)
	}
	cjk, err := s.Shape("中", document.WordTextStyle{FontFamily: "Hiragino Sans GB", FontSize: 12})
	if err != nil {
		t.Fatal(err)
	}
	mixed, err := s.Shape("A 中", document.WordTextStyle{FontFamily: "Hiragino Sans GB", FontFamilyASCII: "Times New Roman", FontFamilyEastAsia: "Hiragino Sans GB", FontSize: 12})
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range mixed.Glyphs {
		want := latin.Glyphs[0].FontKey
		if g.RuneStart == 2 {
			want = cjk.Glyphs[0].FontKey
		}
		if g.FontKey != want {
			t.Fatalf("rune %d font=%s want=%s", g.RuneStart, g.FontKey, want)
		}
	}
}

func TestMissingSimHeiCombiningClusterDoesNotRepeatAdvance(t *testing.T) {
	s := missingSimHeiShaper(t)
	got, err := s.Shape("x\u0338", document.WordTextStyle{FontFamily: "黑体", FontSize: 12})
	if err != nil {
		t.Fatal(err)
	}
	natural, err := s.Shape("x\u0338", document.WordTextStyle{FontFamily: "Heiti SC", FontSize: 12})
	if err != nil {
		t.Fatal(err)
	}
	a, b := 0., 0.
	for _, g := range got.Glyphs {
		a += g.AdvanceXPT
	}
	for _, g := range natural.Glyphs {
		b += g.AdvanceXPT
	}
	if math.Abs(a-b) > .01 {
		t.Fatalf("mark cluster advance=%.3f; natural=%.3f", a, b)
	}
}

func TestScriptFontSelectionRoutesExplicitComplexFonts(t *testing.T) {
	style := document.WordTextStyle{FontFamily: "Default", FontFamilyASCII: "Latin", FontFamilyHighANSI: "Other", FontFamilyEastAsia: "CJK", FontFamilyComplexScript: "Complex"}
	for _, r := range []rune{'ع', 'א', 'ก', 'क', 'ব', 'க', 'ក'} {
		if f := fontFamilyForRune(style, r); f != "Complex" {
			t.Errorf("%U family=%s", r, f)
		}
	}
	for r, want := range map[rune]string{'A': "Latin", ' ': "Latin", 'é': "Other", '中': "CJK", '，': "CJK"} {
		if f := fontFamilyForRune(style, r); f != want {
			t.Errorf("%U family=%s want=%s", r, f, want)
		}
	}
}

func missingSimHeiShaper(t *testing.T) *Shaper {
	t.Helper()
	s, err := NewWithCacheDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.loadSystemFonts(); err != nil {
		t.Fatal(err)
	}
	available := s.fonts[:0]
	for _, f := range s.fonts {
		if f.Family != "simhei" && f.Family != "黑体" {
			available = append(available, f)
		}
	}
	s.fonts = available
	return s
}

func TestInstalledSimHeiUsesOriginalFontAdvances(t *testing.T) {
	s := missingSimHeiShaper(t)
	style := document.WordTextStyle{FontFamily: "Heiti SC", FontSize: 12}
	natural, err := s.Shape("A /", style)
	if err != nil {
		t.Fatal(err)
	}
	// Register the very same available test face under the source family name
	// to exercise the installed-font branch without a proprietary font fixture.
	locationFound := false
	for i, f := range s.fonts {
		if wordFontKey(f.Location) == natural.Glyphs[0].FontKey {
			s.fonts[i].Family = font.NormalizeFamily("SimHei")
			locationFound = true
			break
		}
	}
	if !locationFound {
		t.Fatal("test face not registered")
	}
	style.FontFamily = "黑体"
	original, err := s.Shape("A /", style)
	if err != nil {
		t.Fatal(err)
	}
	if len(original.Glyphs) != len(natural.Glyphs) {
		t.Fatal("installed font changed shaping")
	}
	for i, g := range original.Glyphs {
		if g.FontKey != natural.Glyphs[i].FontKey || g.AdvanceXPT != natural.Glyphs[i].AdvanceXPT || g.OffsetXPT != natural.Glyphs[i].OffsetXPT {
			t.Fatalf("installed original metrics were modified: glyph=%+v natural=%+v", g, natural.Glyphs[i])
		}
	}
}

func TestMissingEastAsianFamilyDoesNotReuseLatinOnlyFamily(t *testing.T) {
	style := document.WordTextStyle{FontFamily: "仿宋", FontFamilyASCII: "仿宋", FontFamilyHighANSI: "仿宋", FontFamilyComplexScript: "仿宋"}
	if got := fontFamilyForRune(style, '中'); got != FallbackFamilies()[0] {
		t.Fatalf("unspecified East Asian family=%q; want platform CJK fallback %q", got, FallbackFamilies()[0])
	}
	if got := fontFamilyForRune(style, 'A'); got != "仿宋" {
		t.Fatalf("Latin font changed to %q", got)
	}
	style.FontFamilyEastAsia = "宋体"
	if got := fontFamilyForRune(style, '中'); got != "宋体" {
		t.Fatalf("explicit East Asian font changed to %q", got)
	}
}
