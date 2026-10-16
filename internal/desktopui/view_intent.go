package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"glance/internal/application"
	"glance/internal/document"
)

type desktopViewIntentRequest struct {
	intent application.ViewIntent
	result chan desktopViewIntentResult
}

type desktopViewIntentResult struct {
	data json.RawMessage
	err  error
}

func (a *App) HandleViewIntent(ctx context.Context, intent application.ViewIntent) (json.RawMessage, error) {
	if a == nil || a.window == nil || a.viewIntentRequests == nil || intent.WindowID != a.windowID {
		return nil, application.ErrNoAttachedView
	}
	request := desktopViewIntentRequest{intent: intent, result: make(chan desktopViewIntentResult, 1)}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-a.shutdown:
		return nil, application.ErrNoAttachedView
	case a.viewIntentRequests <- request:
	}
	a.window.Invalidate()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-a.shutdown:
		return nil, application.ErrNoAttachedView
	case result := <-request.result:
		return result.data, result.err
	}
}

func (a *App) processViewIntentRequests() {
	for {
		select {
		case request := <-a.viewIntentRequests:
			data, err := a.applyDesktopViewIntent(request.intent)
			request.result <- desktopViewIntentResult{data: data, err: err}
		default:
			return
		}
	}
}

func (a *App) applyDesktopViewIntent(intent application.ViewIntent) (json.RawMessage, error) {
	if a == nil || intent.WindowID != a.windowID {
		return nil, application.ErrNoAttachedView
	}
	state := a.documentSessionState
	if state == nil || state.documentID != intent.DocumentID {
		return nil, application.ErrViewContextMismatch
	}
	currentRevision := state.wordRevisionAtomic.Load()
	if intent.Revision == 0 || currentRevision != intent.Revision {
		return nil, fmt.Errorf("%w: current=%d expected=%d", application.ErrRevisionConflict, currentRevision, intent.Revision)
	}
	if state.word != nil {
		switch intent.Action {
		case "word.autosave.set", "word.view.get", "word.view.set", "word.zoom.dialog", "word.view.new_window", "word.view.side_by_side":
			return a.applyWordViewIntent(state, intent.Action, intent.Arguments)
		}
	}
	if state.jsonDoc != nil && intent.Action == "json.view.set" {
		return a.applyJSONViewIntent(intent.Arguments)
	}
	if state.markdownDoc != nil && intent.Action == "markdown.view.set" {
		return a.applyMarkdownViewIntent(intent.Arguments)
	}
	if state.pdfDoc != nil && (intent.Action == "pdf.view.get" || intent.Action == "pdf.view.set") {
		return a.applyPDFViewIntent(intent.Action, intent.Arguments)
	}
	if state.pptxDoc != nil && (intent.Action == "presentation.view.get" || intent.Action == "presentation.view.set") {
		return a.applyPresentationViewIntent(intent.Action, intent.Arguments)
	}
	if state.book == nil {
		return nil, application.ErrCapabilityUnsupported
	}
	if intent.Action == "spreadsheet.filter.apply" {
		return a.applySpreadsheetFilterViewIntent(state, intent.Arguments)
	}
	if intent.Action == "spreadsheet.zoom.get" {
		return json.Marshal(struct {
			WindowID    uint64 `json:"window_id"`
			ZoomPercent int    `json:"zoom_percent"`
			Changed     bool   `json:"changed"`
		}{a.windowID, int(math.Round(a.spreadsheetZoom() * 100)), false})
	}
	if intent.Action != "spreadsheet.zoom.apply" {
		return nil, application.ErrCapabilityUnsupported
	}
	var input struct {
		Operation   string `json:"operation"`
		ZoomPercent int    `json:"zoom_percent"`
	}
	if err := json.Unmarshal(intent.Arguments, &input); err != nil {
		return nil, application.ErrInvalidViewIntent
	}
	current := a.spreadsheetZoom()
	target := current
	switch input.Operation {
	case "zoom_in":
		if input.ZoomPercent != 0 {
			return nil, application.ErrInvalidCommandArguments
		}
		target = math.Min(spreadsheetZoomMaximum, current+0.1)
	case "reset":
		if input.ZoomPercent != 0 {
			return nil, application.ErrInvalidCommandArguments
		}
		target = 1
	case "set":
		if input.ZoomPercent < 50 || input.ZoomPercent > 200 {
			return nil, application.ErrInvalidCommandArguments
		}
		target = float64(input.ZoomPercent) / 100
	default:
		return nil, application.ErrInvalidCommandArguments
	}
	changed := current != target || state.spreadsheetSheetZoomPercent(state.sheet) != int(math.Round(target*100))
	if changed {
		a.setSpreadsheetZoom(target)
	}
	zoomPercent := int(math.Round(a.spreadsheetZoom() * 100))
	if input.Operation == "reset" {
		a.setStatus("缩放比例：100%")
	} else {
		a.setStatus(fmt.Sprintf("缩放比例：%d%%", zoomPercent))
	}
	result, err := json.Marshal(struct {
		WindowID    uint64 `json:"window_id"`
		ZoomPercent int    `json:"zoom_percent"`
		Changed     bool   `json:"changed"`
	}{a.windowID, zoomPercent, changed})
	return result, err
}

