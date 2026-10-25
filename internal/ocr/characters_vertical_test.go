package ocr

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestVerticalCharactersFoldMeasuredInkToSourcePage(t *testing.T) {
	original := image.NewRGBA(image.Rect(0, 0, 20, 40))
	draw.Draw(original, original.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	expected := []image.Rectangle{image.Rect(3, 3, 17, 17), image.Rect(3, 23, 17, 37)}
	for _, box := range expected {
		draw.Draw(original, box, image.NewUniform(color.Black), image.Point{}, draw.Src)
	}
	strip, cells := unfoldVerticalCharacters(original, 2)
	var line detectedTextLine
	for i, cell := range cells {
		line.Characters = append(line.Characters, detectedCharacter{Text: string([]rune("甲乙")[i]), Bounds: [4]float64{
			float64(cell.strip.Min.X) / float64(strip.Bounds().Dx()), float64(cell.strip.Min.Y) / float64(strip.Bounds().Dy()),
			float64(cell.strip.Max.X+2) / float64(strip.Bounds().Dx()), float64(cell.strip.Max.Y) / float64(strip.Bounds().Dy()),
		}})
	}
	folded := foldVerticalCharacterLines([]detectedTextLine{line}, cells, strip.Bounds().Size(), original)
	if len(folded) != 1 || len(folded[0].Characters) != 2 {
		t.Fatalf("folded geometry=%+v", folded)
	}
	for i, c := range folded[0].Characters {
		b := c.Bounds
		got := image.Rect(int(b[0]*20), int(b[1]*40), int(b[2]*20), int(b[3]*40))
		if got != expected[i] {
			t.Fatalf("source character %d=%v want measured ink %v", i, got, expected[i])
		}
	}
}
