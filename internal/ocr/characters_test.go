package ocr

import (
	"context"
	"image"
	"testing"
)

func testDetectedCharacters(text string) []detectedCharacter {
	var result []detectedCharacter
	for i, r := range []rune(text) {
		x := float64(i) / 20
		result = append(result, detectedCharacter{Text: string(r), Bounds: [4]float64{x, .1, x + .04, .2}})
	}
	return result
}
func TestCharacterGeometryAlignmentPreservesRawOffsets(t *testing.T) {
	got := alignCharacterGeometry("甲 乙\n丙丁", testDetectedCharacters("甲乙丙丁"))
	if len(got) != 4 {
		t.Fatalf("mapped=%v", got)
	}
	for i, want := range []int{0, 2, 4, 5} {
		if got[i].offset != want {
			t.Fatalf("mapped offset=%d want %d", got[i].offset, want)
		}
	}
	wrong := testDetectedCharacters("甲己丙丁")
	if got := alignCharacterGeometry("甲乙丙丁", wrong); len(got) != 4 {
		t.Fatalf("isolated OCR substitution lost measured geometry: %v", got)
	}
	if got := alignCharacterGeometry("甲乙丙丁", testDetectedCharacters("天地玄黄")); len(got) != 0 {
		t.Fatalf("unrelated recognition was mapped: %v", got)
	}
}
func TestCharacterGeometryRejectsRepeatedWordBoxes(t *testing.T) {
	observed := testDetectedCharacters("甲乙丙丁")
	observed[1].Bounds = observed[0].Bounds
	got := alignCharacterGeometry("甲乙丙丁", observed)
	if len(got) != 2 {
		t.Fatalf("word-sized duplicate boxes must not be used as characters: %v", got)
	}
}
func TestCharacterGeometryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := LocalizeCharacters(ctx, image.NewRGBA(image.Rect(0, 0, 100, 100)), Layout{Width: 100, Height: 100, Blocks: []LayoutBlock{{Text: "甲乙", Bounds: [4]float64{0, 0, 100, 100}}}})
	if err != context.Canceled {
		t.Fatalf("cancellation=%v", err)
	}
}

func TestCharacterGeometryRejectsSubstitutionAcrossNativeInsertion(t *testing.T) {
	got := alignCharacterGeometry("甲乙丙丁戊", testDetectedCharacters("甲天玄丙丁戊"))
	for _, pair := range got {
		if pair.offset == 1 {
			t.Fatal("an extra native glyph made the source substitution ambiguous")
		}
	}
}

func TestCharacterGeometryPartialFailureIsRetriable(t *testing.T) {
	calls := 0
	layout := Layout{Width: 200, Height: 100, Blocks: []LayoutBlock{
		{ID: 1, Text: "甲乙丙丁", Bounds: [4]float64{0, 0, 100, 50}},
		{ID: 2, Text: "天地玄黄", Bounds: [4]float64{100, 50, 200, 100}},
	}}
	result, err := localizeCharacters(context.Background(), image.NewRGBA(image.Rect(0, 0, 200, 100)), layout, func([]byte) ([]detectedTextLine, error) {
		calls++
		if calls == 2 {
			return nil, ErrCharacterLocalizationUnavailable
		}
		return []detectedTextLine{{Characters: testDetectedCharacters("甲乙丙丁")}}, nil
	})
	if err == nil || result.CharacterVersion != 0 || len(result.Characters) != 4 {
		t.Fatalf("partial failure lost usable geometry or suppressed retry: version=%d count=%d error=%v", result.CharacterVersion, len(result.Characters), err)
	}
	empty, err := localizeCharacters(context.Background(), image.NewRGBA(image.Rect(0, 0, 200, 100)), layout, func([]byte) ([]detectedTextLine, error) { return nil, nil })
	if err == nil || empty.CharacterVersion != 0 {
		t.Fatalf("zero character boxes were cached as successful: %+v error=%v", empty, err)
	}
}

func TestCharacterGeometryKeepsMeasuredAdjacentReplacements(t *testing.T) {
	observed := testDetectedCharacters("山头曈曈日将出")
	pairs := alignCharacterGeometry("山头昈昈日将出", observed)
	if len(pairs) != 7 {
		t.Fatalf("two adjacent OCR substitutions lost measured glyph boxes: %v", pairs)
	}
	for i, pair := range pairs {
		if pair.offset != i || pair.observed != i {
			t.Fatalf("replacement changed character ownership: %+v", pair)
		}
	}
}

func TestCharacterGeometryRejectsUnanchoredReplacementRuns(t *testing.T) {
	for _, item := range []struct{ target, observed string }{
		{"昈昈日将出天地玄黄", "曈曈日将出天地玄黄"},
		{"山头昈昈", "山头曈曈"},
		{"山头甲乙丙丁日将出天地玄黄", "山头戊己庚辛日将出天地玄黄"},
		{"山头昈昈日将出天地玄黄", "山头曈天曈日将出天地玄黄"},
	} {
		pairs := alignCharacterGeometry(item.target, testDetectedCharacters(item.observed))
		for _, p := range pairs {
			if !p.equal {
				t.Fatalf("ambiguous replacement run received geometry: target=%q pairs=%v", item.target, pairs)
			}
		}
	}
	observed := testDetectedCharacters("山头曈曈日将出天地玄黄")
	observed[3].Bounds = [4]float64{.1, .6, .14, .7}
	for _, p := range alignCharacterGeometry("山头昈昈日将出天地玄黄", observed) {
		if !p.equal {
			t.Fatal("replacement run crossed observed lines")
		}
	}
}

func TestCharacterGeometryKeepsAnchoredTitleEdgeReplacement(t *testing.T) {
	for _, item := range []struct{ target, observed string }{{"贈李果毅", "赠李果毅"}, {"天地玄昈", "天地玄曈"}} {
		pairs := alignCharacterGeometry(item.target, testDetectedCharacters(item.observed))
		if len(pairs) != 4 {
			t.Fatalf("title edge lost its measured glyph: target=%q pairs=%v", item.target, pairs)
		}
		for i, p := range pairs {
			if p.offset != i || p.observed != i {
				t.Fatal("title geometry shifted")
			}
		}
	}
}

func TestCharacterGeometryRejectsAmbiguousTitleEdge(t *testing.T) {
	for _, item := range []struct{ target, observed string }{{"贈李果", "赠李果"}, {"贈李果毅", "赠天李果毅"}, {"贈李果毅", "李果毅"}} {
		for _, p := range alignCharacterGeometry(item.target, testDetectedCharacters(item.observed)) {
			if !p.equal {
				t.Fatalf("ambiguous title edge mapped: %q %+v", item.target, p)
			}
		}
	}
}

func TestCharacterGeometryMatchesFullWidthParenthesesAcrossLines(t *testing.T) {
	observed := testDetectedCharacters("甲（乙）丙")
	observed[4].Bounds = [4]float64{.02, .4, .06, .5}
	pairs := alignCharacterGeometry("甲(乙)丙", observed)
	found := map[int]bool{}
	for _, p := range pairs {
		found[p.offset] = true
	}
	if !found[1] || !found[3] {
		t.Fatalf("full-width parentheses with measured boxes lost across a line boundary: %v", pairs)
	}
}
