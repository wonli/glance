package ocr

import (
	"image"
	"image/draw"
	"math"
)

type verticalCharacterCell struct {
	source image.Rectangle
	strip  image.Rectangle
}

// Recompose upright vertical glyphs into a horizontal strip for recognition.
// Bands are separated at observed low-ink valleys. Returned boxes still come
// from Vision; the band rectangles are never published as character boxes.
func unfoldVerticalCharacters(source *image.RGBA, count int) (*image.RGBA, []verticalCharacterCell) {
	if count < 2 || count > 80 || source.Bounds().Dy() < count*4 {
		return source, nil
	}
	width, height := source.Bounds().Dx(), source.Bounds().Dy()
	ink := make([]int, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			p := source.RGBAAt(x, y)
			if int(p.R)*299+int(p.G)*587+int(p.B)*114 < 175000 {
				ink[y]++
			}
		}
	}
	cuts := []int{0}
	cellHeight := float64(height) / float64(count)
	for i := 1; i < count; i++ {
		expected := float64(i) * cellHeight
		low := max(cuts[len(cuts)-1]+2, int(expected-cellHeight*.22))
		high := min(height-2, int(expected+cellHeight*.22))
		bestY, best := int(expected), math.Inf(1)
		for y := low; y <= high; y++ {
			score := float64(ink[y-1]+ink[y]+ink[y+1])*10 + math.Abs(float64(y)-expected)/max(1, cellHeight)
			if score < best {
				best, bestY = score, y
			}
		}
		cuts = append(cuts, bestY)
	}
	cuts = append(cuts, height)
	padding := max(3, width/12)
	stripHeight := 0
	for i := 0; i < count; i++ {
		stripHeight = max(stripHeight, cuts[i+1]-cuts[i])
	}
	result := image.NewRGBA(image.Rect(0, 0, count*(width+padding), stripHeight+2*padding))
	for i := range result.Pix {
		result.Pix[i] = 255
	}
	var cells []verticalCharacterCell
	for i := 0; i < count; i++ {
		original := image.Rect(0, cuts[i], width, cuts[i+1])
		target := image.Rect(i*(width+padding), padding+(stripHeight-original.Dy())/2, i*(width+padding)+width, padding+(stripHeight-original.Dy())/2+original.Dy())
		draw.Draw(result, target, source, original.Min, draw.Src)
		cells = append(cells, verticalCharacterCell{source: original, strip: target})
	}
	return result, cells
}

func foldVerticalCharacterLines(lines []detectedTextLine, cells []verticalCharacterCell, strip image.Point, source *image.RGBA) []detectedTextLine {
	if len(cells) == 0 {
		return lines
	}
	original := source.Bounds().Size()
	var result []detectedTextLine
	for _, line := range lines {
		mapped := detectedTextLine{Bounds: [4]float64{1, 1, 0, 0}}
		for _, character := range line.Characters {
			b := character.Bounds
			x0, y0, x1, y1 := b[0]*float64(strip.X), b[1]*float64(strip.Y), b[2]*float64(strip.X), b[3]*float64(strip.Y)
			center := (x0 + x1) / 2
			for _, cell := range cells {
				if center < float64(cell.strip.Min.X) || center >= float64(cell.strip.Max.X) {
					continue
				}
				// A detected word spanning two reassembled cells is not a glyph.
				overlap := math.Min(x1, float64(cell.strip.Max.X)) - math.Max(x0, float64(cell.strip.Min.X))
				if overlap <= 0 || overlap/math.Max(1, x1-x0) < 0.6 {
					break
				}
				x0 = math.Max(x0, float64(cell.strip.Min.X))
				x1 = math.Min(x1, float64(cell.strip.Max.X))
				y0 = math.Max(y0, float64(cell.strip.Min.Y))
				y1 = math.Min(y1, float64(cell.strip.Max.Y))
				character.Bounds = [4]float64{
					max(0, (x0-float64(cell.strip.Min.X)+float64(cell.source.Min.X))/float64(original.X)),
					max(0, (y0-float64(cell.strip.Min.Y)+float64(cell.source.Min.Y))/float64(original.Y)),
					min(1, (x1-float64(cell.strip.Min.X)+float64(cell.source.Min.X))/float64(original.X)),
					min(1, (y1-float64(cell.strip.Min.Y)+float64(cell.source.Min.Y))/float64(original.Y)),
				}
				b := character.Bounds
				measured, ok := verticalInkBounds(source, image.Rect(int(math.Floor(b[0]*float64(original.X))), int(math.Floor(b[1]*float64(original.Y))), int(math.Ceil(b[2]*float64(original.X))), int(math.Ceil(b[3]*float64(original.Y)))).Intersect(cell.source))
				if !ok {
					break
				}
				character.Bounds = [4]float64{float64(measured.Min.X) / float64(original.X), float64(measured.Min.Y) / float64(original.Y), float64(measured.Max.X) / float64(original.X), float64(measured.Max.Y) / float64(original.Y)}
				mapped.Characters = append(mapped.Characters, character)
				r := character.Bounds
				mapped.Bounds[0] = min(mapped.Bounds[0], r[0])
				mapped.Bounds[1] = min(mapped.Bounds[1], r[1])
				mapped.Bounds[2] = max(mapped.Bounds[2], r[2])
				mapped.Bounds[3] = max(mapped.Bounds[3], r[3])
				break
			}
		}
		if len(mapped.Characters) > 0 {
			result = append(result, mapped)
		}
	}
	return result
}

// Tighten the recognized substring rectangle to the ink in its original band.
func verticalInkBounds(source *image.RGBA, region image.Rectangle) (image.Rectangle, bool) {
	region = region.Intersect(source.Bounds())
	var bounds image.Rectangle
	found := false
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			p := source.RGBAAt(x, y)
			if int(p.R)*299+int(p.G)*587+int(p.B)*114 < 175000 {
				if !found {
					bounds = image.Rect(x, y, x+1, y+1)
					found = true
				} else {
					bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
	}
	return bounds, found
}
