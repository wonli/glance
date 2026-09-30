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

func (w *Workbook) InsertSpreadsheetImage(placement document.SpreadsheetImagePlacement, data []byte) (string, error) {
	extension, config, err := validateSpreadsheetPictureData(placement.MediaType, data)
	if err != nil {
		return "", err
	}
	if err := validatePicturePlacement(placement); err != nil {
		return "", err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return "", err
	}
	if !contains(w.file.GetSheetList(), placement.Sheet) {
		return "", fmt.Errorf("worksheet %q does not exist", placement.Sheet)
	}
	cell, err := excelize.CoordinatesToCellName(placement.Column, placement.Row)
	if err != nil {
		return "", fmt.Errorf("invalid picture anchor: %w", err)
	}
	occupied, err := pictureAnchorOccupied(w.file, placement.Sheet, cell)
	if err != nil {
		return "", err
	}
	if occupied {
		return "", fmt.Errorf("picture anchor %s!%s is already occupied", placement.Sheet, cell)
	}
	scale, err := pictureScaleToFit(placement.WidthPx, placement.HeightPx, config.Width, config.Height)
	if err != nil {
		return "", err
	}
	before, staged, err := w.stagePictureMutationLocked()
	if err != nil {
		return "", err
	}
	options := &excelize.GraphicOptions{
		Name: placement.Name, AltText: placement.AltText,
		OffsetX: placement.OffsetX, OffsetY: placement.OffsetY,
		ScaleX: scale, ScaleY: scale, LockAspectRatio: true,
		Positioning: "twoCell",
	}
	if err := staged.AddPictureFromBytes(placement.Sheet, cell, &excelize.Picture{Extension: extension, File: append([]byte(nil), data...), Format: options}); err != nil {
		_ = staged.Close()
		return "", fmt.Errorf("insert picture at %s!%s: %w", placement.Sheet, cell, err)
	}
	if err := w.commitPictureMutationLocked(before, staged); err != nil {
		return "", fmt.Errorf("commit picture insertion at %s!%s: %w", placement.Sheet, cell, err)
	}
	id := pictureObjectID(placement.Sheet, cell, 0)
	w.pictureIndexes = map[string]spreadsheetPictureIndex{id: {sheet: placement.Sheet, cell: cell, index: 0}}
	return id, nil
}

func (w *Workbook) ReplaceSpreadsheetImage(imageID, mediaType string, data []byte) error {
	extension, config, err := validateSpreadsheetPictureData(mediaType, data)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	resolved, err := w.resolveEditablePictureLocked(w.file, imageID)
	if err != nil {
		return err
	}
	if strings.EqualFold(filepath.Ext(resolved.picture.Extension), extension) && bytes.Equal(resolved.picture.File, data) {
		return nil
	}
	scale, err := pictureScaleToFit(resolved.view.WidthPx, resolved.view.HeightPx, config.Width, config.Height)
	if err != nil {
		return fmt.Errorf("replacement image does not fit the existing picture bounds: %w", err)
	}
	before, staged, err := w.stagePictureMutationLocked()
	if err != nil {
		return err
	}
	current, err := resolvePictureAt(staged, resolved.index)
	if err != nil {
		_ = staged.Close()
		return err
	}
	options := clonePictureOptions(current.picture.Format)
	options.ScaleX, options.ScaleY = scale, scale
	options.LockAspectRatio = true
	if err := staged.DeletePicture(current.index.sheet, current.index.cell); err != nil {
		_ = staged.Close()
		return fmt.Errorf("remove picture before replacement: %w", err)
	}
	if err := staged.AddPictureFromBytes(current.index.sheet, current.index.cell, &excelize.Picture{Extension: extension, File: append([]byte(nil), data...), Format: options}); err != nil {
		_ = staged.Close()
		return fmt.Errorf("add replacement picture: %w", err)
	}
	if err := w.commitPictureMutationLocked(before, staged); err != nil {
		return fmt.Errorf("commit picture replacement: %w", err)
	}
	return nil
}

