package textalign

import (
	"context"
	"strings"
	"testing"
)

func TestFooterAndPrintedPageNumberAreNotBody(t *testing.T) {
	p := Page{Number: 4, Blocks: []Block{{ID: 1, Label: "paragraph_title", Text: "第四册篇目表"}, {ID: 2, Label: "content", Text: "卢纶送韩都护还边送吉中孚校书归楚州"}, {ID: 3, Label: "footer", Text: "第四册篇目表"}, {ID: 4, Label: "number", Text: "一"}}}
	got, err := Align(context.Background(), []Page{p}, "第四册篇目表\n卢纶送韩都护还边送吉中孚校书归楚州", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected == nil || got[0].Selected.Coverage != 1 {
		t.Fatalf("footer treated as missing body %+v", got[0])
	}
	for _, e := range got[0].Selected.Evidence {
		if e.BlockID > 2 {
			t.Fatal("matched page furniture")
		}
	}
}

func TestCoverDoesNotScavengeUnrelatedSection(t *testing.T) {
	p := Page{Number: 3, Blocks: []Block{{ID: 1, Text: "唐诗学书系之六"}, {ID: 2, Label: "doc_title", Text: "唐诗汇评"}, {ID: 5, Text: "增订本"}, {ID: 3, Text: "孙菊园刘初棠副主编"}, {ID: 4, Text: "上海古籍出版社"}}}
	md := "# 唐诗汇评\n增订本\n孙菊园刘初棠副主编\n\n图书在版编目数据\n唐诗汇评陈伯海主编上海古籍出版社\n唐诗学书系"
	got, err := Align(context.Background(), []Page{p}, md, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected == nil {
		t.Fatal("lost cover location")
	}
	text := string([]rune(md)[got[0].Selected.Range.Start:got[0].Selected.Range.End])
	if strings.Contains(text, "图书在版") {
		t.Fatalf("scavenged CIP content: %q", text)
	}
}

func TestConflictingPageCanMakeSelectionAmbiguous(t *testing.T) {
	pages := []Page{{Number: 1, Blocks: []Block{{Text: "天地玄黄宇宙洪荒"}}}, {Number: 2, Blocks: []Block{{Text: "天地玄黄宇宙洪"}}}}
	got, err := Align(context.Background(), pages, "天地玄黄宇宙洪荒", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected == nil || got[0].Status != "ambiguous" {
		t.Fatalf("skip-page alternative ignored: %+v", got)
	}
}

func TestExactPagePathsDoNotInheritUnrelatedBeamPruning(t *testing.T) {
	pages := []Page{{Number: 1, Blocks: []Block{{Text: "天地玄黄宇宙洪荒"}}}, {Number: 2, Blocks: []Block{{Text: "日月盈昃辰宿列张"}}}}
	got, err := Align(context.Background(), pages, "天地玄黄宇宙洪荒\n日月盈昃辰宿列张", Options{BeamWidth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != "matched" || got[1].Status != "matched" {
		t.Fatalf("beam pruning contaminated clear pages %+v", got)
	}
}

func TestOverlappingWindowsReuseAlignmentEvidence(t *testing.T) {
	chars := make([]rune, 200)
	for i := range chars {
		chars[i] = rune(0x4e00 + i)
	}
	body := string(chars)
	noisy := append([]rune(nil), chars...)
	noisy[70] = '错'
	noisy = append(noisy[:130], noisy[131:]...)
	page := Page{Number: 1, Blocks: []Block{{Text: string(noisy)}}}
	got, err := Align(context.Background(), []Page{page}, "前言文字"+body+"结尾文字", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected == nil || got[0].Selected.Coverage < 0.95 || got[0].Search.CacheHits == 0 {
		t.Fatalf("overlapping candidates recomputed: %+v", got[0])
	}
}

func TestDocumentTitleCannotUseCIPMentionWhenHeadingExists(t *testing.T) {
	p := Page{Number: 3, Blocks: []Block{{ID: 1, Label: "doc_title", Text: "唐诗汇评"}, {ID: 2, Text: "陈伯海主编"}, {ID: 3, Text: "上海古籍出版社"}}}
	md := "# 唐诗汇评\n未转录的封面文字\n\n图书在版编目数据：唐诗汇评，陈伯海主编，上海古籍出版社"
	got, err := Align(context.Background(), []Page{p}, md, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got[0].Candidates {
		for _, e := range c.Evidence {
			if e.BlockID == 1 && e.Range.Start != 2 {
				t.Fatalf("title matched body mention %+v", e)
			}
		}
	}
}

func TestHeadingAnchorRejectsImpossibleWindowsBeforeSearchCap(t *testing.T) {
	chars := make([]rune, 80)
	for i := range chars {
		chars[i] = rune(0x5000 + i)
	}
	body := string(chars)
	page := Page{Number: 4, Blocks: []Block{{Label: "paragraph_title", Text: "卷一目录"}, {Text: body}}}
	md := "# 卷一目录\n" + body + strings.Repeat("\n"+body, 100)
	got, err := Align(context.Background(), []Page{page}, md, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status == "limited" || got[0].Selected == nil || got[0].Selected.Coverage != 1 {
		t.Fatalf("impossible windows exhausted search: %+v", got[0])
	}
}

func TestSkippedEqualCompetitorIsAmbiguous(t *testing.T) {
	p := Page{Number: 1, Blocks: []Block{{Text: "天地玄黄宇宙洪荒"}}}
	q := p
	q.Number = 2
	got, err := Align(context.Background(), []Page{p, q}, "天地玄黄宇宙洪荒", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != "ambiguous" || got[1].Status != "ambiguous" {
		t.Fatalf("equally supported page called unmatched: %+v", got)
	}
}

func TestHeadingLookupHasIndependentBudget(t *testing.T) {
	s, err := NewSource("# " + strings.Repeat("天地", 2000))
	if err != nil {
		t.Fatal(err)
	}
	idx := newIndex(s)
	budget := 64000
	ranges, limited, err := idx.titleRanges(context.Background(), normalized("天地"), &budget)
	if err != nil {
		t.Fatal(err)
	}
	if !limited || len(ranges) > 256 || budget < 0 {
		t.Fatalf("unbounded heading lookup: %d ranges, budget%d", len(ranges), budget)
	}
	budget = 64000
	ranges, limited, err = idx.titleRanges(context.Background(), []rune{'天'}, &budget)
	if err != nil || limited || len(ranges) > 0 || budget != 64000 {
		t.Fatal("single-character title became locator")
	}
}
