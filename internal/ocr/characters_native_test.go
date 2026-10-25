//go:build darwin && cgo

package ocr

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"
	"time"

	"glance/internal/format/pdfdoc"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func TestNativeCharacterGeometryMeasuredBoxes(t *testing.T) {
	small := image.NewRGBA(image.Rect(0, 0, 240, 35))
	draw.Draw(small, small.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	d := font.Drawer{Dst: small, Src: image.NewUniform(color.Black), Face: basicfont.Face7x13, Dot: fixed.P(10, 23)}
	d.DrawString("HELLO WORLD 1234")
	large := image.NewRGBA(image.Rect(0, 0, 960, 140))
	xdraw.NearestNeighbor.Scale(large, large.Bounds(), small, small.Bounds(), draw.Src, nil)
	layout := Layout{Width: 960, Height: 140, Blocks: []LayoutBlock{{ID: 7, Text: "HELLO WORLD 1234", Label: "text", Bounds: [4]float64{0, 0, 960, 140}}}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := LocalizeCharacters(ctx, large, layout)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Characters) < 8 {
		t.Fatalf("expected measured native character boxes, got %d", len(result.Characters))
	}
	for _, c := range result.Characters {
		if c.Bounds[2]-c.Bounds[0] > 100 {
			t.Fatalf("word-sized rectangle mislabeled as a character: %+v", c)
		}
	}
}

// Opt-in read-only probe of an actual scanned page. No OCR cloud call or cache
// write is made; the caller supplies an existing PDF and its cached layout.
func TestNativeCharacterGeometryScannedPage(t *testing.T) {
	path, cache := os.Getenv("GLANCE_OCR_GEOMETRY_PDF"), os.Getenv("GLANCE_OCR_GEOMETRY_LAYOUT")
	if path == "" || cache == "" {
		t.Skip("set PDF and layout paths for a scanned-page probe")
	}
	data, err := os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		PageNumber int    `json:"page_number"`
		Layout     Layout `json:"layout"`
	}
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	doc, err := pdfdoc.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	pixels, _, err := doc.RenderVisualPage(record.PageNumber-1, 144)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	result, err := LocalizeCharacters(ctx, pixels, record.Layout)
	if err != nil {
		t.Fatal(err)
	}
	byBlock := map[int]int{}
	for _, c := range result.Characters {
		byBlock[c.BlockID]++
	}
	t.Logf("page=%d measured characters=%d by block=%v", record.PageNumber, len(result.Characters), byBlock)
	if out := os.Getenv("GLANCE_OCR_GEOMETRY_PREVIEW"); out != "" {
		preview := image.NewRGBA(pixels.Bounds())
		draw.Draw(preview, preview.Bounds(), pixels, pixels.Bounds().Min, draw.Src)
		red := image.NewUniform(color.RGBA{R: 220, A: 255})
		for _, character := range result.Characters {
			b := character.Bounds
			w, h := preview.Bounds().Dx(), preview.Bounds().Dy()
			box := image.Rect(int(b[0]*float64(w)/float64(result.Width)), int(b[1]*float64(h)/float64(result.Height)), int(b[2]*float64(w)/float64(result.Width)), int(b[3]*float64(h)/float64(result.Height)))
			for _, edge := range []image.Rectangle{image.Rect(box.Min.X, box.Min.Y, box.Max.X, box.Min.Y+2), image.Rect(box.Min.X, box.Max.Y-2, box.Max.X, box.Max.Y), image.Rect(box.Min.X, box.Min.Y, box.Min.X+2, box.Max.Y), image.Rect(box.Max.X-2, box.Min.Y, box.Max.X, box.Max.Y)} {
				draw.Draw(preview, edge, red, image.Point{}, draw.Src)
			}
		}
		file, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, preview)
		closeErr := file.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}

	if len(result.Characters) < 20 {
		t.Fatalf("insufficient measured geometry on scanned sample: %d", len(result.Characters))
	}
}
