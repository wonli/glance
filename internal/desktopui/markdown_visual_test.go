package desktopui

import (
	"fmt"
	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/document"
	"glance/internal/fontmanager/emojifont"
	"glance/internal/format/markdowndoc"
	"golang.org/x/image/font/gofont/goregular"
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"testing"
)

const markdownVisualSource = "# 产品设计系统\n\n一套为团队协作而构建的设计语言与组件规范。\n\n## ✨ 特性\n\n- **一致性**：统一视觉与交互语言\n- **高效率**：开箱即用的组件资源\n- **可扩展**：支持主题与品牌定制\n\n## 快速开始\n\n```bash\nnpm install @glance/design-system\n```\n\n> 提示：请先阅读组件使用规范。\n\n## 文档目录\n\n1. [快速开始](./docs/start.md)\n2. [组件规范](./docs/components.md)\n3. [更新日志](./docs/changelog.md)\n\n## 数学公式\n\n行内公式 $x^2+y_1$。\n\n$$\n\\frac{a}{b}+\\sqrt{x}\n$$\n"

func TestMarkdownWorkspaceVisualSnapshot(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := markdownTestApp(t)
	v := a.markdownView
	v.editor.SetText(markdownVisualSource)
	a.flushMarkdownDraft()
	p, err := markdowndoc.Parse(markdownVisualSource)
	if err != nil {
		t.Fatal(err)
	}
	images, errors := markdowndoc.PreviewResources(p, "")
	v.requested = markdownVisualSource
	v.accept(markdownParseResult{source: markdownVisualSource, snapshot: p, images: images, errors: errors})
	for _, mode := range []int{0, 1, 2} {
		v.mode = mode
		for _, width := range []int{1440, 640} {
			size := image.Pt(width, 900)
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
			dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(a.layoutWorkspaceTabs), layout.Flexed(1, a.layoutMarkdownDocument))
			if dims.Size != size {
				t.Fatalf("dimensions %v want %v", dims.Size, size)
			}
			window, err := headless.NewWindow(size.X, size.Y)
			if err != nil {
				t.Skipf("headless unavailable: %v", err)
			}
			if err = window.Frame(&ops); err != nil {
				window.Release()
				t.Fatal(err)
			}
			shot := image.NewRGBA(image.Rectangle{Max: size})
			err = window.Screenshot(shot)
			window.Release()
			if err != nil {
				t.Fatal(err)
			}
			if folder := os.Getenv("GLANCE_MARKDOWN_CAPTURE"); folder != "" {
				file, err := os.Create(fmt.Sprintf("%s/markdown-%d-%d.png", folder, mode, width))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, shot)
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestMarkdownPDFPreviewVisualSnapshot(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := markdownTestApp(t)
	v := a.markdownView
	v.editor.SetText("# PDF 导出验收\n\n正文内容与公式 $x^2$。\n\n![视频缩略图](missing-thumbnail.png)\n\n- [x] 完成事项\n\n```js\nconst value = 42;\n```\n")
	a.flushMarkdownDraft()
	a.requestMarkdownPDFExport()
	result := <-v.exports
	v.exports <- result
	a.pollMarkdownFileResults()
	if result.err != nil {
		t.Fatal(result.err)
	}
	for _, size := range []image.Point{image.Pt(800, 600), image.Pt(1200, 800)} {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: size}}
		dims := a.layoutModalDialog(gtx)
		if dims.Size.X > size.X || dims.Size.Y > size.Y {
			t.Fatalf("dialog exceeds window: %v", dims)
		}
		window, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			t.Skipf("headless unavailable: %v", err)
		}
		if err = window.Frame(&ops); err != nil {
			window.Release()
			t.Fatal(err)
		}
		shot := image.NewRGBA(image.Rectangle{Max: size})
		err = window.Screenshot(shot)
		window.Release()
		if err != nil {
			t.Fatal(err)
		}
		if folder := os.Getenv("GLANCE_MARKDOWN_CAPTURE"); folder != "" {
			file, err := os.Create(fmt.Sprintf("%s/pdf-preview-%d.png", folder, size.X))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, shot)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestMarkdownWarningEmojiUsesBundledColorFaceAndKeepsInlineSpacing(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	a := markdownTestApp(t)
	regularFace, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	emojiCollection, err := emojifont.Collection()
	if err != nil {
		t.Fatal(err)
	}
	// Use the same per-character fallback model as the shared manager shaper:
	// regular text first, bundled color emoji last, with no system-font fallback.
	collection := []font.FontFace{{Font: font.Font{Typeface: "Inter"}, Face: regularFace}}
	collection = append(collection, emojiCollection...)
	a.theme.Shaper = newTextShaper(collection)
	a.theme.Face = desktopPreferredTypeface()

	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("%gx", scale), func(t *testing.T) {
			size := image.Pt(int(320*scale), int(80*scale))
			measureWidth := func(value string) int {
				var ops op.Ops
				gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
				return a.layoutMarkdownSpans(gtx, []document.MarkdownSpan{{Text: value}}, 16, color.NRGBA{A: 255}, false, "warning-emoji-width").Size.X
			}
			emojiAdvance := measureWidth("⚠️")
			emojiWithSpaceAdvance := measureWidth("⚠️ ")
			spaceAdvance := measureWidth(" ")
			if got := emojiWithSpaceAdvance - emojiAdvance; absInt(got-spaceAdvance) > int(scale) {
				t.Fatalf("trailing space advance after emoji=%dpx, ordinary body space=%dpx; want the same font metrics", got, spaceAdvance)
			}
			render := func(text string) *image.RGBA {
				t.Helper()
				var ops op.Ops
				gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
				paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
				a.layoutMarkdownSpans(gtx, []document.MarkdownSpan{{Text: text}}, 16, color.NRGBA{A: 255}, false, "warning-emoji")
				window, err := headless.NewWindow(size.X, size.Y)
				if err != nil {
					t.Skipf("GPU unavailable: %v", err)
				}
				if err := window.Frame(&ops); err != nil {
					window.Release()
					t.Fatal(err)
				}
				shot := image.NewRGBA(image.Rectangle{Max: size})
				if err := window.Screenshot(shot); err != nil {
					window.Release()
					t.Fatal(err)
				}
				window.Release()
				return shot
			}

			shot := render("⚠️Text")
			emojiOnly := render("⚠️")
			textOnly := render("Text")
			if folder := os.Getenv("GLANCE_MARKDOWN_CAPTURE"); folder != "" {
				file, err := os.Create(fmt.Sprintf("%s/warning-emoji-%gx.png", folder, scale))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, shot)
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
			}

			yellowCount := 0
			minX, maxX := size.X, -1
			for y := 0; y < size.Y; y++ {
				for x := 0; x < size.X; x++ {
					c := shot.RGBAAt(x, y)
					if c.R > 180 && c.G > 130 && c.B < 110 {
						yellowCount++
						minX = min(minX, x)
						maxX = max(maxX, x)
					}
				}
			}
			if yellowCount < int(4*scale*scale) {
				t.Fatalf("warning emoji has only %d yellow pixels; expected a color glyph", yellowCount)
			}
			minGlyphWidth := int(float32(16) * scale * 0.45)
			if maxX-minX+1 < minGlyphWidth {
				t.Fatalf("warning emoji visible width=%dpx, want at least %dpx", maxX-minX+1, minGlyphWidth)
			}

			emojiBounds := markdownInkBounds(emojiOnly)
			textBounds := markdownInkBounds(textOnly)
			textOrigin := markdownFindRenderedRunOrigin(shot, textOnly, textBounds)
			gap := textOrigin.X + textBounds.Min.X - emojiBounds.Max.X
			t.Logf("emoji-to-text visible gap=%dpx at %gx (%.2fdp)", gap, scale, float64(gap)/float64(scale))
			if gap > int(4*scale) {
				t.Fatalf("visible emoji-to-text gap=%dpx at %gx; want <= %dpx (emoji ink %v, text origin %v ink %v)", gap, scale, int(4*scale), emojiBounds, textOrigin, textBounds)
			}
			spaced := render("⚠️ Text")
			spacedOrigin := markdownFindRenderedRunOrigin(spaced, textOnly, textBounds)
			spacedGap := spacedOrigin.X + textBounds.Min.X - emojiBounds.Max.X
			t.Logf("emoji-to-text gap with one source space=%dpx at %gx (%.2fdp)", spacedGap, scale, float64(spacedGap)/float64(scale))
			if spacedGap > int(8*scale) {
				t.Fatalf("one source space plus emoji has visible gap=%dpx at %gx; want <= %dpx", spacedGap, scale, int(8*scale))
			}
		})
	}
}

