package xlsx

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestSpreadsheetImagesProjectsAnchorsAndReadsBytes(t *testing.T) {
	first := pictureFixturePNG(t, 40, 20, color.NRGBA{R: 30, G: 120, B: 210, A: 255})
	second := pictureFixturePNG(t, 24, 12, color.NRGBA{R: 220, G: 130, B: 40, A: 255})
	f := excelize.NewFile()
	if _, err := f.NewSheet("Other"); err != nil {
		t.Fatal(err)
	}
	addPictureFixture(t, f, "Sheet1", "B3", first, &excelize.GraphicOptions{Name: "overlap one", AltText: "first", OffsetX: 7, OffsetY: 9, ScaleX: 0.5, ScaleY: 0.5})
	addPictureFixture(t, f, "Sheet1", "B3", second, &excelize.GraphicOptions{Name: "overlap two", AltText: "second", OffsetX: 30, OffsetY: 18, ScaleX: 1, ScaleY: 1})
	addPictureFixture(t, f, "Sheet1", "F7", second, &excelize.GraphicOptions{Name: "single", AltText: "editable", OffsetX: 4, OffsetY: 6, ScaleX: 1.5, ScaleY: 1.5, Positioning: "oneCell"})
	addPictureFixture(t, f, "Other", "C5", first, &excelize.GraphicOptions{Name: "other sheet", ScaleX: 1, ScaleY: 1})
	path := filepath.Join(t.TempDir(), "pictures.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	book, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	images, err := book.SpreadsheetImages("Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 3 {
		t.Fatalf("Sheet1 picture count = %d; want 3", len(images))
	}
	if images[0].ID == "" || images[1].ID == "" || images[2].ID == "" || images[0].ID == images[1].ID || images[0].ID == images[2].ID || images[1].ID == images[2].ID {
		t.Fatalf("picture IDs must be non-empty and unique: %#v", images)
	}
	if images[0].ID != imagesAgainID(t, book, "Sheet1", 0) {
		t.Fatal("picture ID changed between reads without a mutation")
	}
	if images[0].Cell != "B3" || images[0].OffsetX != 7 || images[0].OffsetY != 9 || images[0].WidthPx != 20 || images[0].HeightPx != 10 || images[0].Name != "overlap one" || images[0].AltText != "first" {
		t.Fatalf("first picture metadata = %#v", images[0])
	}
	if images[0].MediaType != "image/png" || images[0].Editable || images[1].Editable || images[0].ReadOnlyReason == "" || images[1].ReadOnlyReason == "" {
		t.Fatalf("same-anchor picture group must be read-only: %#v", images[:2])
	}
	if images[2].Cell != "F7" || images[2].OffsetX != 4 || images[2].OffsetY != 6 || images[2].WidthPx != 36 || images[2].HeightPx != 18 || !images[2].Editable {
		t.Fatalf("single picture metadata = %#v", images[2])
	}
	gotBytes, err := book.SpreadsheetImageBytes(images[2].ID)
	if err != nil || !bytes.Equal(gotBytes, second) {
		t.Fatalf("single picture bytes match=%v err=%v", bytes.Equal(gotBytes, second), err)
	}
	otherImages, err := book.SpreadsheetImages("Other")
	if err != nil || len(otherImages) != 1 || otherImages[0].Cell != "C5" || otherImages[0].Editable != true {
		t.Fatalf("Other sheet pictures = %#v, err=%v", otherImages, err)
	}
}

func TestSpreadsheetImageProjectionMarksUnsupportedAndInvalidPicturesReadOnly(t *testing.T) {
	valid := pictureFixturePNG(t, 16, 8, color.NRGBA{R: 60, G: 80, B: 100, A: 255})
	unsupported, err := spreadsheetImageView("svg", "Sheet1", "A1", 1, 1, excelize.Picture{Extension: ".svg", File: []byte("<svg/>")}, 1)
	if err != nil || unsupported.Editable || unsupported.ReadOnlyReason == "" {
		t.Fatalf("unsupported SVG projection = %#v, err=%v", unsupported, err)
	}
	corrupt, err := spreadsheetImageView("corrupt", "Sheet1", "A1", 1, 1, excelize.Picture{Extension: ".png", File: []byte("bad PNG")}, 1)
	if err != nil || corrupt.Editable || corrupt.ReadOnlyReason == "" {
		t.Fatalf("corrupt PNG projection = %#v, err=%v", corrupt, err)
	}
	inCell, err := spreadsheetImageView("in-cell", "Sheet1", "A1", 1, 1, excelize.Picture{Extension: ".png", File: valid, InsertType: excelize.PictureInsertTypePlaceInCell}, 1)
	if err != nil || inCell.Editable || inCell.ReadOnlyReason == "" {
		t.Fatalf("in-cell image projection = %#v, err=%v", inCell, err)
	}
	invalidScale, err := spreadsheetImageView("bad-scale", "Sheet1", "A1", 1, 1, excelize.Picture{Extension: ".png", File: valid, Format: &excelize.GraphicOptions{ScaleX: math.NaN(), ScaleY: 1}}, 1)
	if err != nil || invalidScale.Editable || invalidScale.ReadOnlyReason == "" {
		t.Fatalf("invalid scale projection = %#v, err=%v", invalidScale, err)
	}
}

func imagesAgainID(t *testing.T, book *Workbook, sheet string, index int) string {
	t.Helper()
	images, err := book.SpreadsheetImages(sheet)
	if err != nil {
		t.Fatal(err)
	}
	return images[index].ID
}

func addPictureFixture(t *testing.T, file *excelize.File, sheet, cell string, data []byte, options *excelize.GraphicOptions) {
	t.Helper()
	if err := file.AddPictureFromBytes(sheet, cell, &excelize.Picture{Extension: ".png", File: data, Format: options}); err != nil {
		t.Fatal(err)
	}
}

func pictureFixturePNG(t *testing.T, width, height int, fill color.NRGBA) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixels.SetNRGBA(x, y, fill)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}
