package desktopui

import (
	"fmt"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/format/markdowndoc"
	"image"
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
