package desktopui

import (
	"math/rand"
	"strings"
	"testing"
)

func TestProofreadingOCRPrefixBandMatchesFullEditDistances(t *testing.T) {
	rng := rand.New(rand.NewSource(66))
	for sample := 0; sample < 250; sample++ {
		a, b := make([]rune, 1+rng.Intn(20)), make([]rune, 1+rng.Intn(25))
		for i := range a {
			a[i] = rune('甲' + rng.Intn(4))
		}
		for i := range b {
			b[i] = rune('甲' + rng.Intn(4))
		}
		budget := rng.Intn(7)
		rows := make([][]int, len(a)+1)
		for i := range rows {
			rows[i] = make([]int, len(b)+1)
			rows[i][0] = i
		}
		for j := range rows[0] {
			rows[0][j] = j
		}
		for i := 1; i <= len(a); i++ {
			for j := 1; j <= len(b); j++ {
				cost := 0
				if a[i-1] != b[j-1] {
					cost = 1
				}
				rows[i][j] = min(rows[i-1][j-1]+cost, rows[i-1][j]+1, rows[i][j-1]+1)
			}
		}
		got := proofreadingOCRPrefixEndDistances(a, b, budget)
		for end := max(1, len(a)-budget); end <= min(len(b), len(a)+budget); end++ {
			want := min(budget+1, rows[len(a)][end])
			actual := budget + 1
			if got != nil {
				actual = got[end]
			}
			if actual != want {
				t.Fatalf("sample=%d end=%d budget=%d got=%d want=%d", sample, end, budget, actual, want)
			}
		}
	}
}

func TestProofreadingOCRBoundaryExpandsContextAndRejectsAmbiguousCut(t *testing.T) {
	currentText := "本页有很多不重复的正文作为可靠依据，页首和页尾都允许少量识别差异，而不再依赖固定八字的完全相同。"
	nextText := "接下来这页也有不少内容，前三个字识别错误会使短窗口超过错误比例，但后文提供充分证据可以在扩大窗口之后找回正确边界，所有原始文字必须保留。"
	r := []rune(nextText)
	r[0], r[1], r[2] = '错', '误', '字'
	current, err := proofreadingOCRCacheTestPage(1, "误"+string([]rune(currentText)[1:])).Layout()
	if err != nil {
		t.Fatal(err)
	}
	next, err := proofreadingOCRCacheTestPage(2, string(r)).Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := currentText + "\n" + nextText + strings.Repeat("后续文字", 12000)
	tail, ok := proofreadingOCRBoundaryTailFromPages(current, next, text)
	if !ok || tail.Start != len([]rune(currentText))+1 {
		t.Fatalf("context did not expand: %+v %v", tail, ok)
	}
	// OCR omits a boundary character repeated at the next page's start. Both
	// ownership choices have the same joint edit cost; neither may be guessed.
	current, err = proofreadingOCRCacheTestPage(1, currentText).Layout()
	if err != nil {
		t.Fatal(err)
	}
	next, err = proofreadingOCRCacheTestPage(2, "甲"+nextText).Layout()
	if err != nil {
		t.Fatal(err)
	}
	if tail, ok := proofreadingOCRBoundaryTailFromPages(current, next, currentText+"甲甲"+nextText); ok {
		t.Fatalf("ambiguous character ownership guessed: %+v", tail)
	}
}

func TestProofreadingOCRBoundaryRejectsWeakMarginWithTailTypoAndHeadDeletion(t *testing.T) {
	p, s := make([]rune, 60), make([]rune, 100)
	for i := range p {
		p[i] = rune(0x4e00 + i)
	}
	for i := range s {
		s[i] = rune(0x5000 + i)
	}
	current, err := proofreadingOCRCacheTestPage(1, string(p)+"乙").Layout()
	if err != nil {
		t.Fatal(err)
	}
	next, err := proofreadingOCRCacheTestPage(2, string(s)).Layout()
	if err != nil {
		t.Fatal(err)
	}
	// Actual P甲 | 乙S; OCR reports P乙 | S. Merely taking the unique lowest
	// edit cost would incorrectly move 乙 into the preceding page.
	if tail, ok := proofreadingOCRBoundaryTailFromPages(current, next, string(p)+"甲乙"+string(s)); ok {
		t.Fatalf("weak score margin moved next-page character backward: %+v", tail)
	}
}

