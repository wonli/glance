package desktopui

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestLoadWordImageFileRejectsUnsupportedCorruptAndOversizedImages(t *testing.T) {
	dir := t.TempDir()
	unsupported := filepath.Join(dir, "image.svg")
	if err := os.WriteFile(unsupported, []byte("<svg/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWordImageFile(unsupported); err == nil || !strings.Contains(err.Error(), "PNG/JPEG") {
		t.Fatalf("unsupported image error = %v", err)
	}

	corrupt := filepath.Join(dir, "corrupt.png")
	if err := os.WriteFile(corrupt, []byte("not a PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWordImageFile(corrupt); err == nil {
		t.Fatal("corrupt PNG was accepted")
	}

	oversized := filepath.Join(dir, "oversized.png")
	if err := os.WriteFile(oversized, pngConfigHeader(wordPreviewPixelLimit+1, 1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWordImageFile(oversized); err == nil || !strings.Contains(err.Error(), "pixel limit") {
		t.Fatalf("oversized image error = %v", err)
	}
}

func TestWordImagePickerCancellationAndStaleGenerationDoNotMutateDocument(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)
	state := newDocumentSessionState("sample.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{documentSessionState: state, documentGeneration: 4, wordCommandResults: make(chan wordCommandResult, 1)}
	app.wordImagePickerRunning = true
	app.handleWordImagePickerResult(wordImagePickerResult{state: state, generation: 4, operation: wordImageInsert})
	if app.wordImagePickerRunning || app.status != "已取消选择图片" || word.Dirty() {
		t.Fatalf("picker cancellation state: running=%v status=%q dirty=%v", app.wordImagePickerRunning, app.status, word.Dirty())
	}

	before := len(word.WordBlocks())
	app.status = "保持当前状态"
	app.wordImagePickerRunning = true
	app.documentGeneration++
	app.handleWordImagePickerResult(wordImagePickerResult{state: state, generation: 4, operation: wordImageInsert, path: "stale.png", data: pngImageBytes(t, image.NewNRGBA(image.Rect(0, 0, 1, 1))), mediaType: "image/png"})
	if app.wordImagePickerRunning || app.status != "保持当前状态" || word.Dirty() || len(word.WordBlocks()) != before {
		t.Fatal("stale image picker result changed the active document")
	}
}

func TestWordImagePickerRunsOutsideLayoutAndCanCancel(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("sample.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = state.wordBlocks[0].Paragraph.ID
	started := make(chan struct{})
	release := make(chan struct{})
	app := &App{
		documentSessionState:   state,
		documentGeneration:     1,
		wordImagePickerResults: make(chan wordImagePickerResult, 1),
		wordCommandResults:     make(chan wordCommandResult, 1),
		shutdown:               make(chan struct{}),
		wordImageFilePicker: func() (string, error) {
			close(started)
			<-release
			return "", nil
		},
	}
	app.startWordImagePicker(wordImageInsert)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("native image picker was not started asynchronously")
	}
	if !app.wordImagePickerRunning {
		t.Fatal("image picker should remain active while its asynchronous chooser is open")
	}
	close(release)
	select {
	case result := <-app.wordImagePickerResults:
		app.wordImagePickerResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for canceled asynchronous image picker")
	}
	if app.wordImagePickerRunning || app.status != "已取消选择图片" {
		t.Fatalf("canceled asynchronous picker state: running=%v status=%q", app.wordImagePickerRunning, app.status)
	}
}

func TestWordSampleRoundTripsTextAndImageWithoutChangingSource(t *testing.T) {
	source := os.Getenv("GLANCE_OFFICE_TEST_DOCX")
	if source == "" {
		t.Skip("set GLANCE_OFFICE_TEST_DOCX to run the external Word sample regression")
	}
	originalBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	tempPath := filepath.Join(t.TempDir(), "sample-copy.docx")
	if err := os.WriteFile(tempPath, originalBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	beforeParts := readDocxPartMap(t, tempPath)
	word, err := docx.Open(tempPath)
	if err != nil {
		t.Fatal(err)
	}
	blocks := word.WordBlocks()
	paragraphCount, tableCount := 0, 0
	missingImageSlots := 0
	var images []document.WordImage
	for _, block := range blocks {
		switch block.Kind {
		case document.WordParagraphBlock:
			paragraphCount++
		case document.WordTableBlock:
			tableCount++
		}
		for _, paragraph := range wordParagraphsInBlock(block) {
			for _, inline := range paragraph.Inlines {
				if inline.Image != nil {
					images = append(images, *inline.Image)
				}
				if inline.Unsupported != nil && strings.Contains(inline.Unsupported.Description, "Image media part is missing") {
					missingImageSlots++
				}
			}
		}
	}
	imageRelationships := strings.Count(string(beforeParts["word/_rels/document.xml.rels"]), `Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"`)
	mediaPartCount := 0
	for name := range beforeParts {
		if strings.HasPrefix(name, "word/media/") && !strings.HasSuffix(name, "/") {
			mediaPartCount++
		}
	}
	if paragraphCount != 106 || tableCount != 0 || imageRelationships != 28 || len(images) != 27 || missingImageSlots != 1 || mediaPartCount != 27 {
		_ = word.Close()
		t.Fatalf("sample projection: paragraphs=%d tables=%d imageRelationships=%d resolvedInlineImages=%d missingImageSlots=%d mediaParts=%d; want 106, 0, 28, 27, 1, 27", paragraphCount, tableCount, imageRelationships, len(images), missingImageSlots, mediaPartCount)
	}
	for index, item := range images {
		if !item.Resolved || item.WidthEMU <= 0 || item.HeightEMU <= 0 {
			_ = word.Close()
			t.Fatalf("sample image %d (%s) is unresolved or has invalid extents: %#v", index+1, item.ID, item)
		}
		data, err := word.WordImageBytes(item.ID)
		if err != nil {
			_ = word.Close()
			t.Fatalf("sample image %d (%s) bytes: %v", index+1, item.ID, err)
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || format != "png" {
			_ = word.Close()
			t.Fatalf("sample image %d format/config = %q %#v, %v", index+1, format, config, err)
		}
		extentRatio := float64(item.WidthEMU) / float64(item.HeightEMU)
		pixelRatio := float64(config.Width) / float64(config.Height)
		if math.Abs(extentRatio-pixelRatio)/pixelRatio > 0.03 {
			_ = word.Close()
			t.Fatalf("sample image %d extent ratio %.4f differs from media ratio %.4f", index+1, extentRatio, pixelRatio)
		}
	}

	var editedRun *document.WordTextRun
	var untouchedRun *document.WordTextRun
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || strings.TrimSpace(inline.Text.Text) == "" || !inline.Text.Editable {
				continue
			}
			if editedRun == nil {
				copy := *inline.Text
				editedRun = &copy
			} else if untouchedRun == nil {
				copy := *inline.Text
				untouchedRun = &copy
			}
		}
	}
	if editedRun == nil || untouchedRun == nil {
		_ = word.Close()
		t.Fatal("sample did not provide two editable text runs")
	}
	originalMedia, err := word.WordImageBytes(images[0].ID)
	if err != nil {
		_ = word.Close()
		t.Fatal(err)
	}
	newWidth := images[0].WidthEMU * 9 / 10
	if newWidth <= 0 {
		_ = word.Close()
		t.Fatal("sample image is too small to resize safely")
	}
	textEditor := document.WordTextEditor(word)
	imageEditor := document.WordImageEditor(word)
	if err := textEditor.ReplaceTextRun(editedRun.ID, editedRun.Text+" [Glance 回归]"); err != nil {
		_ = word.Close()
		t.Fatal(err)
	}
	if err := imageEditor.ResizeInlineImage(images[0].ID, newWidth); err != nil {
		_ = word.Close()
		t.Fatal(err)
	}
	if err := word.Save(); err != nil {
		_ = word.Close()
		t.Fatal(err)
	}
	if err := word.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := docx.Open(tempPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedBlocks := reopened.WordBlocks()
	reopenedRun, ok := selectedWordRun(reopenedBlocks, editedRun.ID)
	if !ok || reopenedRun.Text != editedRun.Text+" [Glance 回归]" {
		t.Fatalf("saved text run = %#v; want edited run", reopenedRun)
	}
	untouched, ok := selectedWordRun(reopenedBlocks, untouchedRun.ID)
	if !ok || untouched.Text != untouchedRun.Text {
		t.Fatalf("untouched sample run changed: %#v; want %q", untouched, untouchedRun.Text)
	}
	resized := findWordImage(reopenedBlocks, images[0].ID)
	if resized == nil || resized.WidthEMU != newWidth || resized.HeightEMU <= 0 {
		t.Fatalf("saved image extent = %#v; want width %d", resized, newWidth)
	}
	resizedRatio := float64(resized.WidthEMU) / float64(resized.HeightEMU)
	originalRatio := float64(images[0].WidthEMU) / float64(images[0].HeightEMU)
	if math.Abs(resizedRatio-originalRatio)/originalRatio > 1e-6 {
		t.Fatalf("resized image ratio %.8f; want original %.8f", resizedRatio, originalRatio)
	}
	if afterMedia, err := reopened.WordImageBytes(images[0].ID); err != nil || !bytes.Equal(afterMedia, originalMedia) {
		t.Fatalf("resized image media changed: %v", err)
	}
	afterParts := readDocxPartMap(t, tempPath)
	if len(afterParts) != len(beforeParts) {
		t.Fatalf("save changed package part count: %d -> %d", len(beforeParts), len(afterParts))
	}
	if got := strings.Count(string(afterParts["word/_rels/document.xml.rels"]), `Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"`); got != 28 {
		t.Fatalf("save changed sample's 28 image relationship entries to %d", got)
	}
	postSaveWord, err := docx.Open(tempPath)
	if err != nil {
		t.Fatal(err)
	}
	postSaveBlocks := postSaveWord.WordBlocks()
	postSaveMissing := 0
	for _, paragraph := range allWordParagraphs(postSaveBlocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Unsupported != nil && strings.Contains(inline.Unsupported.Description, "Image media part is missing") {
				postSaveMissing++
			}
		}
	}
	_ = postSaveWord.Close()
	if postSaveMissing != 1 {
		t.Fatalf("save did not retain the broken inline image slot as a placeholder: %d missing slots", postSaveMissing)
	}
	for name, before := range beforeParts {
		if name == "word/document.xml" {
			continue
		}
		if after, ok := afterParts[name]; !ok || !bytes.Equal(before, after) {
			t.Fatalf("untouched package part %q changed or disappeared", name)
		}
	}
	if current, err := os.ReadFile(source); err != nil || !bytes.Equal(current, originalBytes) {
		t.Fatalf("sample source changed during temporary-copy regression: %v", err)
	}
}

func pngImageBytes(t *testing.T, imageData image.Image) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := png.Encode(&output, imageData); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func pngConfigHeader(width, height int) []byte {
	output := append([]byte{}, []byte("\x89PNG\r\n\x1a\n")...)
	chunk := make([]byte, 4+4+13+4)
	binary.BigEndian.PutUint32(chunk[:4], 13)
	copy(chunk[4:8], "IHDR")
	binary.BigEndian.PutUint32(chunk[8:12], uint32(width))
	binary.BigEndian.PutUint32(chunk[12:16], uint32(height))
	chunk[16], chunk[17], chunk[18] = 8, 6, 0
	crc := crc32.ChecksumIEEE(chunk[4 : len(chunk)-4])
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc)
	output = append(output, chunk...)
	return output
}

func readDocxPartMap(t *testing.T, path string) map[string][]byte {
	t.Helper()
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	parts := make(map[string][]byte, len(archive.File))
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		parts[file.Name] = data
	}
	return parts
}

func TestWordImagePickerInsertsAndReplacesThroughAsyncCommandFlow(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("sample.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	paragraph := state.wordBlocks[0].Paragraph
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	app := &App{
		documentSessionState:   state,
		documentGeneration:     8,
		wordCommandResults:     make(chan wordCommandResult, 1),
		wordImagePickerResults: make(chan wordImagePickerResult, 1),
		wordImageResults:       make(chan wordImageDecodeResult, 1),
		shutdown:               make(chan struct{}),
	}
	imagePath := filepath.Join(t.TempDir(), "first.png")
	firstImage := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	firstImage.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	firstBytes := pngImageBytes(t, firstImage)
	if err := os.WriteFile(imagePath, firstBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	app.wordImageFilePicker = func() (string, error) { return imagePath, nil }
	waitForPicker := func() {
		t.Helper()
		select {
		case result := <-app.wordImagePickerResults:
			app.wordImagePickerResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for asynchronous Word image picker")
		}
	}
	waitForCommand := func() {
		t.Helper()
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("Word image command failed: %v", result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for asynchronous Word image command")
		}
	}

	app.startWordImagePicker(wordImageInsert)
	waitForPicker()
	waitForCommand()
	imageID := state.selectedWordImageID
	if imageID == "" || findWordImage(state.wordBlocks, imageID) == nil {
		t.Fatalf("insert result did not select the new inline image: %q", imageID)
	}
	stored, err := word.WordImageBytes(imageID)
	if err != nil || !bytes.Equal(stored, firstBytes) {
		t.Fatalf("inserted image bytes changed: %v", err)
	}

	secondImage := image.NewNRGBA(image.Rect(0, 0, 1, 2))
	secondImage.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
	secondBytes := pngImageBytes(t, secondImage)
	imagePath = filepath.Join(t.TempDir(), "replacement.png")
	if err := os.WriteFile(imagePath, secondBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	app.startWordImagePicker(wordImageReplace)
	waitForPicker()
	waitForCommand()
	stored, err = word.WordImageBytes(imageID)
	if err != nil || !bytes.Equal(stored, secondBytes) {
		t.Fatalf("replacement image bytes changed: %v", err)
	}
	if state.selectedWordImageID != imageID {
		t.Fatalf("replace lost selected image identity: %q != %q", state.selectedWordImageID, imageID)
	}
}
