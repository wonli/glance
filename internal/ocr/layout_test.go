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
