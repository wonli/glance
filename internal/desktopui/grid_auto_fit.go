package desktopui

import (
	"errors"
	"image"
	"math"
	"sort"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"glance/internal/document"
	"glance/internal/fontcatalog"
)

type gridAutoFitMeasurer struct {
	metric    unit.Metric
	scale     float64
	zoom      float64
	fonts     fontcatalog.Snapshot
	fontCache *fontCollectionCache
	themes    map[string]*material.Theme
}

func newGridAutoFitMeasurer(metric unit.Metric, scale float64, fonts fontcatalog.Snapshot) *gridAutoFitMeasurer {
	zoom := 1.0
	if metric.PxPerDp > 0 && scale > 0 {
		zoom = scale / float64(metric.PxPerDp)
	}
	return &gridAutoFitMeasurer{
		metric:    metric,
		scale:     scale,
		zoom:      zoom,
		fonts:     fonts,
		fontCache: newFontCollectionCache(),
		themes:    make(map[string]*material.Theme),
	}
}

func (m *gridAutoFitMeasurer) themeFor(style document.CellStyle) *material.Theme {
	family := strings.TrimSpace(style.FontFamily)
	if theme := m.themes[family]; theme != nil {
		return theme
	}
	theme := material.NewTheme()
	if family != "" {
		faces := fontCollectionWithCache(m.fonts, []string{family, string(preferredSystemTypeface)}, m.fontCache)
		if len(faces) != 0 {
			theme.Shaper = text.NewShaper(text.WithCollection(faces))
		}
	}
	m.themes[family] = theme
	return theme
}

func (m *gridAutoFitMeasurer) rowHeight(cell document.Cell, value string, width int) int {
	return measureGridCellTextHeight(layout.Context{Metric: m.metric}, m.themeFor(cell.Style), cell, value, width, m.scale)
}

func (m *gridAutoFitMeasurer) columnWidth(cell document.Cell, value string, maxWidth int) int {
	if value == "" || maxWidth <= 0 {
		return 0
	}
	style := paintStyle(cell.Style)
	label := material.Body2(m.themeFor(cell.Style), value)
	label.TextSize = cellTextSize(cell.Style, m.zoom)
	label.Color = style.Font
	label.Font.Typeface = cellTypeface(style)
	if style.Bold {
		label.Font.Weight = cellFontWeight(true)
	}
	if style.Italic {
		label.Font.Style = font.Italic
	}
	label.Alignment = cellTextAlignment(style.Align)
	if style.Wrap {
		label.WrapPolicy = text.WrapWords
	}

	padding := layout.Context{Metric: m.metric}.Dp(unit.Dp(9))
	contentWidth := max(1, maxWidth-padding)
	var ops op.Ops
	gtx := layout.Context{
		Metric:      m.metric,
		Ops:         &ops,
		Constraints: layout.Constraints{Min: image.Pt(0, 0), Max: image.Pt(contentWidth, max(1, int(maxGridRowPx*m.scale)))},
	}
	dimensions := label.Layout(gtx)
	return dimensions.Size.X + padding
}

func (a *App) startSpreadsheetAutoFitDimension(gtx layout.Context, row bool) {
	state, v := a.documentSessionState, a.virtualGrid
	if state == nil || state.book == nil || v == nil || !state.selectionSet || state.formatRunning.Load() || a.isSaving() {
		return
	}
	bounds := state.selection.Bounds()
	start, end := bounds.StartColumn, bounds.EndColumn
	if row {
		start, end = bounds.StartRow, bounds.EndRow
	}
	a.startSpreadsheetAutoFitDimensionRange(gtx, row, start, end)
}