func (w *Workbook) MoveSpreadsheetImage(imageID string, row, column, offsetX, offsetY int) error {
	if row < 1 || column < 1 || row > maxXLSXRows || column > maxXLSXColumns {
		return fmt.Errorf("picture anchor is outside XLSX worksheet limits")
	}
	if err := validatePictureOffsets(offsetX, offsetY); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	resolved, err := w.resolveEditablePictureLocked(w.file, imageID)
	if err != nil {
		return err
	}
	targetCell, err := excelize.CoordinatesToCellName(column, row)
	if err != nil {
		return fmt.Errorf("invalid target picture anchor: %w", err)
	}
	if targetCell != resolved.index.cell {
		occupied, err := pictureAnchorOccupied(w.file, resolved.index.sheet, targetCell)
		if err != nil {
			return err
		}
		if occupied {
			return fmt.Errorf("picture anchor %s!%s is already occupied", resolved.index.sheet, targetCell)
		}
	} else if offsetX == resolved.view.OffsetX && offsetY == resolved.view.OffsetY {
		return nil
	}
	before, staged, err := w.stagePictureMutationLocked()
	if err != nil {
		return err
	}
	current, err := resolvePictureAt(staged, resolved.index)
	if err != nil {
		_ = staged.Close()
		return err
	}
	options := clonePictureOptions(current.picture.Format)
	options.OffsetX, options.OffsetY = offsetX, offsetY
	if err := staged.DeletePicture(current.index.sheet, current.index.cell); err != nil {
		_ = staged.Close()
		return fmt.Errorf("remove picture before move: %w", err)
	}
	if err := staged.AddPictureFromBytes(current.index.sheet, targetCell, &excelize.Picture{Extension: current.picture.Extension, File: append([]byte(nil), current.picture.File...), Format: options}); err != nil {
		_ = staged.Close()
		return fmt.Errorf("move picture to %s: %w", targetCell, err)
	}
	if err := w.commitPictureMutationLocked(before, staged); err != nil {
		return fmt.Errorf("commit picture move: %w", err)
	}
	return nil
}

