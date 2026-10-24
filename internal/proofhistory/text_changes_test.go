package proofhistory

import (
	"math/rand"
	"strings"
	"testing"
)

func TestTextChangesPreserveUnicodeAndExactEdits(t *testing.T) {
	for _, tc := range []struct{ before, after, deleted, added string }{
		{"春眠不觉晓", "春眠觉晓", "不", ""},
		{"甲乙丙丁戊己", "甲丙丁己", "乙戊", ""},
		{"人人人人", "人人人", "人", ""},
		{"你好🌸世界", "你好🌻世界", "🌸", "🌻"},
		{"甲\n乙", "甲 乙", "\n", " "},
		{"", "新增", "", "新增"}, {"删除", "", "删除", ""},
		{"不变", "不变", "", ""},
		{strings.Repeat("甲", 80000) + "删" + strings.Repeat("乙", 80000), strings.Repeat("甲", 80000) + strings.Repeat("乙", 80000), "删", ""},
	} {
		spans := CompareText(tc.before, tc.after)
		var before, after, deleted, added strings.Builder
		for _, span := range spans {
			if span.Kind != TextAdded {
				before.WriteString(span.Text)
			}
			if span.Kind != TextDeleted {
				after.WriteString(span.Text)
			}
			if span.Kind == TextDeleted {
				deleted.WriteString(span.Text)
			}
			if span.Kind == TextAdded {
				added.WriteString(span.Text)
			}
		}
		if before.String() != tc.before || after.String() != tc.after {
			t.Fatal("diff did not reconstruct source texts")
		}
		if deleted.String() != tc.deleted || added.String() != tc.added {
			t.Fatalf("deleted=%q added=%q", deleted.String(), added.String())
		}
	}
}

func TestTextChangesBoundedFallbackPreservesUnrelatedLongText(t *testing.T) {
	before, after := strings.Repeat("甲", 20000), strings.Repeat("乙", 20000)
	spans := CompareText(before, after)
	if len(spans) != 2 || spans[0].Text != before || spans[1].Text != after {
		t.Fatal("large replacement lost text")
	}
}

func TestTextChangesReconstructManySmallEdits(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alphabet := []rune("甲乙丙丁🌸\n ")
	for n := 0; n < 300; n++ {
		var a, b []rune
		for i := 0; i < rng.Intn(100); i++ {
			a = append(a, alphabet[rng.Intn(len(alphabet))])
		}
		for _, r := range a {
			if rng.Intn(5) != 0 {
				b = append(b, r)
			}
			if rng.Intn(5) == 0 {
				b = append(b, alphabet[rng.Intn(len(alphabet))])
			}
		}
		var before, after strings.Builder
		for _, span := range CompareText(string(a), string(b)) {
			if span.Kind != TextAdded {
				before.WriteString(span.Text)
			}
			if span.Kind != TextDeleted {
				after.WriteString(span.Text)
			}
		}
		if before.String() != string(a) || after.String() != string(b) {
			t.Fatalf("diff reconstruction failed for %q -> %q", string(a), string(b))
		}
	}
}
