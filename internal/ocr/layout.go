package ocr

import (
	"encoding/json"
	"errors"
	"math"
)

type LayoutBlock struct {
	ID     int        `json:"block_id"`
	Label  string     `json:"block_label"`
	Text   string     `json:"block_content"`
	Bounds [4]float64 `json:"block_bbox"`
	Order  *int       `json:"block_order"`
}
type Layout struct {
	Width  int           `json:"width"`
	Height int           `json:"height"`
	Blocks []LayoutBlock `json:"parsing_res_list"`
}

// Layout decodes provider coordinates in input-image pixels. Empty image blocks
// are retained: their absence of recognized text is not evidence of a blank page.
func (p Page) Layout() (Layout, error) {
	var raw struct {
		Result *Layout `json:"prunedResult"`
	}
	if err := json.Unmarshal(p.Raw, &raw); err != nil || raw.Result == nil {
		return Layout{}, errors.New("ocr: missing structured layout")
	}
	l := *raw.Result
	if l.Width <= 0 || l.Height <= 0 {
		return Layout{}, errors.New("ocr: invalid layout dimensions")
	}
	for _, b := range l.Blocks {
		for _, v := range b.Bounds {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return Layout{}, errors.New("ocr: invalid block coordinates")
			}
		}
		if b.Bounds[2] < b.Bounds[0] || b.Bounds[3] < b.Bounds[1] {
			return Layout{}, errors.New("ocr: inverted block coordinates")
		}
	}
	return l, nil
}
