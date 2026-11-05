package ocr

import (
	"encoding/json"
	"errors"
)

// Windows OCR measures words. Keep a multi-rune word's shared rectangle so it
// participates in text alignment, but the existing duplicate-box guard rejects
// it as character geometry.
func decodeWindowsOCRResult(data []byte) ([]detectedTextLine, error) {
	var result struct {
		Width, Height float64
		Lines         [][]struct {
			Text                string
			X, Y, Width, Height float64
		}
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if result.Width <= 0 || result.Height <= 0 {
		return nil, errors.New("ocr: invalid Windows OCR image dimensions")
	}
	var lines []detectedTextLine
	for _, words := range result.Lines {
		line := detectedTextLine{Bounds: [4]float64{1, 1, 0, 0}}
		for _, word := range words {
			box := [4]float64{word.X / result.Width, word.Y / result.Height, (word.X + word.Width) / result.Width, (word.Y + word.Height) / result.Height}
			for _, r := range word.Text {
				line.Characters = append(line.Characters, detectedCharacter{Text: string(r), Bounds: box})
			}
			line.Bounds[0] = min(line.Bounds[0], box[0])
			line.Bounds[1] = min(line.Bounds[1], box[1])
			line.Bounds[2] = max(line.Bounds[2], box[2])
			line.Bounds[3] = max(line.Bounds[3], box[3])
		}
		if len(line.Characters) > 0 {
			lines = append(lines, line)
		}
	}
	return lines, nil
}
