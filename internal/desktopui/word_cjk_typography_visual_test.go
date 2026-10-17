package desktopui

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/document"
	"glance/internal/wordlayout"
)

func TestWordCJKTypographyPreview(t *testing.T) {
	path := os.Getenv("GLANCE_WORD_TYPOGRAPHY_PREVIEW")
	if path == "" {
		t.Skip("set GLANCE_WORD_TYPOGRAPHY_PREVIEW for the local font comparison")
	}
	shaper, err := newWordTextShaper()
	if err != nil {
		t.Fatal(err)
	}
	window, err := headless.NewWindow(820, 160)
	if err != nil {
		t.Fatalf("Metal GPU unavailable: %v", err)
	}
	defer window.Release()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Constraints: layout.Exact(image.Pt(820, 160)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	paint.Fill(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	// Top: the old Latin-family fallback; bottom: the corrected CJK default.
	for row, eastAsia := range []string{"仿宋", ""} {
		style := document.WordTextStyle{FontFamily: "仿宋", FontFamilyASCII: "仿宋", FontFamilyHighANSI: "仿宋", FontFamilyComplexScript: "仿宋", FontFamilyEastAsia: eastAsia, FontSize: 12, FontColor: "000000"}
		run := document.WordTextRun{ID: "run", Text: "卫生健康领域的安全隐患管理与信息交流", Style: style}
		paragraph := document.WordParagraph{ID: "p", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &run}}}
		result, err := wordlayout.NewEngine(shaper).Layout(context.Background(), wordlayout.Input{Blocks: []document.WordBlock{{Kind: document.WordParagraphBlock, Paragraph: &paragraph}}, DefaultPageSetup: document.WordPageSetup{WidthPT: 400, HeightPT: 50, MarginLeftPT: 8, MarginRightPT: 8, MarginTopPT: 8, MarginBottomPT: 8}})
		if err != nil || !result.CanPreview || len(result.Pages) == 0 {
			t.Fatalf("layout: %v preview=%v", err, result.CanPreview)
		}
		for _, fragment := range result.Pages[0].Text {
			drawWordPageTextFragment(gtx, image.Pt(0, row*80), fragment, 2)
		}
	}
	if err := window.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	shot := image.NewRGBA(image.Rect(0, 0, 820, 160))
	if err := window.Screenshot(shot); err != nil {
		t.Fatal(err)
	}
	output, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	if err := png.Encode(output, shot); err != nil {
		t.Fatal(err)
	}
	t.Log("saved actual GPU font comparison", path)
}