func (a *App) applyJSONViewIntent(raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		Operation string `json:"operation"`
		Level     int    `json:"level"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, application.ErrInvalidViewIntent
	}
	level := input.Level
	switch input.Operation {
	case "expand_all":
		if level != 0 {
			return nil, application.ErrInvalidCommandArguments
		}
		level = -1
	case "collapse_all":
		if level != 0 {
			return nil, application.ErrInvalidCommandArguments
		}
		level = 0
	case "expand_level":
		if level < 1 || level > 3 {
			return nil, application.ErrInvalidCommandArguments
		}
	default:
		return nil, application.ErrInvalidCommandArguments
	}
	view := a.ensureJSONView()
	changed := view.expandedLevel != level || len(view.collapsed) != 0
	view.expandedLevel = level
	view.collapsed = make(map[int]bool)
	view.rebuildRows()
	a.window.Invalidate()
	return json.Marshal(struct {
		WindowID  uint64 `json:"window_id"`
		Operation string `json:"operation"`
		Level     int    `json:"level"`
		Changed   bool   `json:"changed"`
	}{WindowID: a.windowID, Operation: input.Operation, Level: level, Changed: changed})
}

func (a *App) applyMarkdownViewIntent(raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, application.ErrInvalidViewIntent
	}
	mode, ok := map[string]int{"edit": 0, "preview": 1, "split": 2}[input.Mode]
	if !ok {
		return nil, application.ErrInvalidCommandArguments
	}
	view := a.ensureMarkdownView()
	changed := view.mode != mode
	view.mode = mode
	view.focus = mode != 1
	a.persistActiveSession()
	a.window.Invalidate()
	return json.Marshal(struct {
		WindowID uint64 `json:"window_id"`
		Mode     string `json:"mode"`
		Changed  bool   `json:"changed"`
	}{WindowID: a.windowID, Mode: input.Mode, Changed: changed})
}

func (a *App) applySpreadsheetFilterViewIntent(state *documentSessionState, raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		Operation        string             `json:"operation"`
		Sheet            string             `json:"sheet"`
		Bounds           document.CellRange `json:"bounds"`
		Column           int                `json:"column"`
		Value            string             `json:"value"`
		VisibleRows      []int              `json:"visible_rows"`
		RowHeightsPoints []float64          `json:"row_heights_points"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, application.ErrInvalidViewIntent
	}
	if input.Sheet == "" || input.Sheet != state.sheet || state.virtualGrid == nil {
		return nil, application.ErrViewContextMismatch
	}
	changed := false
	rowsVisible := 0
	switch input.Operation {
	case "clear":
		if state.valueFilter != nil {
			changed = true
			a.clearValueFilter(state)
		}
		a.setStatus("已清除筛选")
	case "set":
		rows := input.Bounds.EndRow - input.Bounds.StartRow + 1
		columns := input.Bounds.EndColumn - input.Bounds.StartColumn + 1
		if input.Bounds.StartRow < 1 || input.Bounds.StartColumn < 1 || input.Bounds.EndRow < input.Bounds.StartRow || input.Bounds.EndColumn < input.Bounds.StartColumn || rows > 10_000 || columns > 10_000 || rows*columns > 10_000 || input.Column < input.Bounds.StartColumn || input.Column > input.Bounds.EndColumn || len(input.RowHeightsPoints) != 0 && len(input.RowHeightsPoints) != rows {
			return nil, application.ErrInvalidCommandArguments
		}
		visible := make(map[int]bool, len(input.VisibleRows))
		for _, row := range input.VisibleRows {
			if row <= input.Bounds.StartRow || row > input.Bounds.EndRow {
				return nil, application.ErrInvalidCommandArguments
			}
			visible[row] = true
		}
		rowsVisible = len(visible)
		if state.valueFilter != nil && sameSpreadsheetValueFilter(state.valueFilter, input.Sheet, input.Bounds, input.Column, input.Value, visible) {
			break
		}
		filter := &ValueFilter{
			Sheet: input.Sheet, Bounds: input.Bounds, Column: input.Column, Value: input.Value,
			VisibleRows: visible, RowHeightsPoints: append([]float64(nil), input.RowHeightsPoints...),
		}
		a.installSpreadsheetValueFilter(state, filter)
		changed = true
		a.setStatus("已完成筛选")
	default:
		return nil, application.ErrInvalidCommandArguments
	}
	return json.Marshal(struct {
		WindowID    uint64 `json:"window_id"`
		Operation   string `json:"operation"`
		Sheet       string `json:"sheet"`
		RowsVisible int    `json:"rows_visible"`
		Changed     bool   `json:"changed"`
	}{a.windowID, input.Operation, input.Sheet, rowsVisible, changed})
}

func sameSpreadsheetValueFilter(current *ValueFilter, sheet string, bounds document.CellRange, column int, value string, visible map[int]bool) bool {
	if current == nil || current.Sheet != sheet || current.Bounds != bounds || current.Column != column || current.Value != value || len(current.VisibleRows) != len(visible) {
		return false
	}
	for row := range visible {
		if !current.VisibleRows[row] {
			return false
		}
	}
	return true
}
