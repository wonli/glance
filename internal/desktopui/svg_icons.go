package desktopui

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io/fs"
	"regexp"
	"sync"

	"gioui.org/op/paint"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

//go:embed assets/icons
var svgIconAssets embed.FS

type svgIconCacheKey struct {
	family  string
	name    string
	size    int
	tint    color.NRGBA
	colored bool
}

// Keep one premultiplied pixel buffer and one stable upload identity per
// immutable SVG raster. This avoids a second cached pixel buffer.
type svgImageOp struct {
	once sync.Once
	op   paint.ImageOp
}

var svgIconCache = struct {
	sync.RWMutex
	images   map[svgIconCacheKey]image.Image
	imageOps map[*image.RGBA]*svgImageOp
}{images: make(map[svgIconCacheKey]image.Image), imageOps: make(map[*image.RGBA]*svgImageOp)}

func iconImageOp(icon image.Image) paint.ImageOp {
	if raster, ok := icon.(*image.RGBA); ok {
		svgIconCache.RLock()
		cached := svgIconCache.imageOps[raster]
		svgIconCache.RUnlock()
		if cached != nil {
			cached.once.Do(func() { cached.op = paint.NewImageOp(raster) })
			return cached.op
		}
	}
	return paint.NewImageOp(icon)
}

var svgColorToken = regexp.MustCompile(`(?i)#[0-9a-f]{3}(?:[0-9a-f]{3})?\b|currentColor|rgb\([^)]*\)`)
var svgIconNameToken = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func renderSVGIcon(family, name string, size int, tint *color.NRGBA) (image.Image, error) {
	key := svgIconCacheKey{family: family, name: name, size: size}
	if tint != nil {
		key.tint, key.colored = *tint, true
	}
	svgIconCache.RLock()
	cachedImage, ok := svgIconCache.images[key]
	svgIconCache.RUnlock()
	if ok {
		return cachedImage, nil
	}

	if size <= 0 {
		return nil, fmt.Errorf("SVG icon size must be positive: %d", size)
	}
	if !svgIconNameToken.MatchString(family) || !svgIconNameToken.MatchString(name) || !fs.ValidPath(family) {
		return nil, fmt.Errorf("unknown SVG icon family %q", family)
	}

	data, err := svgIconAssets.ReadFile("assets/icons/" + family + "/" + name + ".svg")
	if err != nil {
		return nil, fmt.Errorf("read %s SVG icon %q: %w", family, name, err)
	}
	if tint != nil {
		replacement := fmt.Sprintf("#%02X%02X%02X", tint.R, tint.G, tint.B)
		data = svgColorToken.ReplaceAll(data, []byte(replacement))
	}
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse %s SVG icon %q: %w", family, name, err)
	}
	if icon.ViewBox.W <= 0 || icon.ViewBox.H <= 0 {
		return nil, fmt.Errorf("SVG icon %s/%s has an invalid viewBox", family, name)
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := float64(size) / max(icon.ViewBox.W, icon.ViewBox.H)
	w, h := icon.ViewBox.W*scale, icon.ViewBox.H*scale
	icon.SetTarget((float64(size)-w)/2, (float64(size)-h)/2, w, h)
	if family == "home" {
		// oksvg transforms paths in SetTarget, but leaves stroke widths in
		// source units. Scale home strokes with the raster to retain their
		// weight at small menu sizes and at Retina pixel densities.
		for i := range icon.SVGPaths {
			icon.SVGPaths[i].LineWidth *= scale
		}
	}
	scanner := rasterx.NewScannerGV(size, size, img, img.Bounds())
	icon.Draw(rasterx.NewDasher(size, size, scanner), 1)

	// Convert exactly as paint.NewImageOp did at every draw. Cache that result
	// instead of the NRGBA source, so visible GPU pixels stay unchanged and the
	// source buffer can be collected once this rasterization returns.
	gpuImage := image.NewRGBA(img.Bounds())
	draw.Draw(gpuImage, gpuImage.Bounds(), img, img.Bounds().Min, draw.Src)
	svgIconCache.Lock()
	var result image.Image = gpuImage
	if cached, exists := svgIconCache.images[key]; exists {
		result = cached
	} else {
		svgIconCache.images[key] = result
		svgIconCache.imageOps[gpuImage] = &svgImageOp{}
	}
	svgIconCache.Unlock()
	return result, nil
}

func svgIconTint(family, name string, tint color.NRGBA) (image.Image, bool) {
	return svgIconTintAtSize(family, name, 96, tint)
}

func svgIconTintAtSize(family, name string, size int, tint color.NRGBA) (image.Image, bool) {
	icon, err := renderSVGIcon(family, name, size, &tint)
	return icon, err == nil
}

func svgIconOriginal(family, name string) (image.Image, bool) {
	icon, err := renderSVGIcon(family, name, 96, nil)
	return icon, err == nil
}