func TestMarkdownHighlightedCodePreservesColorEmoji(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	a := markdownTestApp(t)
	regularFace, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	monospaceData, err := os.ReadFile("/System/Library/Fonts/Menlo.ttc")
	if err != nil {
		t.Skipf("Menlo system font is unavailable: %v", err)
	}
	monoFaces, err := opentype.ParseCollection(monospaceData)
	if err != nil {
		t.Skipf("Menlo collection is unavailable: %v", err)
	}
	var menloFace font.FontFace
	for _, candidate := range monoFaces {
		if candidate.Font.Typeface == "Menlo" {
			if _, ok := candidate.Face.Face().NominalGlyph('⚠'); ok {
				menloFace = candidate
				break
			}
		}
	}
	if menloFace.Face == nil {
		t.Skip("Menlo does not provide a monochrome warning glyph on this system")
	}
	emojiCollection, err := emojifont.Collection()
	if err != nil {
		t.Fatal(err)
	}
	collection := []font.FontFace{
		menloFace,
		{Font: font.Font{Typeface: "Inter"}, Face: regularFace},
	}
	collection = append(collection, emojiCollection...)
	a.theme.Shaper = newTextShaper(collection)
	a.theme.Face = desktopPreferredTypeface()

	const source = `![⚠️](xss.svg "onerror=alert(1)")`
	spans := markdowndoc.HighlightCode(source, "markdown")
	size := image.Pt(720, 80)
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
	paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	a.layoutMarkdownSpans(gtx, spans, 16, color.NRGBA{A: 255}, false, "highlighted-code-emoji")
	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skipf("GPU unavailable: %v", err)
	}
	if err := window.Frame(&ops); err != nil {
		window.Release()
		t.Fatal(err)
	}
	shot := image.NewRGBA(image.Rectangle{Max: size})
	if err := window.Screenshot(shot); err != nil {
		window.Release()
		t.Fatal(err)
	}
	window.Release()

	yellowCount := 0
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			c := shot.RGBAAt(x, y)
			if c.R > 180 && c.G > 130 && c.B < 110 {
				yellowCount++
			}
		}
	}
	if yellowCount < 4 {
		t.Fatalf("highlighted Markdown code rendered warning emoji as a non-color glyph: yellow pixels=%d spans=%+v", yellowCount, spans)
	}
}