func TestProofreadingOCRBoundaryAlignsWhenTailHasFooterNoiseAndNextPageMatches(t *testing.T) {
	currentText := "本页包含足够长的大段诗文内容，用于提供可靠的整页字符对齐基准。在页面末尾由于排版原因附带有少量的书眉或页脚等非正文识别噪音，而底稿正文后续紧接着下一页诗人的介绍与汇评文字。"
	nextText := "李端，生卒年不详，赵州人，大历五年登进士第，授秘书省校书郎。以病辞官，居终南山草堂寺。"
	tailNoise := "/ 唐诗汇评"
	current, err := proofreadingOCRCacheTestPage(81, currentText+"\n"+tailNoise).Layout()
	if err != nil {
		t.Fatal(err)
	}
	next, err := proofreadingOCRCacheTestPage(82, nextText).Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := currentText + "\n" + nextText + strings.Repeat("后续大段文字", 1000)
	tail, ok := proofreadingOCRBoundaryTailFromPages(current, next, text)
	if !ok {
		t.Fatalf("expected boundary alignment with tail noise, got ok=false")
	}
	if tail.Start != len([]rune(currentText))+1 {
		t.Fatalf("boundary Start = %d, want %d", tail.Start, len([]rune(currentText))+1)
	}
}

func TestProofreadingOCRBoundaryAlignsWhenPageHasLaTeXUnderlinesAndFormatNoise(t *testing.T) {
	currentText := "大历五年登进士第，授秘书省校书郎。以病辞官，居终南山草堂寺。与钱起、卢纶、吉中孚、韩翃、司空曙、苗发、崔峒、耿湋、夏侯审齐名，号为大历十才子。早年多宦游江南，晚年隐居衡山以卒。李端工诗，清润婉丽，善叙羁旅离情。其《赠郭暧家妓》云：“鸣筝金粟柱，素手玉房前。欲得周郎顾，时时误拂弦。”尤为脍炙人口。"
	ocrCurrent := "大历五年登进士第，$\\underline{\\text{授秘书省校书郎}}$。以病辞官，居终南山草堂寺。与钱起、卢纶、吉中孚、韩翃、司空曙、苗发、崔峒、耿湋、夏侯审齐名，号为$\\underline{\\text{大历十才子}}$。早年多宦游江南，晚年隐居衡山以卒。李端工诗，清润婉丽，善叙羁旅离情。其《赠郭暧家妓》云：“鸣筝金粟柱，素手玉房前。欲得周郎顾，时时误拂弦。”尤为脍炙人口。\n/ 增订本 四"
	nextText := "开元中，卢象以文词显，天宝中，元结以雅论著，至贞元中，韦应物以清真道古之辞擅名一时。李端与其颉颃相抗，风骨独存。"
	current, err := proofreadingOCRCacheTestPage(82, ocrCurrent).Layout()
	if err != nil {
		t.Fatal(err)
	}
	next, err := proofreadingOCRCacheTestPage(83, nextText).Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := currentText + "\n" + nextText + strings.Repeat("后续文字", 1000)
	tail, ok := proofreadingOCRBoundaryTailFromPages(current, next, text)
	if !ok {
		t.Fatalf("expected boundary alignment with LaTeX underline formatting, got ok=false")
	}
	if tail.Start != len([]rune(currentText))+1 {
		t.Fatalf("boundary Start = %d, want %d", tail.Start, len([]rune(currentText))+1)
	}
}
