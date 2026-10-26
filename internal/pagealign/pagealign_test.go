package pagealign

import (
	"strings"
	"testing"
)

func TestCleanOCRText(t *testing.T) {
	raw := `李端，生卒年不详，赵州人。大历五年登进士第， $ \underline{\text{授秘书省校书郎}} $。以病辞官，居终南山草堂寺。 $ \underline{\text{出为杭州司马}} $。卒于建中、贞元之际。`
	got := CleanOCRText(raw)
	wantContains := []string{
		"授秘书省校书郎",
		"出为杭州司马",
		"李端，生卒年不详",
	}
	for _, w := range wantContains {
		if !strings.Contains(got, w) {
			t.Errorf("CleanOCRText() missing %q, got: %q", w, got)
		}
	}
	dontWant := []string{
		`\underline`,
		`\text`,
		`$`,
	}
	for _, dw := range dontWant {
		if strings.Contains(got, dw) {
			t.Errorf("CleanOCRText() should not contain %q, got: %q", dw, got)
		}
	}
}

func TestFindBoundaryExact(t *testing.T) {
	p1 := "这是第一页的完整正文内容。包含了关于某位历史人物的详细生平与典籍考证。"
	p2 := "这是第二页的起始正文，承接上一页的讨论，继续展开深入的评述与文献引证。"
	doc := p1 + "\n" + p2 + "\n后续的大段文字"

	res, ok := FindBoundary(PageInput{Text: p1}, PageInput{Text: p2}, doc)
	if !ok {
		t.Fatalf("expected boundary found, got ok=false")
	}
	wantCut := len([]rune(p1)) + 1 // +1 for the newline '\n'
	if res.CutIndex != wantCut {
		t.Fatalf("CutIndex = %d, want %d", res.CutIndex, wantCut)
	}
	if res.Confidence < 0.9 {
		t.Fatalf("expected high confidence, got %v", res.Confidence)
	}
}

func TestFindBoundaryWithLaTeXAndNoise(t *testing.T) {
	p1Clean := "大历五年登进士第，授秘书省校书郎。以病辞官，居终南山草堂寺。与钱起、卢纶、吉中孚、韩翃、司空曙、苗发、崔峒、耿湋、夏侯审齐名，号为大历十才子。早年多宦游江南，晚年隐居衡山以卒。李端工诗，清润婉丽，善叙羁旅离情。其《赠郭暧家妓》云：“鸣筝金粟柱，素手玉房前。欲得周郎顾，时时误拂弦。”尤为脍炙人口。"
	// Notice: p1OCR has LaTeX markup AND an arbitrary footer "/ 增订本 四", and no regex was written for it.
	// The algorithm finds the boundary purely via next page anchors and local refinement!
	p1OCR := "大历五年登进士第， $ \\underline{\\text{授秘书省校书郎}} $。以病辞官，居终南山草堂寺。与钱起、卢纶、吉中孚、韩翃、司空曙、苗发、崔峒、耿湋、夏侯审齐名，号为 $ \\underline{\\text{大历十才子}} $。早年多宦游江南，晚年隐居衡山以卒。李端工诗，清润婉丽，善叙羁旅离情。其《赠郭暧家妓》云：“鸣筝金粟柱，素手玉房前。欲得周郎顾，时时误拂弦。”尤为脍炙人口。\n/ 增订本 四"
	p2Text := "开元中，卢象以文词显，天宝中，元结以雅论著，至贞元中，韦应物以清真道古之辞擅名一时。李端与其颉颃相抗，风骨独存。"
	doc := p1Clean + "\n" + p2Text + "\n" + strings.Repeat("后续大段文字", 100)

	res, ok := FindBoundary(PageInput{Text: p1OCR}, PageInput{Text: p2Text}, doc)
	if !ok {
		t.Fatalf("expected boundary with LaTeX noise, got ok=false")
	}
	wantCut := len([]rune(p1Clean)) + 1
	if res.CutIndex != wantCut {
		t.Fatalf("CutIndex = %d, want %d", res.CutIndex, wantCut)
	}
}

func TestFindBoundaryWithNextPageHeader(t *testing.T) {
	p1 := "这是第一页的文字，描述诗人的主要成就。"
	// Next page has a running header that does NOT exist in the manuscript.
	p2OCR := "任意书名 第三卷 页码 148\n这是第二页的正文开始，继续详细分析作品风格。"
	p2Manuscript := "这是第二页的正文开始，继续详细分析作品风格。"
	doc := p1 + "\n" + p2Manuscript + "\n" + strings.Repeat("后续文字", 50)

	res, ok := FindBoundary(PageInput{Text: p1}, PageInput{Text: p2OCR}, doc)
	if !ok {
		t.Fatalf("expected boundary with next page header noise, got ok=false")
	}
	wantCut := len([]rune(p1)) + 1
	if res.CutIndex != wantCut {
		t.Fatalf("CutIndex = %d, want %d", res.CutIndex, wantCut)
	}
}

func TestFindBoundaryRejectsAmbiguousTies(t *testing.T) {
	p1 := "第一页文字包含若干讨论"
	p2 := "甲第二页正文开始"
	// Manuscript has "甲甲", but OCR only has one "甲". Neither page can claim the extra character.
	doc := p1 + "甲甲" + p2[len("甲"):]
	_, ok := FindBoundary(PageInput{Text: p1}, PageInput{Text: p2}, doc)
	if ok {
		t.Fatalf("expected ambiguous boundary to be rejected, got ok=true")
	}
}

func BenchmarkFindBoundary(b *testing.B) {
	p1 := strings.Repeat("李端工诗清润婉丽善叙羁旅离情鸣筝金粟柱素手玉房前欲得周郎顾时时误拂弦", 10)
	p2 := strings.Repeat("开元中卢象以文词显天宝中元结以雅论著至贞元中韦应物以清真道古之辞擅名一时", 10)
	doc := p1 + "\n" + p2 + "\n" + strings.Repeat("后续文字", 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, ok := FindBoundary(PageInput{Text: p1}, PageInput{Text: p2}, doc)
		if !ok {
			b.Fatal("benchmark failed")
		}
	}
}
