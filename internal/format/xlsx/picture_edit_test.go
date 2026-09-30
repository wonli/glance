package xlsx

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"path/filepath"
	"testing"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

func TestSpreadsheetImageEditorRoundTripsHistoryAndPreservesWorkbook(t *testing.T) {
	book := newPictureEditingWorkbook(t)
	defer func() {
		if book != nil {
			_ = book.Close()
		}
	}()
	editor, ok := any(book).(document.SpreadsheetImageEditor)
	if !ok {
		t.Fatal("XLSX workbook does not implement SpreadsheetImageEditor")
	}
	assertWorkbookContentPreserved(t, book)

	images := spreadsheetImagesForTest(t, book)
	var ambiguousID string
	for _, image := range images {
		if image.Cell == "B2" {
			ambiguousID = image.ID
			break
		}
	}
	if ambiguousID == "" {
		t.Fatal("duplicate-anchor fixture is missing")
	}
	if err := editor.DeleteSpreadsheetImage(ambiguousID); err == nil {
		t.Fatal("delete accepted an ambiguous multi-picture anchor")
	}
	if err := editor.ReplaceSpreadsheetImage(ambiguousID, "image/png", pictureFixturePNG(t, 4, 4, color.NRGBA{G: 255, A: 255})); err == nil {
		t.Fatal("replace accepted an ambiguous multi-picture anchor")
	}
	if book.Dirty() {
		t.Fatal("rejected ambiguous edits marked workbook dirty")
	}
	if got := len(spreadsheetImagesForTest(t, book)); got != 3 {
		t.Fatalf("pictures after rejected edits = %d; want 3", got)
	}

	insertedPNG := pictureFixturePNG(t, 40, 20, color.NRGBA{R: 40, G: 120, B: 220, A: 255})
	imageID, err := editor.InsertSpreadsheetImage(document.SpreadsheetImagePlacement{
		Sheet: "Sheet1", Row: 5, Column: 4, OffsetX: 8, OffsetY: 11,
		WidthPx: 80, HeightPx: 40, MediaType: "image/png", Name: "inserted", AltText: "new image",
	}, insertedPNG)
	if err != nil || imageID == "" {
		t.Fatalf("insert picture: id=%q err=%v", imageID, err)
	}
	if !book.Dirty() {
		t.Fatal("successful insert did not mark workbook dirty")
	}
	undone, err := book.Undo()
	if err != nil || !undone {
		t.Fatalf("undo insert = %v, err=%v", undone, err)
	}
	if findSpreadsheetImageOptional(t, book, "D5") != nil || book.Dirty() {
		t.Fatal("undo insert did not restore clean picture state")
	}
	redone, err := book.Redo()
	if err != nil || !redone || !book.Dirty() {
		t.Fatalf("redo insert = %v, dirty=%v, err=%v", redone, book.Dirty(), err)
	}
	book = persistAndReopenPictureWorkbook(t, book)
	inserted := findSpreadsheetImage(t, book, "D5")
	if inserted.WidthPx != 80 || inserted.HeightPx != 40 || inserted.OffsetX != 8 || inserted.OffsetY != 11 || inserted.Name != "inserted" || inserted.AltText != "new image" {
		t.Fatalf("inserted picture metadata after reopen = %#v", inserted)
	}
	if got := spreadsheetImageBytesForTest(t, book, inserted.ID); !bytes.Equal(got, insertedPNG) {
		t.Fatal("inserted PNG bytes changed after save/reopen")
	}
	assertWorkbookContentPreserved(t, book)

	replacementJPEG := pictureFixtureJPEG(t, 30, 15, color.NRGBA{R: 220, G: 80, B: 40, A: 255})
	if err := editorForTest(t, book).ReplaceSpreadsheetImage(inserted.ID, "image/jpeg", replacementJPEG); err != nil {
		t.Fatalf("replace picture: %v", err)
	}
	undone, err = book.Undo()
	if err != nil || !undone || findSpreadsheetImage(t, book, "D5").MediaType != "image/png" {
		t.Fatalf("undo replace = %v, err=%v", undone, err)
	}
	redone, err = book.Redo()
	if err != nil || !redone || findSpreadsheetImage(t, book, "D5").MediaType != "image/jpeg" {
		t.Fatalf("redo replace = %v, err=%v", redone, err)
	}
	book = persistAndReopenPictureWorkbook(t, book)
	inserted = findSpreadsheetImage(t, book, "D5")
	if inserted.MediaType != "image/jpeg" || inserted.WidthPx != 80 || inserted.HeightPx != 40 {
		t.Fatalf("replacement metadata after reopen = %#v", inserted)
	}
	if got := spreadsheetImageBytesForTest(t, book, inserted.ID); !bytes.Equal(got, replacementJPEG) {
		t.Fatal("replacement JPEG bytes changed after save/reopen")
	}
	if err := book.Save(); err != nil {
		t.Fatalf("save replacement baseline: %v", err)
	}
	if err := editorForTest(t, book).ReplaceSpreadsheetImage(inserted.ID, "image/jpeg", replacementJPEG); err != nil || book.Dirty() {
		t.Fatalf("identical replacement should be a no-op: dirty=%v err=%v", book.Dirty(), err)
	}

	if err := editorForTest(t, book).MoveSpreadsheetImage(inserted.ID, 7, 6, 13, 17); err != nil {
		t.Fatalf("move picture: %v", err)
	}
	undone, err = book.Undo()
	if err != nil || !undone || findSpreadsheetImage(t, book, "D5").MediaType != "image/jpeg" {
		t.Fatalf("undo move = %v, err=%v", undone, err)
	}
	redone, err = book.Redo()
	if err != nil || !redone || findSpreadsheetImage(t, book, "F7").OffsetX != 13 {
		t.Fatalf("redo move = %v, err=%v", redone, err)
	}
	book = persistAndReopenPictureWorkbook(t, book)
	inserted = findSpreadsheetImage(t, book, "F7")
	if inserted.Row != 7 || inserted.Column != 6 || inserted.OffsetX != 13 || inserted.OffsetY != 17 {
		t.Fatalf("moved picture metadata after reopen = %#v", inserted)
	}

	if err := editorForTest(t, book).ResizeSpreadsheetImage(inserted.ID, 90); err != nil {
		t.Fatalf("resize picture: %v", err)
	}
	undone, err = book.Undo()
	if err != nil || !undone || findSpreadsheetImage(t, book, "F7").WidthPx != 80 {
		t.Fatalf("undo resize = %v, err=%v", undone, err)
	}
	redone, err = book.Redo()
	if err != nil || !redone || findSpreadsheetImage(t, book, "F7").WidthPx != 90 {
		t.Fatalf("redo resize = %v, err=%v", redone, err)
	}
	book = persistAndReopenPictureWorkbook(t, book)
	inserted = findSpreadsheetImage(t, book, "F7")
	if inserted.WidthPx != 90 || inserted.HeightPx != 45 {
		t.Fatalf("resized picture should preserve 2:1 ratio, got %dx%d", inserted.WidthPx, inserted.HeightPx)
	}
	assertWorkbookContentPreserved(t, book)
	if err := book.Save(); err != nil {
		t.Fatalf("save resize baseline: %v", err)
	}
	if err := editorForTest(t, book).ResizeSpreadsheetImage(inserted.ID, inserted.WidthPx); err != nil || book.Dirty() {
		t.Fatalf("same-width resize should be a no-op: dirty=%v err=%v", book.Dirty(), err)
	}

	if err := editorForTest(t, book).DeleteSpreadsheetImage(inserted.ID); err != nil {
		t.Fatalf("delete picture: %v", err)
	}
	undone, err = book.Undo()
	if err != nil || !undone || findSpreadsheetImageOptional(t, book, "F7") == nil {
		t.Fatalf("undo delete = %v, err=%v", undone, err)
	}
	redone, err = book.Redo()
	if err != nil || !redone || findSpreadsheetImageOptional(t, book, "F7") != nil {
		t.Fatalf("redo delete = %v, err=%v", redone, err)
	}
	book = persistAndReopenPictureWorkbook(t, book)
	if findSpreadsheetImageOptional(t, book, "F7") != nil {
		t.Fatal("deleted picture returned after save/reopen")
	}
	images = spreadsheetImagesForTest(t, book)
	if len(images) != 3 {
		t.Fatalf("remaining pictures = %d; want original 3", len(images))
	}
	assertWorkbookContentPreserved(t, book)
}

