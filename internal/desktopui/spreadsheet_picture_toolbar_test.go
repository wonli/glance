package desktopui

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

type toolbarPictureBook struct {
	*toolbarTestBook
	pictures []document.SpreadsheetImage
}

func (b *toolbarPictureBook) SpreadsheetImages(string) ([]document.SpreadsheetImage, error) {
	return append([]document.SpreadsheetImage(nil), b.pictures...), nil
}

func (*toolbarPictureBook) SpreadsheetImageBytes(string) ([]byte, error) { return nil, nil }
func (*toolbarPictureBook) InsertSpreadsheetImage(document.SpreadsheetImagePlacement, []byte) (string, error) {
	return "inserted", nil
}
func (*toolbarPictureBook) ReplaceSpreadsheetImage(string, string, []byte) error  { return nil }
func (*toolbarPictureBook) MoveSpreadsheetImage(string, int, int, int, int) error { return nil }
func (*toolbarPictureBook) ResizeSpreadsheetImage(string, int) error              { return nil }
func (*toolbarPictureBook) DeleteSpreadsheetImage(string) error                   { return nil }

func TestSpreadsheetPictureToolbarUsesContextGroupAndCapabilities(t *testing.T) {
	state := newDocumentSessionState("pictures.xlsx")
	state.sheet = "Sheet1"
	book := &toolbarPictureBook{toolbarTestBook: &toolbarTestBook{editable: true}}
	state.book = book
	app := &App{documentSessionState: state}

	if got := toolbarCategoryLabelsForState(state); len(got) != 8 {
		t.Fatalf("without selected picture, categories=%v; want eight base tabs", got)
	}
	if !app.toolbarActionEnabled(25) {
		t.Fatal("editable workbook should enable Insert Picture")
	}
	if app.toolbarActionEnabled(26) || app.toolbarActionEnabled(27) || app.toolbarActionEnabled(28) {
		t.Fatal("picture actions should be disabled without a selected image")
	}

	book.pictures = []document.SpreadsheetImage{{ID: "pic-1", Sheet: "Sheet1", Cell: "A1", Row: 1, Column: 1, WidthPx: 80, HeightPx: 40, Editable: true}}
	state.gridPictures, state.gridPicturesSheet, state.selectedGridPictureID = book.pictures, "Sheet1", "pic-1"
	if got := toolbarCategoryLabelsForState(state); len(got) != 9 || got[8] != "图片" {
		t.Fatalf("selected picture categories=%v; want contextual Picture group", got)
	}
	for _, slot := range []int{26, 27, 28} {
		if !app.toolbarActionEnabled(slot) {
			t.Errorf("selected editable picture should enable action slot %d", slot)
		}
	}
	if got := toolbarActionsForTab(toolbarTabPicture); len(got) != 3 || got[0].slot != 26 || got[1].slot != 27 || got[2].slot != 28 {
		t.Fatalf("picture contextual actions=%+v; want replace/delete/size", got)
	}
	state.toolbarActionList.List.Position.First = 5
	state.toolbarTab = toolbarTabHome
	setSelectedGridPicture(state, "pic-1")
	if state.toolbarTab != toolbarTabPicture || state.toolbarActionList.List.Position.First != 0 {
		t.Fatalf("selecting picture left contextual Ribbon at tab=%d position=%d", state.toolbarTab, state.toolbarActionList.List.Position.First)
	}
	if _, ok := toolbarIconData("square-plus"); !ok {
		t.Fatal("expected shared Ribbon's add-image glyph for picture insertion")
	}

	state.gridPictures[0].Editable = false
	if app.toolbarActionEnabled(26) || app.toolbarActionEnabled(27) || app.toolbarActionEnabled(28) {
		t.Fatal("read-only picture should not enable mutation actions")
	}
}

func TestNearbyFreeGridPictureAnchorAvoidsExistingAnchors(t *testing.T) {
	pictures := []document.SpreadsheetImage{{Cell: "B3", Row: 3, Column: 2}, {Cell: "B4", Row: 4, Column: 2}, {Cell: "C3", Row: 3, Column: 3}}
	row, column := nearbyFreeGridPictureAnchor(pictures, 3, 2)
	if row != 4 || column != 3 {
		t.Fatalf("free anchor=(%d,%d); want (4,3)", row, column)
	}
}