func (a *App) startSpreadsheetResizeAutoFit(gtx layout.Context, axis resizeAxis, index int) {
	state := a.documentSessionState
	if state == nil || index < 0 {
		return
	}
	row := axis == resizeRows
	start, end := index+1, index+1
	if state.selectionSet {
		selection := state.selection
		bounds := selection.Bounds()
		selectedDimension := selection.Scope == selectionSheet
		if row {
			selectedDimension = selectedDimension || selection.Scope == selectionRows
		} else {
			selectedDimension = selectedDimension || selection.Scope == selectionColumns
		}
		if selectedDimension {
			selectionStart, selectionEnd := bounds.StartColumn, bounds.EndColumn
			if row {
				selectionStart, selectionEnd = bounds.StartRow, bounds.EndRow
			}
			if start >= selectionStart && start <= selectionEnd {
				start, end = selectionStart, selectionEnd
			}
		}
	}
	a.startSpreadsheetAutoFitDimensionRange(gtx, row, start, end)
}

func (a *App) startSpreadsheetAutoFitDimensionRange(gtx layout.Context, row bool, start, end int) {
	state, v := a.documentSessionState, a.virtualGrid
	if state == nil || state.book == nil || v == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	dimensions, rangeSupported := state.book.(document.SpreadsheetDimensionRangeCellReader)
	batch, batchSupported := state.book.(document.SpreadsheetDimensionBatchEditor)
	if !rangeSupported || !batchSupported {
		a.setStatus("当前工作簿不支持自动调整行列尺寸")
		return
	}
	axis := document.SpreadsheetDimensionColumn
	operation := "自动调整列宽"
	columns := (*AxisGeometry)(nil)
	if row {
		axis = document.SpreadsheetDimensionRow
		operation = "自动调整行高"
		columns = v.Columns.Clone()
	}
	if start < 1 || end < start {
		return
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatus("表格正在处理其他操作，请稍后再试")
		return
	}
	sheet, generation, window := state.sheet, a.documentGeneration, v
	metric, scale := gtx.Metric, v.Scale
	fonts := fontcatalog.Default()
	if a.desktopRuntime != nil && a.desktopRuntime.fonts != nil {
		fonts = a.desktopRuntime.fonts.Snapshot()
	}
	if !a.launchManagedBackground(func() {
		measurer := newGridAutoFitMeasurer(metric, scale, fonts)
		type dimensionMeasurement struct {
			currentPixels float64
			currentValue  float64
			desiredPixels float64
			hasContent    bool
		}
		measurements := make(map[int]dimensionMeasurement)
		contentCount := 0
		unlock := lockSessionAdapterWrite(state)
		err := dimensions.VisitDimensionRangeCells(sheet, axis, start, end, func(entry document.SpreadsheetDimensionCell) error {
			select {
			case <-a.shutdown:
				return errGridAutoFitCanceled
			default:
			}
			index := entry.DimensionIndex
			if index < start || index > end || !finite(entry.DimensionSize) || entry.DimensionSize <= 0 {
				return nil
			}
			currentPixels := charsToDevicePixels(entry.DimensionSize, scale)
			if row {
				currentPixels = pointsToDevicePixels(entry.DimensionSize, scale)
			}
			measurement := measurements[index]
			measurement.currentPixels = currentPixels
			measurement.currentValue = entry.DimensionSize
			if entry.Cell.Value == "" {
				measurements[index] = measurement
				return nil
			}
			contentCount++
			measurement.hasContent = true
			if row {
				if entry.Index < 1 || entry.Index > xlsxColumns {
					return nil
				}
				cellWidth := columns.sizeAt(entry.Index - 1)
				if finite(entry.ColumnWidthChars) && entry.ColumnWidthChars > 0 {
					cellWidth = math.Max(minGridColumnPx*scale, columnCharsToGridPixels(entry.ColumnWidthChars)*scale)
				}
				width := int(math.Max(0, cellWidth))
				measurement.desiredPixels = math.Max(measurement.desiredPixels, float64(measurer.rowHeight(entry.Cell, entry.Cell.Value, width)))
				measurements[index] = measurement
				return nil
			}
			maxWidth := int(math.Ceil(maxGridColumnPx * scale))
			measurement.desiredPixels = math.Max(measurement.desiredPixels, float64(measurer.columnWidth(entry.Cell, entry.Cell.Value, maxWidth)))
			measurements[index] = measurement
			return nil
		})
		unlock()

		indexes := make([]int, 0, len(measurements))
		for index := range measurements {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		var sizes []document.SpreadsheetDimensionSize
		changes := make([]spreadsheetDimensionChange, 0, len(indexes))
		minPixels, maxPixels := minGridColumnPx*scale, maxGridColumnPx*scale
		if row {
			minPixels, maxPixels = minGridRowPx*scale, maxGridRowPx*scale
		}
		for _, index := range indexes {
			measurement := measurements[index]
			if !measurement.hasContent {
				continue
			}
			desiredPixels := math.Max(minPixels, math.Min(maxPixels, measurement.desiredPixels))
			if math.Abs(desiredPixels-measurement.currentPixels) <= 0.5 {
				continue
			}
			value := devicePixelsToChars(desiredPixels, scale)
			if row {
				value = devicePixelsToPoints(desiredPixels, scale)
			}
			value = math.Round(value*100) / 100
			if math.Abs(value-measurement.currentValue) <= 0.005 {
				continue
			}
			sizes = append(sizes, document.SpreadsheetDimensionSize{Index: index, Value: value})
			changes = append(changes, spreadsheetDimensionChange{index: index, value: value, row: row, autoFit: true})
		}

		changed := false
		details := ""
		noOp := false
		if err == nil && contentCount == 0 {
			noOp = true
			if row {
				details = selectedDimensionNoContentDetails(start, end, true)
			} else {
				details = selectedDimensionNoContentDetails(start, end, false)
			}
		} else if err == nil && len(sizes) == 0 {
			noOp = true
			if row {
				details = selectedDimensionAlreadyFitDetails(start, end, true)
			} else {
				details = selectedDimensionAlreadyFitDetails(start, end, false)
			}
		} else if err == nil {
			unlock := lockSessionAdapterWrite(state)
			err = batch.SetDimensionSizes(sheet, axis, sizes)
			unlock()
			changed = err == nil
			if changed {
				details = selectedDimensionChangedDetails(start, end, len(sizes), row)
			}
		}
		a.completeSessionOperation(state, token, err, changed)
		select {
		case a.formatResults <- spreadsheetFormatResult{
			state:      state,
			token:      token,
			revision:   state.wordRevisionAtomic.Load(),
			generation: generation,
			sheet:      sheet,
			window:     window,
			err:        err,
			operation:  operation,
			applied:    changed,
			noOp:       noOp,
			details:    details,
			dimensions: changes,
		}:
		case <-a.shutdown:
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}) {
		state.finishOperation(token)
		a.setStatus("无法启动自动调整行列尺寸")
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	a.setStatus("正在" + operation + "…")
	if a.window != nil {
		a.window.Invalidate()
	}
}

var errGridAutoFitCanceled = errors.New("automatic dimension sizing canceled")

func selectedDimensionNoContentDetails(start, end int, row bool) string {
	name := "列"
	if row {
		name = "行"
	}
	if start == end {
		return "当前" + name + "没有可自动调整的内容"
	}
	return "所选" + name + "没有可自动调整的内容"
}

func selectedDimensionAlreadyFitDetails(start, end int, row bool) string {
	name := "列"
	if row {
		name = "行"
	}
	if start == end {
		return "第 " + strconv.Itoa(start) + " " + name + "已适合当前内容"
	}
	return "所选" + name + "已适合当前内容"
}

func selectedDimensionChangedDetails(start, end, changed int, row bool) string {
	name := "列"
	if row {
		name = "行"
	}
	if start == end {
		return "第 " + strconv.Itoa(start) + " " + name + "已按内容自动调整；点击保存写入原文件"
	}
	return "已按内容自动调整选中的 " + strconv.Itoa(changed) + " " + name + "；点击保存写入原文件"
}
