package xlsx

import (
	"errors"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"glance/internal/document"
)

var _ document.SpreadsheetChartEditor = (*Workbook)(nil)

// AddChart creates and inserts a column, line, or pie chart referencing a worksheet range.
func (w *Workbook) AddChart(spec document.SpreadsheetChartSpec) error {
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

	r := spec.Range
	if r.StartRow <= 0 {
		r.StartRow = 1
	}
	if r.StartColumn <= 0 {
		r.StartColumn = 1
	}
	if r.EndRow < r.StartRow {
		r.EndRow = r.StartRow + 3
	}
	if r.EndColumn < r.StartColumn {
		r.EndColumn = r.StartColumn + 1
	}

	cell := strings.TrimSpace(spec.Cell)
	if cell == "" {
		nextColName, err := excelize.ColumnNumberToName(r.EndColumn + 2)
		if err != nil {
			nextColName = "E"
		}
		cell = fmt.Sprintf("%s%d", nextColName, r.StartRow)
	}

	var chartType excelize.ChartType
	switch spec.Kind {
	case document.SpreadsheetChartLine:
		chartType = excelize.Line
	case document.SpreadsheetChartPie:
		chartType = excelize.Pie
	case document.SpreadsheetChartColumn:
		fallthrough
	default:
		chartType = excelize.Col
	}

	title := strings.TrimSpace(spec.Title)
	if title == "" {
		switch spec.Kind {
		case document.SpreadsheetChartLine:
			title = "折线图"
		case document.SpreadsheetChartPie:
			title = "饼图"
		default:
			title = "柱状图"
		}
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}

	var series []excelize.ChartSeries
	startColName, _ := excelize.ColumnNumberToName(r.StartColumn)

	if r.EndColumn > r.StartColumn {
		// First column treated as Categories, subsequent columns as series
		catRange := fmt.Sprintf("%s!$%s$%d:$%s$%d", sheet, startColName, r.StartRow+1, startColName, r.EndRow)
		for c := r.StartColumn + 1; c <= r.EndColumn; c++ {
			cName, _ := excelize.ColumnNumberToName(c)
			series = append(series, excelize.ChartSeries{
				Name:       fmt.Sprintf("%s!$%s$%d", sheet, cName, r.StartRow),
				Categories: catRange,
				Values:     fmt.Sprintf("%s!$%s$%d:$%s$%d", sheet, cName, r.StartRow+1, cName, r.EndRow),
			})
		}
	} else {
		// Single column
		series = append(series, excelize.ChartSeries{
			Name:   title,
			Values: fmt.Sprintf("%s!$%s$%d:$%s$%d", sheet, startColName, r.StartRow, startColName, r.EndRow),
		})
	}

	enable, disable := true, false
	chartObj := &excelize.Chart{
		Type:   chartType,
		Series: series,
		Format: excelize.GraphicOptions{
			ScaleX:          1,
			ScaleY:          1,
			PrintObject:     &enable,
			LockAspectRatio: false,
			Locked:          &disable,
		},
		Title: excelize.ChartTitle{
			Paragraph: []excelize.RichTextRun{
				{
					Text: title,
				},
			},
		},
		Legend: excelize.ChartLegend{
			Position: "right",
		},
	}

	if err := w.file.AddChart(sheet, cell, chartObj); err != nil {
		return fmt.Errorf("add chart to sheet %q at %s: %w", sheet, cell, err)
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}