func (w *Workbook) ResizeSpreadsheetImage(imageID string, widthPx int) error {
	if widthPx < 1 || widthPx > maxSpreadsheetPictureSize {
		return fmt.Errorf("picture width must be between 1 and %d pixels", maxSpreadsheetPictureSize)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	resolved, err := w.resolveEditablePictureLocked(w.file, imageID)
	if err != nil {
		return err
	}
	if widthPx == resolved.view.WidthPx {
		return nil
	}
	if resolved.view.WidthPx < 1 || resolved.view.HeightPx < 1 {
		return fmt.Errorf("picture has invalid current dimensions")
	}
	heightPx := int(math.Round(float64(widthPx) * float64(resolved.view.HeightPx) / float64(resolved.view.WidthPx)))
	if heightPx < 1 || heightPx > maxSpreadsheetPictureSize {
		return fmt.Errorf("resulting picture height is outside the supported pixel range")
	}
	before, staged, err := w.stagePictureMutationLocked()
	if err != nil {
		return err
	}
	current, err := resolvePictureAt(staged, resolved.index)
	if err != nil {
		_ = staged.Close()
		return err
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(current.picture.File))
	if err != nil {
		_ = staged.Close()
		return fmt.Errorf("decode current picture for resize: %w", err)
	}
	if config.Width < 1 || config.Height < 1 {
		_ = staged.Close()
		return fmt.Errorf("current picture has invalid dimensions")
	}
	scaleX, err := pixelsToExcelizeScale(widthPx, config.Width)
	if err != nil {
		_ = staged.Close()
		return err
	}
	scaleY, err := pixelsToExcelizeScale(heightPx, config.Height)
	if err != nil {
		_ = staged.Close()
		return err
	}
	options := clonePictureOptions(current.picture.Format)
	options.ScaleX, options.ScaleY = scaleX, scaleY
	options.LockAspectRatio = true
	if err := staged.DeletePicture(current.index.sheet, current.index.cell); err != nil {
		_ = staged.Close()
		return fmt.Errorf("remove picture before resize: %w", err)
	}
	if err := staged.AddPictureFromBytes(current.index.sheet, current.index.cell, &excelize.Picture{Extension: current.picture.Extension, File: append([]byte(nil), current.picture.File...), Format: options}); err != nil {
		_ = staged.Close()
		return fmt.Errorf("resize picture: %w", err)
	}
	if err := w.commitPictureMutationLocked(before, staged); err != nil {
		return fmt.Errorf("commit picture resize: %w", err)
	}
	return nil
}

func (w *Workbook) DeleteSpreadsheetImage(imageID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	resolved, err := w.resolveEditablePictureLocked(w.file, imageID)
	if err != nil {
		return err
	}
	before, staged, err := w.stagePictureMutationLocked()
	if err != nil {
		return err
	}
	current, err := resolvePictureAt(staged, resolved.index)
	if err != nil {
		_ = staged.Close()
		return err
	}
	if err := staged.DeletePicture(current.index.sheet, current.index.cell); err != nil {
		_ = staged.Close()
		return fmt.Errorf("delete picture at %s!%s: %w", current.index.sheet, current.index.cell, err)
	}
	if err := w.commitPictureMutationLocked(before, staged); err != nil {
		return fmt.Errorf("commit picture deletion: %w", err)
	}
	return nil
}

type resolvedSpreadsheetPicture struct {
	index   spreadsheetPictureIndex
	picture excelize.Picture
	view    document.SpreadsheetImage
}

func (w *Workbook) resolveEditablePictureLocked(file *excelize.File, imageID string) (resolvedSpreadsheetPicture, error) {
	index, ok := w.pictureIndexes[imageID]
	if !ok {
		return resolvedSpreadsheetPicture{}, fmt.Errorf("spreadsheet picture %q is stale or unknown", imageID)
	}
	resolved, err := resolvePictureAt(file, index)
	if err != nil {
		return resolvedSpreadsheetPicture{}, err
	}
	if !resolved.view.Editable {
		if resolved.view.ReadOnlyReason != "" {
			return resolvedSpreadsheetPicture{}, fmt.Errorf("spreadsheet picture %q is read-only: %s", imageID, resolved.view.ReadOnlyReason)
		}
		return resolvedSpreadsheetPicture{}, fmt.Errorf("spreadsheet picture %q is read-only", imageID)
	}
	return resolved, nil
}

func resolvePictureAt(file *excelize.File, index spreadsheetPictureIndex) (resolvedSpreadsheetPicture, error) {
	pictures, err := file.GetPictures(index.sheet, index.cell)
	if err != nil {
		return resolvedSpreadsheetPicture{}, fmt.Errorf("read pictures at %s!%s: %w", index.sheet, index.cell, err)
	}
	if index.index < 0 || index.index >= len(pictures) {
		return resolvedSpreadsheetPicture{}, fmt.Errorf("spreadsheet picture at %s!%s is stale or unknown", index.sheet, index.cell)
	}
	column, row, err := excelize.CellNameToCoordinates(index.cell)
	if err != nil {
		return resolvedSpreadsheetPicture{}, fmt.Errorf("invalid picture anchor %q: %w", index.cell, err)
	}
	view, err := spreadsheetImageView(pictureObjectID(index.sheet, index.cell, index.index), index.sheet, index.cell, row, column, pictures[index.index], len(pictures))
	if err != nil {
		return resolvedSpreadsheetPicture{}, err
	}
	return resolvedSpreadsheetPicture{index: index, picture: pictures[index.index], view: view}, nil
}

func (w *Workbook) stagePictureMutationLocked() ([]byte, *excelize.File, error) {
	before, err := w.snapshotLocked()
	if err != nil {
		return nil, nil, fmt.Errorf("capture picture edit history: %w", err)
	}
	staged, err := excelize.OpenReader(bytes.NewReader(before))
	if err != nil {
		return nil, nil, fmt.Errorf("stage picture edit: %w", err)
	}
	return before, staged, nil
}

func (w *Workbook) commitPictureMutationLocked(before []byte, staged *excelize.File) error {
	stagedBuffer, err := staged.WriteToBuffer()
	if err != nil {
		_ = staged.Close()
		return fmt.Errorf("serialize staged picture edit: %w", err)
	}
	if bytes.Equal(before, stagedBuffer.Bytes()) {
		_ = staged.Close()
		return nil
	}
	previous, previousIndexes := w.file, w.pictureIndexes
	w.file = staged
	w.pictureIndexes = nil
	if err := w.recordHistoryLocked(before); err != nil {
		w.file, w.pictureIndexes = previous, previousIndexes
		_ = staged.Close()
		return err
	}
	w.dirty = true
	if previous != nil {
		_ = previous.Close()
	}
	return nil
}

func validateSpreadsheetPictureData(mediaType string, data []byte) (string, image.Config, error) {
	if len(data) == 0 || len(data) > maxSpreadsheetPictureBytes {
		return "", image.Config{}, fmt.Errorf("picture data must be between 1 and %d bytes", maxSpreadsheetPictureBytes)
	}
	var extension string
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/png":
		extension = ".png"
	case "image/jpeg", "image/jpg":
		extension = ".jpg"
	default:
		return "", image.Config{}, fmt.Errorf("only PNG and JPEG pictures can be edited")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", image.Config{}, fmt.Errorf("decode picture metadata: %w", err)
	}
	if config.Width < 1 || config.Height < 1 || uint64(config.Width) > maxSpreadsheetPicturePixels/uint64(config.Height) {
		return "", image.Config{}, fmt.Errorf("picture dimensions exceed the %d-megapixel limit", maxSpreadsheetPicturePixels/(1024*1024))
	}
	if spreadsheetPictureMediaType("."+format) != spreadsheetPictureMediaType(extension) {
		return "", image.Config{}, fmt.Errorf("picture MIME type does not match encoded %s data", format)
	}
	return extension, config, nil
}