func TestSpreadsheetImageEditorRejectsInvalidRequestsWithoutDirtyingWorkbook(t *testing.T) {
	book := newPictureEditingWorkbook(t)
	defer book.Close()
	editor := editorForTest(t, book)
	valid := pictureFixturePNG(t, 8, 4, color.NRGBA{B: 255, A: 255})
	cases := []struct {
		name string
		run  func() error
	}{
		{"unknown worksheet", func() error {
			_, err := editor.InsertSpreadsheetImage(document.SpreadsheetImagePlacement{Sheet: "Missing", Row: 1, Column: 1, WidthPx: 8, HeightPx: 4, MediaType: "image/png"}, valid)
			return err
		}},
		{"out of bounds anchor", func() error {
			_, err := editor.InsertSpreadsheetImage(document.SpreadsheetImagePlacement{Sheet: "Sheet1", Row: maxXLSXRows + 1, Column: 1, WidthPx: 8, HeightPx: 4, MediaType: "image/png"}, valid)
			return err
		}},
		{"invalid dimensions", func() error {
			_, err := editor.InsertSpreadsheetImage(document.SpreadsheetImagePlacement{Sheet: "Sheet1", Row: 1, Column: 1, WidthPx: 0, HeightPx: 4, MediaType: "image/png"}, valid)
			return err
		}},
		{"MIME mismatch", func() error {
			_, err := editor.InsertSpreadsheetImage(document.SpreadsheetImagePlacement{Sheet: "Sheet1", Row: 1, Column: 1, WidthPx: 8, HeightPx: 4, MediaType: "image/jpeg"}, valid)
			return err
		}},
		{"corrupt bytes", func() error {
			_, err := editor.InsertSpreadsheetImage(document.SpreadsheetImagePlacement{Sheet: "Sheet1", Row: 1, Column: 1, WidthPx: 8, HeightPx: 4, MediaType: "image/png"}, []byte("not an image"))
			return err
		}},
		{"occupied anchor", func() error {
			_, err := editor.InsertSpreadsheetImage(document.SpreadsheetImagePlacement{Sheet: "Sheet1", Row: 2, Column: 2, WidthPx: 8, HeightPx: 4, MediaType: "image/png"}, valid)
			return err
		}},
		{"stale id", func() error { return editor.DeleteSpreadsheetImage("stale-picture-id") }},
		{"invalid move", func() error { return editor.MoveSpreadsheetImage("stale-picture-id", 0, 1, 0, 0) }},
		{"invalid resize", func() error { return editor.ResizeSpreadsheetImage("stale-picture-id", 0) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Fatal("invalid picture edit unexpectedly succeeded")
			}
			if book.Dirty() {
				t.Fatal("rejected picture edit marked workbook dirty")
			}
		})
	}
	if got := len(spreadsheetImagesForTest(t, book)); got != 3 {
		t.Fatalf("pictures after rejected edits = %d; want 3", got)
	}
}

