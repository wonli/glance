package ocr

import (
	"encoding/json"
	"testing"
)

func TestLayoutPreservesVerticalAndImageBlocks(t *testing.T) {
	page := Page{Number: 1, Raw: json.RawMessage(`{"prunedResult":{"width":100,"height":200,"parsing_res_list":[{"block_id":9,"block_label":"vertical_text","block_content":"唐诗汇评","block_bbox":[60,10,90,180],"block_order":1},{"block_id":10,"block_label":"image","block_content":"","block_bbox":[0,0,40,200],"block_order":null}]}}`)}
	layout, err := page.Layout()
	if err != nil {
		t.Fatal(err)
	}
	if layout.Width != 100 || len(layout.Blocks) != 2 || layout.Blocks[0].Label != "vertical_text" || layout.Blocks[0].Text != "唐诗汇评" || layout.Blocks[1].Order != nil {
		t.Fatalf("bad layout: %+v", layout)
	}
}

func TestLayoutCharacterGeometryValidationAndRawPreservation(t *testing.T) {
	page := Page{Number: 1, Markdown: "甲乙", Raw: json.RawMessage(`{"extra":{"keep":true},"prunedResult":{"width":100,"height":100,"parsing_res_list":[{"block_id":7,"block_content":"甲乙","block_bbox":[10,10,80,80]}]}}`)}
	layout, err := page.Layout()
	if err != nil {
		t.Fatal(err)
	}
	layout.CharacterVersion = CharacterGeometryVersion
	layout.Characters = []CharacterBox{{BlockID: 7, Offset: 1, Text: "乙", Bounds: [4]float64{30, 10, 50, 30}}}
	enriched, err := page.WithLayout(layout)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = enriched.Layout(); err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err = json.Unmarshal(enriched.Raw, &raw); err != nil || len(raw["extra"]) == 0 || enriched.Markdown != page.Markdown {
		t.Fatal("geometry enrichment replaced non-layout recognition data")
	}
	for _, bad := range []CharacterBox{{BlockID: 7, Offset: 5, Text: "乙", Bounds: [4]float64{30, 10, 50, 30}}, {BlockID: 7, Offset: 1, Text: "甲", Bounds: [4]float64{30, 10, 50, 30}}, {BlockID: 7, Offset: 1, Text: "乙", Bounds: [4]float64{30, 10, 150, 30}}} {
		layout.Characters = []CharacterBox{bad}
		invalid, err := page.WithLayout(layout)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = invalid.Layout(); err == nil {
			t.Fatalf("invalid glyph accepted: %+v", bad)
		}
	}
}
