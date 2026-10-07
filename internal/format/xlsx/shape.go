package xlsx

import (
	"errors"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"glance/internal/document"
)

var _ document.SpreadsheetShapeEditor = (*Workbook)(nil)

// AddShape inserts a geometric shape or text box anchored at the specified cell in a worksheet.
func (w *Workbook) AddShape(spec document.SpreadsheetShapeSpec) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}

	sheet := strings.TrimSpace(spec.Sheet)
	if sheet == "" {
		if w.file.SheetCount <= 0 {
			return errors.New("工作簿中没有可用的工作表")
		}
		sheet = w.file.GetSheetName(0)
	}

	cell := strings.TrimSpace(spec.Cell)
	if cell == "" {
		cell = "B2"
	}
	shapeType := strings.TrimSpace(spec.Type)
	if shapeType == "" {
		shapeType = "rect"
	}
	width := spec.Width
	if width == 0 {
		width = 160
	}
	height := spec.Height
	if height == 0 {
		height = 90
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}

	printObj := true
	shape := &excelize.Shape{
		Cell:   cell,
		Type:   shapeType,
		Width:  width,
		Height: height,
		Format: excelize.GraphicOptions{
			PrintObject:     &printObj,
			LockAspectRatio: false,
		},
	}

	if text := strings.TrimSpace(spec.Text); text != "" {
		shape.Paragraph = []excelize.RichTextRun{
			{
				Text: text,
				Font: &excelize.Font{
					Family: "Arial",
					Size:   11,
					Color:  "333333",
				},
			},
		}
	}

	if err := w.file.AddShape(sheet, shape); err != nil {
		return fmt.Errorf("add shape to sheet %q at %s: %w", sheet, cell, err)
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}
