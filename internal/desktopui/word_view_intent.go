package desktopui

import (
	"encoding/json"
	"fmt"
	"math"

	"glance/internal/application"
)

func (a *App) applyWordViewIntent(state *documentSessionState, action string, raw json.RawMessage) (json.RawMessage, error) {
	if state == nil || state.word == nil {
		return nil, application.ErrCapabilityUnsupported
	}
	switch action {
	case "word.autosave.set":
		return a.applyWordAutosaveSettings(state, raw)
	case "word.view.get":
		return marshalWordViewState(a.windowID, state, false)
	case "word.zoom.dialog":
		a.showWordZoomDialog()
		return json.Marshal(struct {
			WindowID uint64 `json:"window_id"`
			Opened   bool   `json:"opened"`
		}{a.windowID, true})
	case "word.view.new_window":
		if err := a.openWordNewWindow(); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			SourceWindowID uint64 `json:"source_window_id"`
			DocumentID     string `json:"document_id"`
			Requested      bool   `json:"requested"`
		}{a.windowID, state.documentID, true})
	case "word.view.side_by_side":
		strategy := "two_page"
		if a.desktopRuntime != nil && a.desktopRuntime.windowCount() > 1 {
			strategy = "existing_windows"
			a.setStatus("已开启并排查看模式")
		} else if enabled, _ := wordNewWindowAvailability(state, a.desktopRuntime); enabled {
			if err := a.openWordNewWindow(); err != nil {
				return nil, err
			}
			strategy = "new_window"
			a.setStatus("已打开新窗口并排查看")
		} else {
			a.selectWordMultiplePagesView()
			a.setStatus("已开启双页并排查看")
		}
		return json.Marshal(struct {
			WindowID uint64 `json:"window_id"`
			Strategy string `json:"strategy"`
		}{a.windowID, strategy})
	case "word.view.set":
		return a.applyWordViewSettings(state, raw)
	default:
		return nil, application.ErrCapabilityUnsupported
	}
}

func (a *App) applyWordAutosaveSettings(state *documentSessionState, raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.Enabled == nil {
		return nil, application.ErrInvalidCommandArguments
	}
	changed := state.wordAutoSave != *input.Enabled
	if changed {
		state.wordAutoSave = *input.Enabled
		switch {
		case !*input.Enabled:
			a.setStatus("已关闭自动保存")
		case sessionPath(state) == "":
			a.setStatus("自动保存将在文档另存为后启用")
		default:
			a.setStatus("已开启自动保存")
		}
		a.persistActiveSession()
	}
	return json.Marshal(struct {
		WindowID  uint64 `json:"window_id"`
		Enabled   bool   `json:"enabled"`
		Effective bool   `json:"effective"`
		Changed   bool   `json:"changed"`
	}{a.windowID, state.wordAutoSave, state.wordAutoSave && sessionPath(state) != "", changed})
}