func editorForTest(t *testing.T, book *Workbook) document.SpreadsheetImageEditor {
	t.Helper()
	editor, ok := any(book).(document.SpreadsheetImageEditor)
	if !ok {
		t.Fatal("XLSX workbook does not implement SpreadsheetImageEditor")
	}
	return editor
}

func newPictureEditingWorkbook(t *testing.T) *Workbook {
	t.Helper()
	first := pictureFixturePNG(t, 20, 10, color.NRGBA{R: 30, G: 90, B: 160, A: 255})
	second := pictureFixturePNG(t, 12, 6, color.NRGBA{G: 170, B: 40, A: 255})
	file := excelize.NewFile()
	if err := file.SetCellValue("Sheet1", "A1", "preserve me"); err != nil {
		t.Fatal(err)
	}
	if err := file.SetCellFormula("Sheet1", "C2", "=SUM(1,2)"); err != nil {
		t.Fatal(err)
	}
	style, err := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"#DDEEFF"}, Pattern: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := file.SetCellStyle("Sheet1", "A1", "A1", style); err != nil {
		t.Fatal(err)
	}
	if err := file.MergeCell("Sheet1", "D1", "E1"); err != nil {
		t.Fatal(err)
	}
	if err := file.SetRowHeight("Sheet1", 3, 31); err != nil {
		t.Fatal(err)
	}
	if err := file.SetColWidth("Sheet1", "H", "H", 22); err != nil {
		t.Fatal(err)
	}
	addPictureFixture(t, file, "Sheet1", "B2", first, &excelize.GraphicOptions{ScaleX: 1, ScaleY: 1})
	addPictureFixture(t, file, "Sheet1", "B2", second, &excelize.GraphicOptions{ScaleX: 1, ScaleY: 1})
	addPictureFixture(t, file, "Sheet1", "J10", first, &excelize.GraphicOptions{Name: "unrelated", AltText: "preserve drawing", ScaleX: 0.5, ScaleY: 0.5})
	path := filepath.Join(t.TempDir(), "fixture.xlsx")
	if err := file.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	book, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return book
}

