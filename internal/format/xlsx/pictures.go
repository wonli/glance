package xlsx

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"path/filepath"
	"strings"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

const (
	maxSpreadsheetPictureBytes  = 128 << 20
	maxSpreadsheetPicturePixels = uint64(32 * 1024 * 1024)
	maxSpreadsheetPictureOffset = 1_000_000
	maxSpreadsheetPictureSize   = 1_000_000
)

type spreadsheetPictureIndex struct {
	sheet string
	cell  string
	index int
}

func pictureObjectID(sheet, cell string, index int) string {
	return fmt.Sprintf("xlsx-picture:%q:%s:%d", sheet, cell, index)
}

func (w *Workbook) SpreadsheetImages(sheet string) ([]document.SpreadsheetImage, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return nil, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	cells, err := w.file.GetPictureCells(sheet)
	if err != nil {
		return nil, fmt.Errorf("list pictures in worksheet %q: %w", sheet, err)
	}
	if w.pictureIndexes == nil {
		w.pictureIndexes = make(map[string]spreadsheetPictureIndex)
	}
	for id, index := range w.pictureIndexes {
		if index.sheet == sheet {
			delete(w.pictureIndexes, id)
		}
	}
	images := make([]document.SpreadsheetImage, 0, len(cells))
	for _, cell := range cells {
		column, row, err := excelize.CellNameToCoordinates(cell)
		if err != nil || row < 1 || row > maxXLSXRows || column < 1 || column > maxXLSXColumns {
			return nil, fmt.Errorf("invalid picture anchor %q in worksheet %q", cell, sheet)
		}
		pictures, err := w.file.GetPictures(sheet, cell)
		if err != nil {
			return nil, fmt.Errorf("read pictures at %s!%s: %w", sheet, cell, err)
		}
		for index, picture := range pictures {
			id := pictureObjectID(sheet, cell, index)
			w.pictureIndexes[id] = spreadsheetPictureIndex{sheet: sheet, cell: cell, index: index}
			view, err := spreadsheetImageView(id, sheet, cell, row, column, picture, len(pictures))
			if err != nil {
				return nil, fmt.Errorf("read picture %s!%s[%d]: %w", sheet, cell, index, err)
			}
			images = append(images, view)
		}
	}
	return images, nil
}

func spreadsheetImageView(id, sheet, cell string, row, column int, picture excelize.Picture, anchorCount int) (document.SpreadsheetImage, error) {
	view := document.SpreadsheetImage{
		ID: id, Sheet: sheet, Cell: cell, Row: row, Column: column,
		MediaType: spreadsheetPictureMediaType(picture.Extension),
	}
	if picture.Format != nil {
		view.OffsetX, view.OffsetY = picture.Format.OffsetX, picture.Format.OffsetY
		view.Name, view.AltText = picture.Format.Name, picture.Format.AltText
		view.LockAspectRatio = picture.Format.LockAspectRatio
		view.MoveWithCells, view.SizeWithCells = picturePositioning(picture.Format.Positioning)
	}
	if picture.InsertType != excelize.PictureInsertTypePlaceOverCells {
		view.ReadOnlyReason = "仅支持编辑锚定在单元格上方的图片"
	}
	if anchorCount > 1 && view.ReadOnlyReason == "" {
		view.ReadOnlyReason = "同一单元格锚点有多张图片，当前无法安全单独编辑"
	}
	if view.MediaType == "" {
		if view.ReadOnlyReason == "" {
			view.ReadOnlyReason = "图片格式暂不支持预览或编辑"
		}
		return view, nil
	}
	if len(picture.File) == 0 || len(picture.File) > maxSpreadsheetPictureBytes {
		if view.ReadOnlyReason == "" {
			view.ReadOnlyReason = "图片文件超过安全限制"
		}
		return view, nil
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(picture.File))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		if view.ReadOnlyReason == "" {
			view.ReadOnlyReason = "图片无法解码"
		}
		return view, nil
	}
	if uint64(config.Width) > maxSpreadsheetPicturePixels/uint64(config.Height) {
		if view.ReadOnlyReason == "" {
			view.ReadOnlyReason = "图片像素超过安全限制"
		}
		return view, nil
	}
	if mediaType := spreadsheetPictureMediaType("." + format); mediaType == "" || mediaType != view.MediaType {
		if view.ReadOnlyReason == "" {
			view.ReadOnlyReason = "图片扩展名与实际内容不匹配"
		}
		return view, nil
	}
	scaleX, scaleY := 1.0, 1.0
	if picture.Format != nil {
		if picture.Format.ScaleX != 0 {
			scaleX = picture.Format.ScaleX
		}
		if picture.Format.ScaleY != 0 {
			scaleY = picture.Format.ScaleY
		}
	}
	if !finitePictureScale(scaleX) || !finitePictureScale(scaleY) {
		view.ReadOnlyReason = "图片尺寸比例无效"
		return view, nil
	}
	view.WidthPx = int(math.Round(float64(config.Width) * scaleX))
	view.HeightPx = int(math.Round(float64(config.Height) * scaleY))
	if view.WidthPx <= 0 || view.HeightPx <= 0 {
		if view.ReadOnlyReason == "" {
			view.ReadOnlyReason = "图片尺寸无效"
		}
		return view, nil
	}
	if view.ReadOnlyReason != "" {
		return view, nil
	}
	view.Editable = true
	return view, nil
}

func spreadsheetPictureMediaType(extension string) string {
	switch strings.ToLower(filepath.Ext(extension)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	default:
		return ""
	}
}

func picturePositioning(position string) (moveWithCells, sizeWithCells bool) {
	switch strings.ToLower(position) {
	case "onecell":
		return true, false
	case "absolute":
		return false, false
	default: // Excelize's empty default and "twoCell" both track cell geometry.
		return true, true
	}
}

func finitePictureScale(scale float64) bool {
	return !math.IsNaN(scale) && !math.IsInf(scale, 0) && scale > 0 && scale <= 1000
}

func (w *Workbook) SpreadsheetImageBytes(imageID string) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	index, ok := w.pictureIndexes[imageID]
	if !ok {
		return nil, fmt.Errorf("spreadsheet picture %q is stale or unknown", imageID)
	}
	pictures, err := w.file.GetPictures(index.sheet, index.cell)
	if err != nil {
		return nil, fmt.Errorf("read picture bytes at %s!%s: %w", index.sheet, index.cell, err)
	}
	if index.index < 0 || index.index >= len(pictures) {
		return nil, fmt.Errorf("spreadsheet picture %q is stale or unknown", imageID)
	}
	return append([]byte(nil), pictures[index.index].File...), nil
}

type resolvedSpreadsheetPicture struct {
	index   spreadsheetPictureIndex
	picture excelize.Picture
	view    document.SpreadsheetImage
}

var _ document.SpreadsheetImageEditor = (*Workbook)(nil)
