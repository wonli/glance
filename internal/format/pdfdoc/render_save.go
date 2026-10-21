package pdfdoc

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/SalvioniDigitalSolutions/gopdf"
	"glance/internal/document"
	"glance/internal/fileutil"
	"image"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"sync"
)

var substituteOnce sync.Once
var substitute func(gopdf.FontRequest) []byte

// Render returns pixels and hit data derived under the same revision lock.
func (d *Document) Render(index int, dpi float64) (image.Image, document.PDFPage, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.reader == nil {
		return nil, document.PDFPage{}, "", errors.New("PDF 已关闭")
	}
	// Reader operations populate internal caches. Serialize rendering with page
	// reads and mutations, and reuse the already parsed immutable revision.
	return d.render(index, dpi)
}

// RenderThumbnail renders only page pixels and geometry. It skips text-fragment
// extraction because thumbnails do not need hit data.
func (d *Document) RenderThumbnail(index int, dpi float64) (image.Image, document.PDFPage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.reader == nil {
		return nil, document.PDFPage{}, errors.New("PDF 已关闭")
	}
	size, err := d.reader.PageSize(index)
	if err != nil {
		return nil, document.PDFPage{}, err
	}
	page := document.PDFPage{
		Index: index, Revision: d.revision, Width: size.W, Height: size.H,
		Rotation: rotation(d.reader, index),
	}
	pixels, _, err := d.renderPixels(index, page, dpi, false)
	return pixels, page, err
}

func (d *Document) render(index int, dpi float64) (image.Image, document.PDFPage, string, error) {
	p, e := d.page(index)
	if e != nil {
		return nil, p, "", e
	}
	if d.closed {
		return nil, p, "", errors.New("PDF 已关闭")
	}
	img, warning, err := d.renderPixels(index, p, dpi, true)
	return img, p, warning, err
}

func (d *Document) renderPixels(index int, p document.PDFPage, dpi float64, includeText bool) (image.Image, string, error) {
	if p.Width <= 0 || p.Height <= 0 || math.IsNaN(p.Width*p.Height) || math.IsInf(p.Width*p.Height, 0) {
		return nil, "", errors.New("无效页面尺寸")
	}
	dpi = clampRasterDPI(dpi, p.Width, p.Height)
	// The Go renderer currently ignores JPEG 2000 image decode errors. Detect
	// images actually painted on this page before rendering so macOS can use
	// CoreGraphics instead of silently publishing an empty or incomplete page.
	if pageHasJPEG2000Image(d.reader, index) {
		img, warning, err := d.renderNativePage(index, p, dpi, true)
		return img, warning, err
	}
	// Native rendering also consumes this revision snapshot, not the original file.
	// Keep gopdf for annotation appearances: CoreGraphics omits annotation overlays.
	substituteOnce.Do(func() { substitute = gopdf.SystemFonts() })
	img, report, err := d.reader.RenderPageDetail(index, gopdf.RenderOpts{DPI: dpi, IncludeText: includeText, IncludeVector: true, IncludeRasterImages: true, IncludeAnnotations: true, SubstituteFont: substitute})
	if report.Missing > 0 || err != nil {
		if native, warning, nativeErr := d.renderNativePage(index, p, dpi, false); native != nil || nativeErr != nil {
			return native, warning, nativeErr
		}
	}
	warning := ""
	if report.Missing > 0 {
		warning = fmt.Sprintf("此页 %d 个字形无法显示", report.Missing)
	}
	return img, warning, err
}

func clampRasterDPI(dpi, pageWidth, pageHeight float64) float64 {
	if dpi <= 0 || math.IsNaN(dpi) || math.IsInf(dpi, 0) {
		dpi = 72
	}
	dpi = math.Max(24, dpi)
	rasterPixels := func() float64 {
		width := math.Ceil(pageWidth * dpi / 72)
		height := math.Ceil(pageHeight * dpi / 72)
		return width * height
	}
	if pixels := rasterPixels(); pixels > 8e6 {
		dpi *= math.Sqrt(8e6 / pixels)
		for rasterPixels() > 8e6 {
			dpi *= 0.999
		}
	}
	return dpi
}

func pageHasJPEG2000Image(reader *gopdf.Reader, index int) bool {
	images, err := reader.PageImages(index)
	if err != nil {
		return false
	}
	for _, imageRef := range images {
		if imageRef.Filter == "JPXDecode" {
			return true
		}
	}
	return false
}

func (d *Document) renderNativePage(index int, page document.PDFPage, dpi float64, required bool) (image.Image, string, error) {
	width := int(math.Ceil(page.Width * dpi / 72))
	height := int(math.Ceil(page.Height * dpi / 72))
	native := renderNative(d.data, index, width, height)
	if native == nil {
		if required {
			return nil, "", errors.New("此页包含 JPEG 2000 图像，当前平台没有可用的系统 PDF 渲染器")
		}
		return nil, "", nil
	}
	if len(page.Annotations) > 0 {
		overlay, err := d.annotationOverlay(index, dpi)
		if err != nil {
			return native, "批注外观渲染失败，请查看右侧列表", nil
		}
		composed := image.NewRGBA(native.Bounds())
		draw.Draw(composed, composed.Bounds(), native, native.Bounds().Min, draw.Src)
		draw.Draw(composed, composed.Bounds(), overlay, overlay.Bounds().Min, draw.Over)
		native = composed
	}
	return native, "已使用系统 PDF 渲染", nil
}

func (d *Document) Save() error              { d.mu.Lock(); defer d.mu.Unlock(); return d.save(d.path) }
func (d *Document) SaveAs(path string) error { d.mu.Lock(); defer d.mu.Unlock(); return d.save(path) }
func (d *Document) save(path string) error {
	if path == "" {
		return errors.New("请选择 PDF 保存路径")
	}
	if d.closed {
		return errors.New("PDF 已关闭")
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".glance-pdf-*.pdf")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = f.Write(d.data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	r, e := gopdf.Open(tmp)
	if e != nil {
		return e
	}
	if r.Repaired() || r.NumPages() != d.reader.NumPages() {
		return errors.New("PDF 保存验证失败")
	}
	if e = fileutil.ReplaceFile(tmp, path); e != nil {
		return e
	}
	d.path = path
	d.saved = d.data
	d.forcedDirty = false
	d.modified = false
	d.publishInfo()
	return nil
}

// The overlay is a disposable render projection of the same revision. It is
// never used for editing, history or saving.
func (d *Document) annotationOverlay(index int, dpi float64) (image.Image, error) {
	update := gopdf.Update(d.reader)
	if err := update.SetPageEntry(index, "Contents", gopdf.Array{}); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if _, err := update.WriteTo(&b); err != nil {
		return nil, err
	}
	reader, err := gopdf.NewReader(b.Bytes())
	if err != nil {
		return nil, err
	}
	return reader.RenderPage(index, gopdf.RenderOpts{DPI: dpi, IncludeVector: true, IncludeRasterImages: true, IncludeText: true, IncludeAnnotations: true, Transparent: true, SubstituteFont: substitute})
}