func persistAndReopenPictureWorkbook(t *testing.T, book *Workbook) *Workbook {
	t.Helper()
	path := filepath.Join(t.TempDir(), "roundtrip.xlsx")
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return reopened
}

func spreadsheetImagesForTest(t *testing.T, book *Workbook) []document.SpreadsheetImage {
	t.Helper()
	images, err := book.SpreadsheetImages("Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	return images
}

func findSpreadsheetImage(t *testing.T, book *Workbook, cell string) document.SpreadsheetImage {
	t.Helper()
	image := findSpreadsheetImageOptional(t, book, cell)
	if image == nil {
		t.Fatalf("picture at %s not found", cell)
	}
	return *image
}

func findSpreadsheetImageOptional(t *testing.T, book *Workbook, cell string) *document.SpreadsheetImage {
	t.Helper()
	for _, image := range spreadsheetImagesForTest(t, book) {
		if image.Cell == cell {
			copy := image
			return &copy
		}
	}
	return nil
}

func spreadsheetImageBytesForTest(t *testing.T, book *Workbook, id string) []byte {
	t.Helper()
	data, err := book.SpreadsheetImageBytes(id)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertWorkbookContentPreserved(t *testing.T, book *Workbook) {
	t.Helper()
	if value, err := book.Value("Sheet1", "A1"); err != nil || value != "preserve me" {
		t.Fatalf("untouched value = %q, err=%v", value, err)
	}
	if formula, err := book.file.GetCellFormula("Sheet1", "C2"); err != nil || formula != "=SUM(1,2)" {
		t.Fatalf("untouched formula = %q, err=%v", formula, err)
	}
	styleID, err := book.file.GetCellStyle("Sheet1", "A1")
	if err != nil || styleID == 0 {
		t.Fatalf("untouched cell style = %d, err=%v", styleID, err)
	}
	merges, err := book.file.GetMergeCells("Sheet1")
	if err != nil || len(merges) != 1 {
		t.Fatalf("untouched merges = %#v, err=%v", merges, err)
	}
	if height, err := book.file.GetRowHeight("Sheet1", 3); err != nil || height != 31 {
		t.Fatalf("untouched row height = %v, err=%v", height, err)
	}
	if width, err := book.file.GetColWidth("Sheet1", "H"); err != nil || width != 22 {
		t.Fatalf("untouched column width = %v, err=%v", width, err)
	}
	if image := findSpreadsheetImage(t, book, "J10"); image.Name != "unrelated" || image.AltText != "preserve drawing" {
		t.Fatalf("unrelated picture metadata changed: %#v", image)
	}
	if data := spreadsheetImageBytesForTest(t, book, findSpreadsheetImage(t, book, "J10").ID); !bytes.Equal(data, pictureFixturePNG(t, 20, 10, color.NRGBA{R: 30, G: 90, B: 160, A: 255})) {
		t.Fatal("unrelated picture bytes changed")
	}
}

func pictureFixtureJPEG(t *testing.T, width, height int, fill color.NRGBA) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixels.SetNRGBA(x, y, fill)
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, pixels, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}