func TestLoadSpreadsheetImageFileValidatesFormatAndBounds(t *testing.T) {
	directory := t.TempDir()
	valid := filepath.Join(directory, "valid.png")
	if err := os.WriteFile(valid, pngImageBytes(t, image.NewNRGBA(image.Rect(0, 0, 2, 3))), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSpreadsheetImageFile(valid)
	if err != nil || loaded.mediaType != "image/png" || len(loaded.data) == 0 {
		t.Fatalf("valid PNG loaded as %#v, error=%v", loaded, err)
	}
	misnamed := filepath.Join(directory, "misnamed.jpg")
	if err := os.WriteFile(misnamed, loaded.data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSpreadsheetImageFile(misnamed); err == nil {
		t.Fatal("extension and image format mismatch was accepted")
	}
	corrupt := filepath.Join(directory, "corrupt.png")
	if err := os.WriteFile(corrupt, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSpreadsheetImageFile(corrupt); err == nil {
		t.Fatal("corrupt image was accepted")
	}
	width, height, err := spreadsheetPictureInitialSize(pngImageBytes(t, image.NewNRGBA(image.Rect(0, 0, 1000, 500))))
	if err != nil || width != 240 || height != 120 {
		t.Fatalf("initial picture size=%dx%d err=%v; want aspect-ratio-preserving 240x120", width, height, err)
	}
}

func TestSpreadsheetPicturePickerRejectsStaleGeneration(t *testing.T) {
	book := &toolbarPictureBook{toolbarTestBook: &toolbarTestBook{editable: true}}
	state := newDocumentSessionState("pictures.xlsx")
	state.book, state.sheet = book, "Sheet1"
	app := &App{documentSessionState: state, documentGeneration: 5}
	app.xlsxImagePickerRunning = true
	app.handleSpreadsheetImagePickerResult(spreadsheetImagePickerResult{state: state, generation: 4, sheet: "Sheet1", operation: spreadsheetImageInsert, path: "stale.png"})
	if app.xlsxImagePickerRunning || book.Dirty() {
		t.Fatal("stale workbook generation changed picker state or dirtied the workbook")
	}
}

func TestSpreadsheetPicturePickerIsAsyncAndStaleSheetDoesNotMutate(t *testing.T) {
	book := &toolbarPictureBook{toolbarTestBook: &toolbarTestBook{editable: true}}
	state := newDocumentSessionState("pictures.xlsx")
	state.book, state.sheet = book, "Sheet1"
	state.selection.Active = CellPosition{Row: 1, Column: 1}
	state.selectionSet = true
	started, release := make(chan struct{}), make(chan struct{})
	app := &App{
		documentSessionState:   state,
		documentGeneration:     9,
		xlsxImagePickerResults: make(chan spreadsheetImagePickerResult, 1),
		gridPictureCommands:    make(chan gridPictureCommandResult, 1),
		shutdown:               make(chan struct{}),
		xlsxImageFilePicker: func() (string, error) {
			close(started)
			<-release
			return "", nil
		},
	}
	app.startSpreadsheetImagePicker(spreadsheetImageInsert)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("spreadsheet image picker did not start asynchronously")
	}
	if !app.xlsxImagePickerRunning {
		t.Fatal("picker running state should remain active until selection completes")
	}
	state.sheet = "OtherSheet"
	close(release)
	select {
	case result := <-app.xlsxImagePickerResults:
		app.handleSpreadsheetImagePickerResult(result)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for picker result")
	}
	if book.Dirty() || len(app.gridPictureCommands) != 0 {
		t.Fatal("stale sheet result mutated or queued a workbook command")
	}
}

func TestSpreadsheetPicturePickerCancellationDoesNotDirtyWorkbook(t *testing.T) {
	book := &toolbarPictureBook{toolbarTestBook: &toolbarTestBook{editable: true}}
	state := newDocumentSessionState("pictures.xlsx")
	state.book, state.sheet = book, "Sheet1"
	app := &App{
		documentSessionState:   state,
		documentGeneration:     2,
		xlsxImagePickerResults: make(chan spreadsheetImagePickerResult, 1),
		shutdown:               make(chan struct{}),
		xlsxImageFilePicker:    func() (string, error) { return "", nil },
	}
	app.startSpreadsheetImagePicker(spreadsheetImageInsert)
	select {
	case result := <-app.xlsxImagePickerResults:
		app.handleSpreadsheetImagePickerResult(result)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for canceled picker")
	}
	if app.xlsxImagePickerRunning || app.status != "已取消选择图片" || book.Dirty() {
		t.Fatalf("cancel changed state: running=%v status=%q dirty=%v", app.xlsxImagePickerRunning, app.status, book.Dirty())
	}
}

func TestSpreadsheetPicturePickerInsertSavesAndReopens(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "picker-insert.xlsx")
	book := xlsx.NewBlank()
	if err := book.SaveAsDocument(path); err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	imagePath := filepath.Join(directory, "picked.png")
	data := pngImageBytes(t, image.NewNRGBA(image.Rect(0, 0, 4, 2)))
	if err := os.WriteFile(imagePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState(path)
	state.book, state.sheet = book, book.SheetNames()[0]
	state.selectionSet = true
	state.selection.Active = CellPosition{Row: 0, Column: 0}
	app := &App{
		documentSessionState:   state,
		documentGeneration:     3,
		xlsxImagePickerResults: make(chan spreadsheetImagePickerResult, 1),
		gridPictureCommands:    make(chan gridPictureCommandResult, 1),
		shutdown:               make(chan struct{}),
		xlsxImageFilePicker:    func() (string, error) { return imagePath, nil },
	}
	app.startSpreadsheetImagePicker(spreadsheetImageInsert)
	var picker spreadsheetImagePickerResult
	select {
	case picker = <-app.xlsxImagePickerResults:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for selected image")
	}
	app.handleSpreadsheetImagePickerResult(picker)
	var command gridPictureCommandResult
	select {
	case command = <-app.gridPictureCommands:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for picture insertion")
	}
	app.applyGridPictureCommand(command)
	if !book.Dirty() || state.selectedGridPictureID == "" || len(state.gridPictures) != 1 || state.gridPictures[0].Cell != "A1" {
		t.Fatalf("picker insertion did not select and dirty the workbook: dirty=%v selected=%q images=%#v", book.Dirty(), state.selectedGridPictureID, state.gridPictures)
	}
	if err := book.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := xlsx.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	images, err := reopened.SpreadsheetImages(state.sheet)
	if err != nil || len(images) != 1 || images[0].Cell != "A1" || images[0].WidthPx != 4 || images[0].HeightPx != 2 {
		t.Fatalf("reopened picker image metadata=%#v err=%v", images, err)
	}
	got, err := reopened.SpreadsheetImageBytes(images[0].ID)
	if err != nil || string(got) != string(data) {
		t.Fatalf("reopened picker image bytes changed: err=%v", err)
	}
}
