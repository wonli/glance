package xlsx

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"strings"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

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