func validatePicturePlacement(placement document.SpreadsheetImagePlacement) error {
	if placement.Row < 1 || placement.Column < 1 || placement.Row > maxXLSXRows || placement.Column > maxXLSXColumns {
		return fmt.Errorf("picture anchor is outside XLSX worksheet limits")
	}
	if placement.WidthPx < 1 || placement.HeightPx < 1 || placement.WidthPx > maxSpreadsheetPictureSize || placement.HeightPx > maxSpreadsheetPictureSize {
		return fmt.Errorf("picture bounds must be between 1 and %d pixels", maxSpreadsheetPictureSize)
	}
	return validatePictureOffsets(placement.OffsetX, placement.OffsetY)
}

func validatePictureOffsets(offsetX, offsetY int) error {
	if offsetX < 0 || offsetY < 0 || offsetX > maxSpreadsheetPictureOffset || offsetY > maxSpreadsheetPictureOffset {
		return fmt.Errorf("picture offsets must be between 0 and %d pixels", maxSpreadsheetPictureOffset)
	}
	return nil
}

func pictureAnchorOccupied(file *excelize.File, sheet, cell string) (bool, error) {
	cells, err := file.GetPictureCells(sheet)
	if err != nil {
		return false, fmt.Errorf("list picture anchors in worksheet %q: %w", sheet, err)
	}
	for _, candidate := range cells {
		if strings.EqualFold(candidate, cell) {
			return true, nil
		}
	}
	return false, nil
}

func clonePictureOptions(options *excelize.GraphicOptions) *excelize.GraphicOptions {
	if options == nil {
		return &excelize.GraphicOptions{ScaleX: 1, ScaleY: 1}
	}
	cloned := *options
	return &cloned
}

func pictureScaleToFit(widthPx, heightPx, sourceWidth, sourceHeight int) (float64, error) {
	if widthPx < 1 || heightPx < 1 || sourceWidth < 1 || sourceHeight < 1 {
		return 0, fmt.Errorf("picture dimensions must be positive")
	}
	fitX := float64(widthPx) / float64(sourceWidth)
	fitY := float64(heightPx) / float64(sourceHeight)
	scale := math.Min(fitX, fitY)
	// Excelize persists picture scales to two decimal places. Nearest rounding
	// minimizes the displayed-size error while keeping both axes proportional.
	scale = math.Round(scale*100) / 100
	if !finitePictureScale(scale) {
		return 0, fmt.Errorf("picture bounds are too small for Excel's scale precision")
	}
	return scale, nil
}

func pixelsToExcelizeScale(displayPx, intrinsicPx int) (float64, error) {
	if displayPx < 1 || displayPx > maxSpreadsheetPictureSize || intrinsicPx < 1 {
		return 0, fmt.Errorf("picture dimensions must be between 1 and %d pixels", maxSpreadsheetPictureSize)
	}
	scale := math.Round(float64(displayPx)/float64(intrinsicPx)*100) / 100
	if !finitePictureScale(scale) {
		return 0, fmt.Errorf("picture dimensions exceed Excel's supported scale")
	}
	return scale, nil
}

var _ document.SpreadsheetImageEditor = (*Workbook)(nil)
