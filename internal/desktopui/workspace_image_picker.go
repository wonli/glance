package desktopui

import (
	"bytes"
	"fmt"
	"glance/internal/document"
	"image"
	"os"
	"path/filepath"
	"strings"
)

type wordImagePickerOperation uint8

const (
	wordImageInsert wordImagePickerOperation = iota + 1
	wordImageReplace
)

const maxWordImageFileBytes = 128 << 20

type wordImageFile struct {
	data      []byte
	mediaType string
}

type wordImagePickerResult struct {
	state         *documentSessionState
	generation    uint64
	operation     wordImagePickerOperation
	paragraphID   string
	afterInlineID string
	imageID       string
	path          string
	data          []byte
	mediaType     string
	err           error
}

func (a *App) startWordImagePicker(operation wordImagePickerOperation) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() || a.wordImagePickerRunning {
		return
	}
	if _, ok := any(state.word).(document.WordImageEditor); !ok {
		return
	}
	result := wordImagePickerResult{state: state, generation: a.documentGeneration, operation: operation}
	switch operation {
	case wordImageInsert:
		if _, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID); !ok {
			return
		}
		result.paragraphID = state.selectedWordParagraphID
		if state.selectedWordRunID != "" {
			result.afterInlineID = state.selectedWordRunID
		} else if state.selectedWordImageID != "" {
			result.afterInlineID = state.selectedWordImageID
		}
	case wordImageReplace:
		if findWordImage(wordBlocksForState(state), state.selectedWordImageID) == nil {
			return
		}
		result.imageID = state.selectedWordImageID
	default:
		return
	}
	if a.wordImagePickerResults == nil {
		a.wordImagePickerResults = make(chan wordImagePickerResult, 1)
	}
	a.wordImagePickerRunning = true
	if operation == wordImageInsert {
		a.setStatus("选择要插入的 PNG/JPEG 图片…")
	} else {
		a.setStatus("选择用于替换的 PNG/JPEG 图片…")
	}
	choose := a.wordImageFilePicker
	if choose == nil {
		choose = chooseOfficeImageFile
	}
	shutdown, results, window := a.shutdown, a.wordImagePickerResults, a.window
	go func() {
		result.path, result.err = choose()
		if result.err == nil && result.path != "" {
			var file wordImageFile
			file, result.err = loadWordImageFile(result.path)
			result.data, result.mediaType = file.data, file.mediaType
		}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func loadWordImageFile(path string) (wordImageFile, error) {
	var mediaType, wantFormat string
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		mediaType, wantFormat = "image/png", "png"
	case ".jpg", ".jpeg":
		mediaType, wantFormat = "image/jpeg", "jpeg"
	default:
		return wordImageFile{}, fmt.Errorf("仅支持 PNG/JPEG 图片")
	}
	info, err := os.Stat(path)
	if err != nil {
		return wordImageFile{}, fmt.Errorf("读取图片文件: %w", err)
	}
	if info.Size() > maxWordImageFileBytes {
		return wordImageFile{}, fmt.Errorf("图片文件超过 %d MiB", maxWordImageFileBytes>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return wordImageFile{}, fmt.Errorf("读取图片文件: %w", err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return wordImageFile{}, fmt.Errorf("图片文件损坏: %w", err)
	}
	if format != wantFormat {
		return wordImageFile{}, fmt.Errorf("图片扩展名与实际 %s 格式不匹配", format)
	}
	if err := validateWordPreviewConfig(config, format); err != nil {
		return wordImageFile{}, err
	}
	if _, err := decodeWordPreview(data); err != nil {
		return wordImageFile{}, fmt.Errorf("图片文件损坏: %w", err)
	}
	return wordImageFile{data: data, mediaType: mediaType}, nil
}

func (a *App) handleWordImagePickerResult(result wordImagePickerResult) {
	a.wordImagePickerRunning = false
	if result.state == nil || result.state.closed.Load() || result.generation != a.documentGeneration || result.state != a.documentSessionState {
		return
	}
	if result.err != nil {
		a.setStatus("图片读取失败：" + result.err.Error())
		return
	}
	if result.path == "" {
		a.setStatus("已取消选择图片")
		return
	}
	if result.state.formatRunning.Load() || a.isSaving() {
		a.setStatus("文档正在处理其他操作，请稍后重试")
		return
	}
	switch result.operation {
	case wordImageInsert:
		a.runWordImageInsertCommand(result.paragraphID, result.afterInlineID, result.mediaType, result.data)
	case wordImageReplace:
		imageID := result.imageID
		a.runWordImageCommand("替换图片", imageID, func(editor document.WordImageEditor) error {
			return editor.ReplaceInlineImage(imageID, result.mediaType, result.data)
		}, false)
	default:
		a.setStatus("无效的图片操作")
	}
}

func (a *App) runWordImageInsertCommand(paragraphID, afterInlineID, mediaType string, data []byte) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordImageEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		var imageID string
		blocks, err := runWordMutation(state, func() error {
			var insertErr error
			imageID, insertErr = editor.InsertInlineImage(paragraphID, afterInlineID, mediaType, data)
			return insertErr
		})
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "插入图片", err: err, imageID: imageID, selectInsertedImage: err == nil, blocks: blocks})
	}()
}
