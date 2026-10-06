package markdowndoc

import (
	"bytes"
	"codeberg.org/go-latex/latex/drawtex/drawimg"
	"codeberg.org/go-latex/latex/mtex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"sync"
	"unicode"
)

var mathMu sync.Mutex

// The math library handles symbols and roots but lacks script nodes. Compose
// script/fraction boxes here; unsupported commands remain explicit errors.
func RenderMath(expression string, display bool) (img image.Image, err error) {
	if len(expression) > 4096 || strings.TrimSpace(expression) == "" {
		return nil, fmt.Errorf("formula is empty or exceeds 4096 bytes")
	}
	mathMu.Lock()
	defer mathMu.Unlock()
	defer func() {
		if p := recover(); p != nil {
			img = nil
			err = fmt.Errorf("unsupported formula: %v", p)
		}
	}()
	size := 20.0
	if display {
		size = 28
	}
	p := mathParser{r: []rune(expression)}
	b, err := p.sequence(size, 0)
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.r) {
		return nil, fmt.Errorf("unexpected formula delimiter")
	}
	return b.img, nil
}

type mathBox struct {
	img  image.Image
	base int
}
type mathParser struct {
	r   []rune
	pos int
}

func mathAtom(value string, size float64) (mathBox, error) {
	var b bytes.Buffer
	if err := mtex.Render(drawimg.NewRenderer(&b), "$"+value+"$", size, 144, nil); err != nil {
		return mathBox{}, err
	}
	img, err := png.Decode(&b)
	if err != nil {
		return mathBox{}, err
	}
	return mathBox{img: img, base: img.Bounds().Dy() * 4 / 5}, nil
}
func (p *mathParser) group(size float64, depth int) (mathBox, error) {
	if p.pos >= len(p.r) {
		return mathBox{}, fmt.Errorf("missing formula argument")
	}
	if p.r[p.pos] == '{' {
		p.pos++
		b, err := p.sequence(size, depth+1)
		if err != nil {
			return b, err
		}
		if p.pos >= len(p.r) || p.r[p.pos] != '}' {
			return b, fmt.Errorf("unclosed formula group")
		}
		p.pos++
		return b, nil
	}
	return p.atom(size, depth+1)
}
func (p *mathParser) atom(size float64, depth int) (mathBox, error) {
	if depth > 24 {
		return mathBox{}, fmt.Errorf("formula nesting exceeds 24")
	}
	if p.pos >= len(p.r) {
		return mathBox{}, fmt.Errorf("missing formula atom")
	}
	if p.r[p.pos] == '{' {
		return p.group(size, depth)
	}
	start := p.pos
	p.pos++
	if p.r[start] == '\\' {
		for p.pos < len(p.r) && unicode.IsLetter(p.r[p.pos]) {
			p.pos++
		}
		name := string(p.r[start:p.pos])
		if name == `\frac` {
			a, err := p.group(size*.9, depth+1)
			if err != nil {
				return a, err
			}
			b, err := p.group(size*.9, depth+1)
			if err != nil {
				return b, err
			}
			w := max(a.img.Bounds().Dx(), b.img.Bounds().Dx()) + 8
			h := a.img.Bounds().Dy() + b.img.Bounds().Dy() + 8
			img := image.NewNRGBA(image.Rect(0, 0, w, h))
			draw.Draw(img, image.Rect((w-a.img.Bounds().Dx())/2, 0, w, a.img.Bounds().Dy()), a.img, image.Point{}, draw.Over)
			y := a.img.Bounds().Dy() + 3
			draw.Draw(img, image.Rect(1, y, w-1, y+1), image.NewUniform(color.Black), image.Point{}, draw.Src)
			draw.Draw(img, image.Rect((w-b.img.Bounds().Dx())/2, y+4, w, h), b.img, image.Point{}, draw.Over)
			return mathBox{img: img, base: y + int(size*.5)}, nil
		}
		if name == `\sqrt` {
			a, err := p.group(size, depth+1)
			if err != nil {
				return a, err
			}
			root, err := mathRoot(a.img.Bounds().Dy())
			if err != nil {
				return a, err
			}
			out := joinMath(root, a)
			img := image.NewNRGBA(out.img.Bounds())
			draw.Draw(img, img.Bounds(), out.img, image.Point{}, draw.Src)
			draw.Draw(img, image.Rect(root.img.Bounds().Dx(), 1, img.Bounds().Dx(), 2), image.NewUniform(color.Black), image.Point{}, draw.Src)
			out.img = img
			return out, nil
		}
	}
	return mathAtom(string(p.r[start:p.pos]), size)
}
func (p *mathParser) sequence(size float64, depth int) (mathBox, error) {
	var result mathBox
	for p.pos < len(p.r) && p.r[p.pos] != '}' {
		if unicode.IsSpace(p.r[p.pos]) {
			p.pos++
			continue
		}
		if p.r[p.pos] == '^' || p.r[p.pos] == '_' {
			return result, fmt.Errorf("script has no base")
		}
		b, err := p.atom(size, depth)
		if err != nil {
			return result, err
		}
		var sup, sub mathBox
		for p.pos < len(p.r) && (p.r[p.pos] == '^' || p.r[p.pos] == '_') {
			kind := p.r[p.pos]
			p.pos++
			s, err := p.group(size*.7, depth+1)
			if err != nil {
				return result, err
			}
			if kind == '^' {
				if sup.img != nil {
					return result, fmt.Errorf("duplicate superscript")
				}
				sup = s
			} else {
				if sub.img != nil {
					return result, fmt.Errorf("duplicate subscript")
				}
				sub = s
			}
		}
		if sup.img != nil || sub.img != nil {
			b = scriptMath(b, sup, sub)
		}
		if result.img == nil {
			result = b
		} else {
			result = joinMath(result, b)
		}
	}
	if result.img == nil {
		result = mathBox{img: image.NewNRGBA(image.Rect(0, 0, 1, 1))}
	}
	if result.img.Bounds().Dx() > 8192 || result.img.Bounds().Dy() > 4096 {
		return mathBox{}, fmt.Errorf("formula exceeds rendering bounds")
	}
	return result, nil
}
func joinMath(a, b mathBox) mathBox {
	base := max(a.base, b.base)
	h := base + max(a.img.Bounds().Dy()-a.base, b.img.Bounds().Dy()-b.base)
	w := a.img.Bounds().Dx() + b.img.Bounds().Dx() + 1
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, image.Rect(0, base-a.base, a.img.Bounds().Dx(), h), a.img, image.Point{}, draw.Over)
	draw.Draw(img, image.Rect(a.img.Bounds().Dx()+1, base-b.base, w, h), b.img, image.Point{}, draw.Over)
	return mathBox{img: img, base: base}
}
func scriptMath(b, sup, sub mathBox) mathBox {
	top, bottom, sw := 0, 0, 0
	if sup.img != nil {
		top = sup.img.Bounds().Dy() * 2 / 3
		sw = sup.img.Bounds().Dx()
	}
	if sub.img != nil {
		bottom = sub.img.Bounds().Dy() * 2 / 3
		sw = max(sw, sub.img.Bounds().Dx())
	}
	w, h := b.img.Bounds().Dx()+sw+2, b.img.Bounds().Dy()+top+bottom
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, image.Rect(0, top, w, h), b.img, image.Point{}, draw.Over)
	x := b.img.Bounds().Dx() + 1
	if sup.img != nil {
		draw.Draw(img, image.Rect(x, 0, w, h), sup.img, image.Point{}, draw.Over)
	}
	if sub.img != nil {
		draw.Draw(img, image.Rect(x, top+b.base, w, h), sub.img, image.Point{}, draw.Over)
	}
	return mathBox{img: img, base: b.base + top}
}

func mathRoot(height int) (mathBox, error) {
	height = max(16, height)
	img := image.NewNRGBA(image.Rect(0, 0, 14, height))
	points := []image.Point{{0, height / 2}, {3, height/2 - 2}, {6, height - 3}, {12, 1}, {14, 1}}
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		steps := max(abs(b.X-a.X), abs(b.Y-a.Y))
		for j := 0; j <= steps; j++ {
			x, y := a.X+(b.X-a.X)*j/max(1, steps), a.Y+(b.Y-a.Y)*j/max(1, steps)
			for dx := 0; dx < 2; dx++ {
				img.Set(x+dx, y, color.Black)
			}
		}
	}
	return mathBox{img: img, base: height * 4 / 5}, nil
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