func (a *App) applyWordViewSettings(state *documentSessionState, raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		Mode              *string                   `json:"mode"`
		ReadingMode       *bool                     `json:"reading_mode"`
		RulerVisible      *bool                     `json:"ruler_visible"`
		GridlinesVisible  *bool                     `json:"gridlines_visible"`
		NavigationVisible *bool                     `json:"navigation_visible"`
		MultiplePages     *bool                     `json:"multiple_pages"`
		ZoomPercent       *int                      `json:"zoom_percent"`
		Selection         *wordTextSelectionRequest `json:"selection"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, application.ErrInvalidViewIntent
	}
	if input.Mode == nil && input.ReadingMode == nil && input.RulerVisible == nil && input.GridlinesVisible == nil && input.NavigationVisible == nil && input.MultiplePages == nil && input.ZoomPercent == nil && input.Selection == nil {
		return nil, application.ErrInvalidCommandArguments
	}
	if input.Mode != nil && *input.Mode != "continuous" && *input.Mode != "print" && *input.Mode != "web" {
		return nil, application.ErrInvalidCommandArguments
	}
	if input.ZoomPercent != nil && (*input.ZoomPercent < 50 || *input.ZoomPercent > 150) {
		return nil, application.ErrInvalidCommandArguments
	}
	if (input.MultiplePages != nil && input.Mode != nil && *input.Mode != "print") || (input.MultiplePages != nil && input.ZoomPercent != nil && *input.MultiplePages) {
		return nil, application.ErrInvalidCommandArguments
	}
	changed := false
	if input.Selection != nil {
		var err error
		changed, err = a.setWordTextSelection(state, *input.Selection)
		if err != nil {
			return nil, err
		}
		switch input.Selection.Scope {
		case "document":
			a.setStatus("已全选文档正文")
		case "range":
			a.setStatus("已选择正文范围")
		case "clear":
			a.setStatus("已清除正文选择")
		}
	}
	if input.Mode != nil {
		var mode wordViewMode
		switch *input.Mode {
		case "continuous":
			mode = wordViewContinuous
		case "print":
			mode = wordViewPrintLayout
		case "web":
			mode = wordViewWebLayout
		default:
			return nil, application.ErrInvalidCommandArguments
		}
		if state.wordViewMode != mode {
			if mode == wordViewPrintLayout && state.wordViewMode != wordViewPrintLayout {
				a.toggleWordPrintLayout()
			} else if mode == wordViewWebLayout && state.wordViewMode != wordViewWebLayout {
				a.toggleWordWebLayout()
			} else if mode == wordViewContinuous && state.wordViewMode == wordViewPrintLayout {
				a.toggleWordPrintLayout()
			} else if mode == wordViewContinuous && state.wordViewMode == wordViewWebLayout {
				a.toggleWordWebLayout()
			}
			changed = true
		}
	}
	if input.ReadingMode != nil && state.wordReadingMode != *input.ReadingMode {
		state.wordReadingMode = *input.ReadingMode
		state.wordToolbarColorPicker = false
		state.wordToolbarHighlightPicker = false
		if *input.ReadingMode {
			a.setStatus("已进入阅读模式")
		} else {
			a.setStatus("已退出阅读模式")
		}
		changed = true
	}
	if input.RulerVisible != nil && (!state.wordRulerHidden) != *input.RulerVisible {
		state.wordRulerHidden = !*input.RulerVisible
		if *input.RulerVisible {
			a.setStatus("已显示标尺")
		} else {
			a.setStatus("已隐藏标尺")
		}
		changed = true
	}
	if input.GridlinesVisible != nil && state.wordGridlinesVisible != *input.GridlinesVisible {
		state.wordGridlinesVisible = *input.GridlinesVisible
		if *input.GridlinesVisible {
			a.setStatus("已显示网格线")
		} else {
			a.setStatus("已隐藏网格线")
		}
		changed = true
	}
	if input.NavigationVisible != nil && state.wordNavigationVisible != *input.NavigationVisible {
		state.wordNavigationVisible = *input.NavigationVisible
		if *input.NavigationVisible {
			a.setStatus("已打开导航窗格")
		} else {
			a.setStatus("已关闭导航窗格")
		}
		changed = true
	}
	if input.MultiplePages != nil {
		if *input.MultiplePages && (!state.wordMultiplePagesMode || state.wordViewMode != wordViewPrintLayout || !state.wordPageFitMode) {
			a.selectWordMultiplePagesView()
			changed = true
		} else if !*input.MultiplePages && (state.wordMultiplePagesMode || state.wordViewMode != wordViewPrintLayout || !state.wordPageFitMode) {
			a.selectWordSinglePageView()
			changed = true
		}
	}
	if input.ZoomPercent != nil {
		target := wordStatusZoomClamp(float64(*input.ZoomPercent) / 100)
		if math.Abs(state.wordLayoutZoom-target) > 0.0001 || state.wordPageFitMode || state.wordMultiplePagesMode {
			setWordManualZoom(state, target)
			a.setStatus(fmt.Sprintf("缩放：%d%%", *input.ZoomPercent))
			changed = true
		}
	}
	if changed {
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	return marshalWordViewState(a.windowID, state, changed)
}

func marshalWordViewState(windowID uint64, state *documentSessionState, changed bool) (json.RawMessage, error) {
	mode := "continuous"
	switch state.wordViewMode {
	case wordViewPrintLayout:
		mode = "print"
	case wordViewWebLayout:
		mode = "web"
	}
	return json.Marshal(struct {
		WindowID         uint64                    `json:"window_id"`
		Mode             string                    `json:"mode"`
		ReadingMode      bool                      `json:"reading_mode"`
		RulerVisible     bool                      `json:"ruler_visible"`
		GridlinesVisible bool                      `json:"gridlines_visible"`
		Navigation       bool                      `json:"navigation_visible"`
		PageFit          bool                      `json:"page_fit"`
		MultiplePages    bool                      `json:"multiple_pages"`
		ZoomPercent      int                       `json:"zoom_percent"`
		Changed          bool                      `json:"changed"`
		Selection        wordSelectionSnapshotData `json:"selection"`
	}{
		WindowID: windowID, Mode: mode, ReadingMode: state.wordReadingMode,
		RulerVisible: !state.wordRulerHidden, GridlinesVisible: state.wordGridlinesVisible,
		Navigation: state.wordNavigationVisible, PageFit: state.wordPageFitMode,
		MultiplePages: state.wordMultiplePagesMode, ZoomPercent: int(math.Round(wordStatusZoomClamp(state.wordLayoutZoom) * 100)), Changed: changed,
		Selection: wordSelectionSnapshot(state),
	})
}