func TestMarkdownBoldHeadingKeepsColorEmoji(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	a := markdownTestApp(t)
	regularFace, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	emojiCollection, err := emojifont.Collection()
	if err != nil {
		t.Fatal(err)
	}
	collection := []font.FontFace{{Font: font.Font{Typeface: "Inter"}, Face: regularFace}}
	collection = append(collection, emojiCollection...)
	a.theme.Shaper = newTextShaper(collection)
	a.theme.Face = desktopPreferredTypeface()

	size := image.Pt(720, 100)
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
	paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	a.layoutMarkdownSpans(gtx, []document.MarkdownSpan{{Text: "⚠️ 安全注意事项"}}, 32, color.NRGBA{A: 255}, true, "heading-emoji")
	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skipf("GPU unavailable: %v", err)
	}
	if err := window.Frame(&ops); err != nil {
		window.Release()
		t.Fatal(err)
	}
	shot := image.NewRGBA(image.Rectangle{Max: size})
	if err := window.Screenshot(shot); err != nil {
		window.Release()
		t.Fatal(err)
	}
	window.Release()

	yellowCount := 0
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			c := shot.RGBAAt(x, y)
			if c.R > 180 && c.G > 130 && c.B < 110 {
				yellowCount++
			}
		}
	}
	if yellowCount < 4 {
		t.Fatalf("bold Markdown heading rendered warning emoji as a non-color glyph: yellow pixels=%d", yellowCount)
	}
}

