package desktopui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"glance/internal/document"
)

const spreadsheetSelectionStatsDebounce = 90 * time.Millisecond

type spreadsheetSelectionStatsRequestKey struct {
	Generation uint64
	Revision   uint64
	Sheet      string
	Bounds     document.CellRange
	Sequence   uint64
}

type spreadsheetSelectionStatsResult struct {
	Key   spreadsheetSelectionStatsRequestKey
	Stats document.SpreadsheetSelectionStats
	Err   error
}

func (key spreadsheetSelectionStatsRequestKey) sameSelection(other spreadsheetSelectionStatsRequestKey) bool {
	return key.Generation == other.Generation && key.Revision == other.Revision && key.Sheet == other.Sheet && key.Bounds == other.Bounds
}

func (state *documentSessionState) cancelSpreadsheetSelectionStats() {
	if state == nil {
		return
	}
	if state.spreadsheetSelectionStatsCancel != nil {
		state.spreadsheetSelectionStatsCancel()
		state.spreadsheetSelectionStatsCancel = nil
	}
	state.spreadsheetSelectionStatsPending = false
	if state.spreadsheetSelectionStatsResults == nil {
		return
	}
	for {
		select {
		case <-state.spreadsheetSelectionStatsResults:
		default:
			return
		}
	}
}

func (a *App) spreadsheetStatusSelectionStatsText() string {
	if a == nil {
		return ""
	}
	state := a.documentSessionState
	if state == nil || state.closed.Load() || state.book == nil || !state.selectionSet || a.selectionDragging {
		if state != nil {
			state.cancelSpreadsheetSelectionStats()
			state.spreadsheetSelectionStatsKeySet = false
			state.spreadsheetSelectionStatsSet = false
		}
		return ""
	}
	reader, ok := state.book.(document.SpreadsheetSelectionStatsReader)
	if !ok {
		state.cancelSpreadsheetSelectionStats()
		state.spreadsheetSelectionStatsKeySet = false
		state.spreadsheetSelectionStatsSet = false
		return ""
	}
	bounds := state.selection.Bounds()
	cellCount := int64(bounds.EndRow-bounds.StartRow+1) * int64(bounds.EndColumn-bounds.StartColumn+1)
	if cellCount <= 1 {
		state.cancelSpreadsheetSelectionStats()
		state.spreadsheetSelectionStatsKeySet = false
		state.spreadsheetSelectionStatsSet = false
		return ""
	}
	current := spreadsheetSelectionStatsRequestKey{
		Generation: a.documentGeneration,
		Revision:   state.wordRevisionAtomic.Load(),
		Sheet:      state.sheet,
		Bounds:     bounds,
	}
	if state.spreadsheetSelectionStatsKeySet && state.spreadsheetSelectionStatsKey.sameSelection(current) {
		if state.spreadsheetSelectionStatsSet {
			return formatSpreadsheetSelectionStats(state.spreadsheetSelectionStats)
		}
		if state.spreadsheetSelectionStatsPending {
			return ""
		}
	}
	state.cancelSpreadsheetSelectionStats()
	state.spreadsheetSelectionStatsSequence++
	current.Sequence = state.spreadsheetSelectionStatsSequence
	state.spreadsheetSelectionStatsKey = current
	state.spreadsheetSelectionStatsKeySet = true
	state.spreadsheetSelectionStatsSet = false
	state.spreadsheetSelectionStatsPending = true
	state.spreadsheetSelectionStats = document.SpreadsheetSelectionStats{}
	if state.spreadsheetSelectionStatsResults == nil {
		state.spreadsheetSelectionStatsResults = make(chan spreadsheetSelectionStatsResult, 1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	state.spreadsheetSelectionStatsCancel = cancel
	if !a.launchManagedBackground(func() {
		select {
		case <-a.shutdown:
			cancel()
		case <-ctx.Done():
		}
	}) {
		cancel()
		state.spreadsheetSelectionStatsPending = false
		return ""
	}
	results := state.spreadsheetSelectionStatsResults
	shutdown := a.shutdown
	window := a.window
	if !a.launchManagedBackground(func() {
		defer cancel()
		timer := time.NewTimer(spreadsheetSelectionStatsDebounce)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-shutdown:
			return
		case <-timer.C:
		}
		stats, err := reader.SelectionStats(ctx, current.Sheet, current.Bounds)
		if ctx.Err() != nil {
			return
		}
		select {
		case results <- spreadsheetSelectionStatsResult{Key: current, Stats: stats, Err: err}:
			if window != nil {
				window.Invalidate()
			}
		case <-ctx.Done():
		case <-shutdown:
		}
	}) {
		cancel()
		state.spreadsheetSelectionStatsPending = false
		return ""
	}
	return ""
}

func (a *App) processSpreadsheetSelectionStatsResults() {
	if a == nil || a.documentSessionState == nil {
		return
	}
	state := a.documentSessionState
	if state.spreadsheetSelectionStatsResults == nil {
		return
	}
	select {
	case result := <-state.spreadsheetSelectionStatsResults:
		key := result.Key
		if state.closed.Load() || !state.spreadsheetSelectionStatsPending ||
			!state.spreadsheetSelectionStatsKeySet || key != state.spreadsheetSelectionStatsKey ||
			key.Generation != a.documentGeneration || key.Revision != state.wordRevisionAtomic.Load() ||
			key.Sheet != state.sheet || !state.selectionSet || key.Bounds != state.selection.Bounds() {
			return
		}
		state.spreadsheetSelectionStatsPending = false
		state.spreadsheetSelectionStatsSet = true
		if result.Err == nil {
			state.spreadsheetSelectionStats = result.Stats
		}
		if state.spreadsheetSelectionStatsCancel != nil {
			state.spreadsheetSelectionStatsCancel()
			state.spreadsheetSelectionStatsCancel = nil
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	default:
	}
}

func formatSpreadsheetSelectionStats(stats document.SpreadsheetSelectionStats) string {
	if stats.Count == 0 {
		return ""
	}
	parts := make([]string, 0, 3)
	if stats.NumericCount > 0 {
		parts = append(parts, "平均值 "+formatSpreadsheetStatusNumber(stats.Sum/float64(stats.NumericCount)))
	}
	parts = append(parts, "计数 "+strconv.FormatInt(stats.Count, 10))
	if stats.NumericCount > 0 {
		parts = append(parts, "求和 "+formatSpreadsheetStatusNumber(stats.Sum))
	}
	return strings.Join(parts, "  ·  ")
}

func formatSpreadsheetStatusNumber(value float64) string {
	text := fmt.Sprintf("%.2f", value)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "-0" {
		return "0"
	}
	return text
}
