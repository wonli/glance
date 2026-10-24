package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/i18n"
	"glance/internal/proofhistory"
)

func TestProofreadingHistoryPageUsesFullWindowSplitLayout(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		w := a.ensureProofreadingWorkbench()
		w.historyOpen = true
		w.historyPage = 9
		w.historyPages = []proofhistory.PageSummary{{
			Page: 9, CommitCount: 1,
			Latest: proofhistory.Entry{Hash: "abcdef0123456789", Author: "Reader", Date: "2026-10-24T17:05:55+08:00", Message: "第 9 页：确认校对"},
		}}
		w.history = []proofhistory.PageChange{{
			Entry: proofhistory.Entry{Hash: "abcdef0123456789", Author: "Reader", Date: "2026-10-24T17:05:55+08:00", Message: "第 9 页：确认校对"},
			Paths: []string{"pages/000009.json"}, Status: "added", Additions: 4, FilesChanged: 1,
			Diff: "+ first line\n+ second line",
		}}
		w.selectedCommit = "abcdef0123456789"

		var ops op.Ops
		size := image.Pt(int(1440*scale), int(900*scale))
		gtx := layout.Context{
			Ops:         &ops,
			Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
			Constraints: layout.Exact(size),
		}
		dims := a.layoutProofreadingHistoryPage(gtx)
		if dims.Size != size {
			t.Fatalf("scale=%gx: history page size=%v, want full window %v", scale, dims.Size, size)
		}
		img := renderPresentationTestOps(t, &ops, size)
		wantPanel := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		if got := color.NRGBAModel.Convert(img.At(int(80*scale), int(260*scale))).(color.NRGBA); got != wantPanel {
			t.Fatalf("scale=%gx: left page-list panel pixel=%v, want %v", scale, got, wantPanel)
		}
		if got := color.NRGBAModel.Convert(img.At(int(500*scale), int(260*scale))).(color.NRGBA); got != wantPanel {
			t.Fatalf("scale=%gx: right timeline panel pixel=%v, want %v", scale, got, wantPanel)
		}
		wantBackground := color.NRGBA{R: 247, G: 248, B: 250, A: 255}
		if got := color.NRGBAModel.Convert(img.At(int(20*scale), int(500*scale))).(color.NRGBA); got != wantBackground {
			t.Fatalf("scale=%gx: history page background pixel=%v, want undimmed %v", scale, got, wantBackground)
		}
	}
}

func TestProofreadingHistoryPageMatchesDesignSpacingAndRightAlignment(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		w := a.ensureProofreadingWorkbench()
		w.historyOpen = true
		w.historyPage = 9
		w.historyPages = []proofhistory.PageSummary{{Page: 9}}
		w.history = []proofhistory.PageChange{{
			Entry: proofhistory.Entry{Hash: "abcdef0123456789", Author: "Reader", Date: "2026-10-24T17:05:55+08:00", Message: "第 9 页：确认校对"},
			Paths: []string{"pages/000009.json", "notes.txt"}, Status: "added", Additions: 32, FilesChanged: 2,
			Diff: "+ first line\n+ second line",
		}}
		w.selectedCommit = "abcdef0123456789"

		var ops op.Ops
		size := image.Pt(int(1440*scale), int(900*scale))
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		a.layoutProofreadingHistoryPage(gtx)
		img := renderPresentationTestOps(t, &ops, size)
		if path := os.Getenv("GLANCE_PROOF_HISTORY_SCREENSHOT"); path != "" && scale == 1 {
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, img); err != nil {
				file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		}

		backInk := historyLayoutPixelBounds(img, image.Rect(int(64*scale), int(42*scale), int(300*scale), int(102*scale)), func(c color.NRGBA) bool {
			return int(c.B)-int(c.G) > 8 && int(c.G)-int(c.R) > 8
		})
		if backInk.Empty() {
			t.Fatalf("scale=%gx: returned-workbench label/icon has no blue pixels", scale)
		}
		backCenter := float32(backInk.Min.Y+backInk.Max.Y) / (2 * scale)
		if backCenter < 68 || backCenter > 76 {
			t.Fatalf("scale=%gx: return control ink center y=%.1f, want centered in the 60dp bar at y=72", scale, backCenter)
		}

		titleInk, _, ok := darkInkVerticalBounds(img, image.Rect(int(64*scale), int(130*scale), int(500*scale), int(180*scale)))
		if !ok || float32(titleInk)/scale < 140 || float32(titleInk)/scale > 150 {
			t.Fatalf("scale=%gx: title ink top y=%.1f, want the design's 38dp body inset", scale, float32(titleInk)/scale)
		}
		timelineTitleInk, _, ok := darkInkVerticalBounds(img, image.Rect(int(510*scale), int(340*scale), int(740*scale), int(375*scale)))
		if !ok || float32(timelineTitleInk)/scale < 352 || float32(timelineTitleInk)/scale > 361 {
			t.Fatalf("scale=%gx: timeline title ink top y=%.1f, want the design's top inset", scale, float32(timelineTitleInk)/scale)
		}
		legendArea := image.Rect(int(1000*scale), int(345*scale), int(1360*scale), int(380*scale))
		addedLegend := historyLayoutPixelBounds(img, legendArea, func(c color.NRGBA) bool {
			return int(c.G)-int(c.R) > 12 && int(c.G)-int(c.B) > 8
		})
		deletedLegend := historyLayoutPixelBounds(img, legendArea, func(c color.NRGBA) bool {
			return int(c.R)-int(c.G) > 20 && int(c.R)-int(c.B) > 20
		})
		changedLegend := historyLayoutPixelBounds(img, legendArea, func(c color.NRGBA) bool {
			return int(c.B)-int(c.R) > 12 && int(c.R)-int(c.G) > 8
		})
		if addedLegend.Empty() || deletedLegend.Empty() || changedLegend.Empty() || float32(addedLegend.Min.X)/scale < 1092 || float32(addedLegend.Min.X)/scale > 1110 || float32(deletedLegend.Min.X)/scale < 1175 || float32(deletedLegend.Min.X)/scale > 1193 || float32(changedLegend.Min.X)/scale < 1258 || float32(changedLegend.Min.X)/scale > 1276 {
			t.Fatalf("scale=%gx: legend bounds added=%v deleted=%v changed=%v, want design-aligned right group", scale, addedLegend, deletedLegend, changedLegend)
		}

		statsInk := historyLayoutPixelBounds(img, image.Rect(int(1180*scale), int(445*scale), int(1345*scale), int(475*scale)), func(c color.NRGBA) bool {
			return int(c.G)-int(c.R) > 12 && int(c.G)-int(c.B) > 8
		})
		if statsInk.Empty() || float32(statsInk.Max.X)/scale < 1315 || float32(statsInk.Max.X)/scale > 1334 {
			allAddedInk := historyLayoutPixelBounds(img, image.Rect(0, 0, size.X, size.Y), func(c color.NRGBA) bool {
				return int(c.G)-int(c.R) > 12 && int(c.G)-int(c.B) > 8
			})
			t.Fatalf("scale=%gx: right-aligned change stats bounds=%v, all green ink=%v, want right edge near x=1330", scale, statsInk, allAddedInk)
		}
	}
}