func markdownInkBounds(img *image.RGBA) image.Rectangle {
	bounds := image.Rectangle{Min: image.Pt(img.Bounds().Max.X, img.Bounds().Max.Y)}
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.R < 245 || c.G < 245 || c.B < 245 {
				bounds.Min.X = min(bounds.Min.X, x)
				bounds.Min.Y = min(bounds.Min.Y, y)
				bounds.Max.X = max(bounds.Max.X, x+1)
				bounds.Max.Y = max(bounds.Max.Y, y+1)
			}
		}
	}
	return bounds
}

// markdownFindRenderedRunOrigin aligns the standalone text raster against the
// combined raster. It measures the visible glyph position, including the
// label's real font bearings, rather than comparing two equivalent layouts.
func markdownFindRenderedRunOrigin(combined, standalone *image.RGBA, ink image.Rectangle) image.Point {
	best, bestScore := image.Point{}, int(^uint(0)>>1)
	for dy := -8; dy <= 8; dy++ {
		for dx := 0; dx < 100; dx++ {
			score := 0
			for y := ink.Min.Y; y < ink.Max.Y; y++ {
				for x := ink.Min.X; x < ink.Max.X; x++ {
					want := standalone.RGBAAt(x, y)
					gotX, gotY := x+dx, y+dy
					if !image.Pt(gotX, gotY).In(combined.Bounds()) {
						score += 3 * 255
						continue
					}
					got := combined.RGBAAt(gotX, gotY)
					score += absInt(int(want.R)-int(got.R)) + absInt(int(want.G)-int(got.G)) + absInt(int(want.B)-int(got.B))
				}
			}
			if score < bestScore {
				best, bestScore = image.Pt(dx, dy), score
			}
		}
	}
	return best
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestMarkdownAcceptanceVisualSnapshot(t *testing.T) {
	path := os.Getenv("GLANCE_MARKDOWN_ACCEPTANCE_SOURCE")
	folder := os.Getenv("GLANCE_MARKDOWN_CAPTURE")
	if path == "" || folder == "" {
		t.Skip("optional acceptance captures")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := markdownTestApp(t)
	v := a.markdownView
	source := string(raw)
	v.editor.SetText(source)
	a.flushMarkdownDraft()
	p, err := markdowndoc.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	images, errors := markdowndoc.PreviewResources(p, path)
	for key, reason := range errors {
		t.Logf("resource %s: %s", key, reason)
	}
	a.path = path
	v.requested = source
	v.requestedPath = path
	v.accept(markdownParseResult{source: source, path: path, snapshot: p, images: images, errors: errors})
	v.mode = 2
	for _, heading := range p.Headings {
		if heading.Level != 1 {
			continue
		}
		v.preview.ScrollTo(heading.Block)
		size := image.Pt(1440, 1100)
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
		a.layoutMarkdownDocument(gtx)
		window, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			t.Fatal(err)
		}
		if err = window.Frame(&ops); err != nil {
			window.Release()
			t.Fatal(err)
		}
		shot := image.NewRGBA(image.Rectangle{Max: size})
		err = window.Screenshot(shot)
		window.Release()
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(fmt.Sprintf("%s/acceptance-%03d.png", folder, heading.Block))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, shot)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("capture %03d: %s", heading.Block, heading.Title)
	}
}
