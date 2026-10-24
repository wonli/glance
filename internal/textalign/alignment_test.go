package textalign

import (
	"context"
	"strings"
	"testing"
)

func TestSourceMapsOriginalRunesAndSkipsMarkup(t *testing.T) {
	md := "# 卢𠮷\n\n![误导文字](images/错误标题.jpg)\n[天地](https://example.com/不可匹配) **玄黄**"
	s, err := NewSource(md)
	if err != nil {
		t.Fatal(err)
	}
	if string(s.Text) != "卢𠮷天地玄黄" {
		t.Fatalf("indexed %q", string(s.Text))
	}
	r := s.OriginalRange(1, 4)
	if got := string([]rune(md)[r.Start:r.End]); !strings.HasPrefix(got, "𠮷") || !strings.HasSuffix(got, "地") {
		t.Fatalf("mapped %q", got)
	}
}

func TestAlignmentHandlesErrorsAndOriginalOffsets(t *testing.T) {
	md := "# 前言\n\n天地玄黄，宇宙洪荒。日月盈昃，辰宿列张。\n\n# 后记"
	pages := []Page{{Number: 1, Blocks: []Block{{ID: 1, Text: "天地玄黄宇宙洪荒日月盈仄辰宿列张"}}}}
	got, err := Align(context.Background(), pages, md, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected == nil {
		t.Fatalf("no selection: %+v", got[0])
	}
	c := got[0].Selected
	text := string([]rune(md)[c.Range.Start:c.Range.End])
	if text != "天地玄黄，宇宙洪荒。日月盈昃，辰宿列张" {
		t.Fatalf("range %q", text)
	}
	if c.Coverage < 0.9 {
		t.Fatalf("coverage %v", c.Coverage)
	}
}

func TestBlocksMayMoveWithoutReusingText(t *testing.T) {
	pages := []Page{{Number: 1, Blocks: []Block{{ID: 1, Text: "唐诗学书系之六"}, {ID: 2, Text: "唐诗汇评增订本"}, {ID: 3, Text: "陈伯海主编"}, {ID: 4, Text: "孙菊园刘初棠副主编"}}}}
	md := "唐诗学书系之六\n# 唐诗汇评\n增订本\n孙菊园刘初棠副主编\n陈伯海主编"
	got, err := Align(context.Background(), pages, md, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected == nil || !got[0].Selected.Reordered || got[0].Selected.Coverage < 0.95 {
		t.Fatalf("not reordered: %+v", got[0])
	}
	duplicate := []Page{{Number: 1, Blocks: []Block{{ID: 1, Text: "天地玄黄宇宙洪荒"}, {ID: 2, Text: "天地玄黄宇宙洪荒"}}}}
	got, err = Align(context.Background(), duplicate, "天地玄黄宇宙洪荒", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected != nil && got[0].Selected.Coverage > 0.5 {
		t.Fatal("reused the same B text twice")
	}
}

func TestRepeatedTitleRemainsAmbiguous(t *testing.T) {
	pages := []Page{{Number: 1, Blocks: []Block{{ID: 1, Text: "唐诗汇评增订本"}}}}
	got, err := Align(context.Background(), pages, "唐诗汇评增订本\n中间文字\n唐诗汇评增订本", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != "ambiguous" || len(got[0].Candidates) < 2 {
		t.Fatalf("false certainty: %+v", got[0])
	}
}

func TestMissingPageAndLaterAnchor(t *testing.T) {
	p := func(n int, text string) Page { return Page{Number: n, Blocks: []Block{{ID: 0, Text: text}}} }
	pages := []Page{p(1, "唐诗汇评增订本"), p(2, "唐诗书目总录与意象艺术"), p(3, "唐诗汇评增订本"), p(4, "第四册篇目表卢纶三十六首送韩都护还边")}
	md := "唐诗汇评增订本\n唐诗汇评增订本\n第四册篇目表卢纶三十六首送韩都护还边"
	got, err := Align(context.Background(), pages, md, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[1].Status != "unmatched" || got[1].Selected != nil {
		t.Fatalf("missing page assigned: %+v", got[1])
	}
	if got[0].Selected == nil || got[2].Selected == nil || got[3].Selected == nil {
		t.Fatalf("failed to recover: %+v", got)
	}
	if got[0].Selected.Range.End > got[2].Selected.Range.Start || got[2].Selected.Range.End > got[3].Selected.Range.Start {
		t.Fatal("crossed page ranges")
	}
}

func TestCancellationAndBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Align(ctx, []Page{{Number: 1}}, "天地", Options{}); err == nil {
		t.Fatal("ignored cancellation")
	}
	pages := []Page{{Number: 1, Blocks: []Block{{Text: strings.Repeat("天地玄黄宇宙洪荒", 10)}}}}
	got, err := Align(context.Background(), pages, strings.Repeat("天地玄黄宇宙洪荒", 20), Options{MaxCells: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != "limited" {
		t.Fatalf("budget exhaustion reported as %s", got[0].Status)
	}
}

func TestOmittedSourceGapIsNotConsumed(t *testing.T) {
	pages := []Page{{Number: 1, Blocks: []Block{{Text: "天地玄黄宇宙洪荒"}}}, {Number: 2, Blocks: []Block{{Text: "日月盈昃辰宿列张"}}}}
	md := "天地玄黄宇宙洪荒\nB中独有的整段文字必须保留\n日月盈昃辰宿列张"
	got, err := Align(context.Background(), pages, md, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected == nil || got[1].Selected == nil {
		t.Fatalf("missing selections %+v", got)
	}
	gap := string([]rune(md)[got[0].Selected.Range.End:got[1].Selected.Range.Start])
	if !strings.Contains(gap, "整段文字必须保留") {
		t.Fatalf("swallowed gap %q", gap)
	}
	for _, r := range got {
		if r.ExactBoundary {
			t.Fatal("evidence promoted to exact page boundary")
		}
	}
}

func TestRepeatedPostingLimitCannotClaimUnique(t *testing.T) {
	pages := []Page{{Number: 1, Blocks: []Block{{Text: "天地玄黄宇宙洪荒"}}}}
	got, err := Align(context.Background(), pages, strings.Repeat("天地玄黄宇宙洪荒", 300), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != "limited" {
		t.Fatalf("truncated search reports %s", got[0].Status)
	}
}

func TestEntitiesUseVisibleCharactersAndOriginalSpans(t *testing.T) {
	md := "# &#x4e2d;&#x6587; &amp; end"
	s, err := NewSource(md)
	if err != nil {
		t.Fatal(err)
	}
	if string(s.Text) != "中文end" {
		t.Fatalf("indexed entity syntax: %q", string(s.Text))
	}
	r := s.OriginalRange(0, 2)
	if string([]rune(md)[r.Start:r.End]) != "&#x4e2d;&#x6587;" {
		t.Fatalf("wrong entity span %+v", r)
	}
}

func TestRepeatedBlocksCanUseSeparateOccurrences(t *testing.T) {
	phrase := "天地玄黄宇宙洪荒"
	p := Page{Number: 1, Blocks: []Block{{ID: 1, Text: phrase}, {ID: 2, Text: phrase}}}
	got, err := Align(context.Background(), []Page{p}, phrase+"\n"+phrase, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected == nil || got[0].Selected.Coverage != 1 || len(got[0].Selected.Evidence) != 2 {
		t.Fatalf("lost repeated occurrence %+v", got[0])
	}
}

func TestImageBlocksCannotProveCompletePage(t *testing.T) {
	page := Page{Number: 1, Blocks: []Block{{Text: "天地玄黄宇宙洪荒", Label: "text"}, {Label: "image"}}}
	got, err := Align(context.Background(), []Page{page}, "天地玄黄宇宙洪荒", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != "ambiguous" {
		t.Fatalf("unrecognized region reported %s", got[0].Status)
	}
}

func TestOversizedInputsAreBounded(t *testing.T) {
	page := Page{Number: 1, Blocks: []Block{{Text: strings.Repeat("天", 100000)}}}
	got, err := Align(context.Background(), []Page{page}, "天地玄黄", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != "limited" {
		t.Fatalf("large OCR input not limited")
	}
}

func TestLongExactAnchorAvoidsQuadraticAlignment(t *testing.T) {
	chars := make([]rune, 160)
	for i := range chars {
		chars[i] = rune(0x4e00 + i)
	}
	phrase := string(chars)
	page := Page{Number: 1, Blocks: []Block{{Text: phrase}}}
	got, err := Align(context.Background(), []Page{page}, phrase, Options{MaxCells: 10000})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected == nil || got[0].Selected.Coverage != 1 || got[0].Status == "limited" {
		t.Fatalf("exact anchor exhausted DP budget: %+v", got[0])
	}
}

func TestEntityCannotBelongToTwoPages(t *testing.T) {
	pages := []Page{{Number: 1, Blocks: []Block{{Text: "天地玄f"}}}, {Number: 2, Blocks: []Block{{Text: "j宇宙洪"}}}}
	got, err := Align(context.Background(), pages, "天地玄&fjlig;宇宙洪", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected != nil && got[1].Selected != nil && got[0].Selected.Range.End > got[1].Selected.Range.Start {
		t.Fatal("one source entity assigned to two pages")
	}
}

func TestInvalidAndEscapedEntitiesRemainLiteral(t *testing.T) {
	for _, input := range []string{"&notanentity;", `\&amp;`} {
		s, err := NewSource(input)
		if err != nil {
			t.Fatal(err)
		}
		if string(s.Text) != string(normalized(input)) {
			t.Fatalf("changed literal %q to %q", input, string(s.Text))
		}
	}
}
