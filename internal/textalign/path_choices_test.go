package textalign

import (
	"context"
	"testing"
)

func TestAlignPathChoicesExcludeLocallyRankedButGloballyUnsupportedCandidates(t *testing.T) {
	first := "天地玄黄宇宙洪荒日月盈昃辰宿列张"
	second := "寒来暑往秋收冬藏风起云涌龙腾虎跃"
	markdown := first + "\n" + second + "\n" + first
	pages := []Page{
		{Number: 1, Blocks: []Block{{ID: 1, Text: first}}},
		{Number: 2, Blocks: []Block{{ID: 2, Text: second}}},
	}
	got, err := Align(context.Background(), pages, markdown, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected == nil || len(got[0].Candidates) < 2 {
		t.Fatalf("fixture did not produce a selected route and a local decoy: %+v", got[0])
	}
	lateStart := len([]rune(first + "\n" + second + "\n"))
	localDecoy := false
	for _, candidate := range got[0].Candidates {
		if candidate.Range.Start >= lateStart {
			localDecoy = true
			break
		}
	}
	if !localDecoy {
		t.Fatalf("fixture did not produce a later local candidate: %+v", got[0].Candidates)
	}
	if len(got[0].PathChoices) == 0 || got[0].PathChoices[0].Range != got[0].Selected.Range {
		t.Fatalf("globally selected route must be the first path choice: selected=%+v choices=%+v", got[0].Selected, got[0].PathChoices)
	}
	for _, candidate := range got[0].PathChoices {
		if candidate.Range.Start >= lateStart {
			t.Fatalf("locally ranked but globally unsupported candidate leaked into path choices: %+v", candidate)
		}
	}

	ambiguousText := "甲乙丙丁戊己庚辛壬癸子丑寅卯"
	ambiguous, err := Align(context.Background(), []Page{{Number: 1, Blocks: []Block{{Text: ambiguousText}}}}, ambiguousText+"\n"+ambiguousText, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous[0].Status != "ambiguous" || ambiguous[0].Selected == nil || len(ambiguous[0].PathChoices) < 2 {
		t.Fatalf("globally supported repeated placements should remain reviewable alternatives: %+v", ambiguous[0])
	}
	if ambiguous[0].PathChoices[0].Range != ambiguous[0].Selected.Range {
		t.Fatalf("selected path is not first for ambiguous page: %+v", ambiguous[0].PathChoices)
	}
	for i := 1; i < len(ambiguous[0].PathChoices); i++ {
		if ambiguous[0].PathChoices[i-1].Score < ambiguous[0].PathChoices[i].Score {
			t.Fatalf("alternatives are not in deterministic descending score order: %+v", ambiguous[0].PathChoices)
		}
	}

	unique, err := Align(context.Background(), []Page{{Number: 1, Blocks: []Block{{Text: second}}}}, second, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if unique[0].Status != "matched" || unique[0].Selected == nil || len(unique[0].PathChoices) != 1 || unique[0].PathChoices[0].Range != unique[0].Selected.Range {
		t.Fatalf("unambiguous match should expose only its selected path: %+v", unique[0])
	}
}