func TestProofreadingHistoryPageListCountAndActionAlignment(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	a := New(nil, "")
	a.applyLocaleSelection(i18n.SelectionChinese)
	w := a.ensureProofreadingWorkbench()
	w.historyPages = []proofhistory.PageSummary{{Page: 1}, {Page: 2}}

	if got := fmt.Sprintf(a.textID("proofread.history.pages_count"), len(w.historyPages)); got != "2 条" {
		t.Errorf("Chinese page-list count = %q, want row count", got)
	}

	var ops op.Ops
	size := image.Pt(700, 300)
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
	a.layoutProofreadingHistoryPages(gtx)
	img := renderPresentationTestOps(t, &ops, size)
	countInk := historyLayoutPixelBounds(img, image.Rect(540, 0, 680, 60), func(c color.NRGBA) bool {
		return c.R < 180 && int(c.G)-int(c.R) > 3 && int(c.B)-int(c.G) > 3
	})
	if countInk.Empty() {
		t.Fatal("page-list row count has no visible text in the header")
	}
	center := float32(countInk.Min.Y+countInk.Max.Y) / 2
	if center < 29 || center > 36 {
		t.Fatalf("page-list row count ink center y=%.1f, want centered in the 65dp header", center)
	}

	var rowOps op.Ops
	rowSize := image.Pt(700, 85)
	rowGtx := layout.Context{Ops: &rowOps, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(rowSize)}
	a.layoutProofreadingHistoryPageRow(rowGtx, proofhistory.PageSummary{Page: 9})
	rowImg := renderPresentationTestOps(t, &rowOps, rowSize)
	actionInk := historyLayoutPixelBounds(rowImg, image.Rect(580, 0, 700, 85), func(c color.NRGBA) bool {
		return int(c.G)-int(c.R) > 10 && int(c.B)-int(c.G) > 10
	})
	if actionInk.Empty() {
		t.Fatal("page-row history action has no visible text")
	}
	actionCenter := float32(actionInk.Min.Y+actionInk.Max.Y) / 2
	if actionCenter < 40 || actionCenter > 45 {
		t.Fatalf("page-row history action ink center y=%.1f, want centered in the 85dp row", actionCenter)
	}
}

func historyLayoutPixelBounds(img image.Image, area image.Rectangle, matches func(color.NRGBA) bool) image.Rectangle {
	bounds := image.Rectangle{Min: image.Pt(area.Max.X, area.Max.Y), Max: image.Pt(area.Min.X, area.Min.Y)}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if !matches(c) {
				continue
			}
			bounds.Min.X = min(bounds.Min.X, x)
			bounds.Min.Y = min(bounds.Min.Y, y)
			bounds.Max.X = max(bounds.Max.X, x+1)
			bounds.Max.Y = max(bounds.Max.Y, y+1)
		}
	}
	if bounds.Min.X == area.Max.X {
		return image.Rectangle{}
	}
	return bounds
}

func TestProofreadingHistoryPageHasSelectablePageAndCommitState(t *testing.T) {
	w := (&App{}).ensureProofreadingWorkbench()
	if w.historyPageList.Axis != layout.Vertical || w.historyTimelineList.Axis != layout.Vertical {
		t.Fatal("history page and timeline lists must have stable vertical list state")
	}
	page := w.button("history-page-9")
	commit := w.button("history-commit-abcdef0")
	if page != w.button("history-page-9") || commit != w.button("history-commit-abcdef0") {
		t.Fatal("history page and commit buttons must retain stable widget state")
	}
}
