package xlsx

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"path/filepath"
	"strings"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

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
