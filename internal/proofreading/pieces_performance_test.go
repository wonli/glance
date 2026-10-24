package proofreading

import (
	"strings"
	"testing"
)

func largeBoxModel(b testing.TB, fragments int) *Model {
	b.Helper()
	s := Snapshot{Version: 2, Next: 1}
	text := strings.Repeat("中文校对 mixed English。", 32)
	for i := 0; i < 892; i++ {
		id := PageID(i + 1)
		s.Pages = append(s.Pages, Page{ID: id})
		for j := 0; j < fragments; j++ {
			s.Runs = append(s.Runs, Run{First: s.Next, Text: text, Page: id})
			s.Next += CharacterID(len([]rune(text)))
		}
	}
	m, err := Restore(s)
	if err != nil {
		b.Fatal(err)
	}
	return m
}

func BenchmarkBoxFineBoundary(b *testing.B) {
	m := largeBoxModel(b, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := m.AdjustBoundary(6, 1); err != nil {
			b.Fatal(err)
		}
		if !m.Undo() {
			b.Fatal("undo")
		}
	}
}

func BenchmarkBoxRefill(b *testing.B) {
	m := largeBoxModel(b, 8)
	n := len([]rune(m.Text(7)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := m.AdjustBoundary(6, n); err != nil {
			b.Fatal(err)
		}
		if !m.Undo() {
			b.Fatal("undo")
		}
	}
}
