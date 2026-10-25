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
	Width            int            `json:"width"`
	Height           int            `json:"height"`
	Blocks           []LayoutBlock  `json:"parsing_res_list"`
	CharacterVersion int            `json:"character_geometry_version,omitempty"`
	Characters       []CharacterBox `json:"character_boxes,omitempty"`
}

// CharacterBox uses the same top-left input-image coordinate system as Blocks.
// Offset is a rune offset in the original block text, including whitespace.
type CharacterBox struct {
	BlockID int        `json:"block_id"`
	Offset  int        `json:"offset"`
	Text    string     `json:"text"`
	Bounds  [4]float64 `json:"bounds"`
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
	blocks := make(map[int][]rune, len(l.Blocks))
	for _, block := range l.Blocks {
		blocks[block.ID] = []rune(block.Text)
	}
	for _, character := range l.Characters {
		text, exists := blocks[character.BlockID]
		if !exists || character.Offset < 0 || character.Offset >= len(text) || character.Text != string(text[character.Offset]) {
			return Layout{}, errors.New("ocr: invalid character identity")
		}
		for _, coordinate := range character.Bounds {
			if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
				return Layout{}, errors.New("ocr: invalid character coordinates")
			}
		}
		b := character.Bounds
		if b[0] < 0 || b[1] < 0 || b[2] > float64(l.Width) || b[3] > float64(l.Height) || b[2] <= b[0] || b[3] <= b[1] {
			return Layout{}, errors.New("ocr: invalid character bounds")
		}
	}
	return l, nil
}

func (p Page) WithLayout(layout Layout) (Page, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(p.Raw, &raw); err != nil {
		return p, err
	}
	if raw == nil {
		return p, errors.New("ocr: missing page result")
	}
	encoded, err := json.Marshal(layout)
	if err != nil {
		return p, err
	}
	raw["prunedResult"] = encoded
	p.Raw, err = json.Marshal(raw)
	return p, err
}
