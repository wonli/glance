package ocr

import "testing"

func TestWindowsOCRWordsKeepMeasuredGeometry(t *testing.T) {
	data := []byte(`{"width":200,"height":100,"lines":[[{"text":"甲","x":20,"y":10,"width":20,"height":30},{"text":"乙丙","x":40,"y":10,"width":40,"height":30},{"text":"丁","x":80,"y":10,"width":20,"height":30}]]}`)
	lines, err := decodeWindowsOCRResult(data)
	if err != nil || len(lines) != 1 || len(lines[0].Characters) != 4 {
		t.Fatalf("lines=%+v err=%v", lines, err)
	}
	if got := lines[0].Characters[0].Bounds; got != [4]float64{.1, .1, .2, .4} {
		t.Fatalf("normalized box=%v", got)
	}
	pairs := alignCharacterGeometry("甲乙丙丁", lines[0].Characters)
	if len(pairs) != 2 || pairs[0].offset != 0 || pairs[1].offset != 3 {
		t.Fatalf("multi-character word must align but never become single-character boxes: %+v", pairs)
	}
}

func TestWindowsOCRRejectsInvalidOutput(t *testing.T) {
	for _, data := range []string{`{}`, `{"width":0,"height":100}`, `not json`} {
		if _, err := decodeWindowsOCRResult([]byte(data)); err == nil {
			t.Fatalf("accepted invalid OCR result %q", data)
		}
	}
}
