package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/proofhistory"
	"image"
	"strings"
	"testing"
)

func TestProofreadingHistoryDiffGrowsForAllLinesAndWrappedText(t *testing.T) {
	a := New(nil, "")
	for _, scale := range []float32{1, 2} {
		measure := func(diff string) int {
			var ops op.Ops
			g := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(int(800*scale), 1000000)}}
			return a.layoutProofreadingHistoryCommit(g, proofhistory.PageChange{Entry: proofhistory.Entry{Hash: "abcdef0123456789"}, Diff: diff}).Size.Y
		}
		short := measure("+ first\n+ second")
		if long := measure(strings.Repeat("+ complete history\n", 40)); long <= short+int(300*scale) {
			t.Fatalf("%gx: history lines clipped: short=%d long=%d", scale, short, long)
		}
		if wrapped := measure("+ " + strings.Repeat("完整历史内容", 200)); wrapped <= short+int(100*scale) {
			t.Fatalf("%gx: long history line clipped: short=%d wrapped=%d", scale, short, wrapped)
		}
	}
}
