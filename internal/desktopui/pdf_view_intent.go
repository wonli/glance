package desktopui

import (
	"encoding/json"
	"glance/internal/i18n"
	"math"

	"glance/internal/application"
	"glance/internal/document"
)

func (a *App) applyPDFViewIntent(action string, raw json.RawMessage) (json.RawMessage, error) {
	if a == nil {
		return nil, application.ErrNoAttachedView
	}
	state := a.documentSessionState
	if a.window == nil || state == nil || state.pdfDoc == nil || state.pdfDoc != a.pdfDoc {
		return nil, application.ErrNoAttachedView
	}
	v := a.ensurePDFView()
	info := state.pdfDoc.Info()
	v.syncInfo(info)
	if action == "pdf.view.get" {
		var input map[string]json.RawMessage
		if err := json.Unmarshal(raw, &input); err != nil || input == nil || len(input) != 0 {
			return nil, application.ErrInvalidViewIntent
		}
		return marshalPDFViewState(a.windowID, v, info, false)
	}
	if action != "pdf.view.set" || application.ValidatePDFViewArguments(raw) != nil {
		return nil, application.ErrInvalidViewIntent
	}
	var input struct {
		Operation   string `json:"operation"`
		PageNumber  *int   `json:"page_number"`
		ZoomPercent *int   `json:"zoom_percent"`
		Enabled     *bool  `json:"enabled"`
		Mode        string `json:"mode"`
		Tool        string `json:"tool"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, application.ErrInvalidViewIntent
	}
	oldPage, oldZoom, oldFit, oldMode, oldTool, oldMessage, oldMessageRefID, oldThumbnails, oldSearchVisible := v.page, v.zoom, v.fit, v.mode, v.tool, v.message, v.messageRef.ID, v.thumbnails, v.showSearch
	switch input.Operation {
	case "set_page":
		if *input.PageNumber > info.Pages {
			return nil, application.ErrInvalidCommandArguments
		}
		v.page = *input.PageNumber - 1
	case "previous":
		v.page = max(0, v.page-1)
	case "next":
		v.page = min(info.Pages-1, v.page+1)
	case "set_zoom":
		v.zoom = float64(*input.ZoomPercent) / 100
		v.fit = ""
	case "zoom_in", "zoom_out":
		zoomPercent := int(math.Round(v.zoom * 100))
		if zoomPercent < 25 || zoomPercent > 400 {
			zoomPercent = 100
		}
		if input.Operation == "zoom_in" {
			zoomPercent = min(400, zoomPercent+10)
		} else {
			zoomPercent = max(25, zoomPercent-10)
		}
		v.zoom = float64(zoomPercent) / 100
		v.fit = ""
	case "reset_zoom":
		v.zoom = 1
		v.fit = ""
	case "fit_width":
		v.fit = "width"
	case "fit_page":
		v.fit = "page"
	case "set_thumbnails":
		v.thumbnails = *input.Enabled
	case "set_search_visible":
		v.showSearch = *input.Enabled
	case "set_mode":
		v.mode = input.Mode
		v.tool = ""
		v.message = ""
		v.messageRef = i18n.Message{}
		if input.Mode == "signature" {
			v.tool = "ink"
			v.messageRef = i18n.Message{ID: "pdf.tool.signature_hint"}
		}
	case "set_tool":
		v.mode = "annotate"
		v.tool = input.Tool
		if input.Tool == "highlight" {
			v.message = ""
			v.messageRef = i18n.Message{ID: "pdf.tool.highlight_hint"}
		} else if input.Tool == "ink" {
			v.message = ""
			v.messageRef = i18n.Message{ID: "pdf.tool.ink_hint"}
		} else {
			v.message = ""
			v.messageRef = i18n.Message{}
		}
	}
	changed := oldPage != v.page || oldZoom != v.zoom || oldFit != v.fit || oldMode != v.mode || oldTool != v.tool || oldMessage != v.message || oldMessageRefID != v.messageRef.ID || oldThumbnails != v.thumbnails || oldSearchVisible != v.showSearch
	if oldPage != v.page {
		v.canvas.Position.First = v.page
		v.canvas.Position.Offset = 0
		v.canvas.Position.OffsetLast = 0
		v.canvas.Position.BeforeEnd = false
		v.followPDFThumbnailPage(v.page)
		v.clearSelection()
		v.trimCacheWindow()
	}
	if changed {
		a.window.Invalidate()
	}
	info = state.pdfDoc.Info()
	return marshalPDFViewState(a.windowID, v, info, changed)
}

func marshalPDFViewState(windowID uint64, view *pdfViewState, info document.PDFInfo, changed bool) (json.RawMessage, error) {
	fitMode := view.fit
	if fitMode == "" {
		fitMode = "custom"
	}
	mode := view.mode
	if mode == "" {
		mode = "read"
	}
	pageCount := max(1, info.Pages)
	pageNumber := min(pageCount, max(1, view.page+1))
	return json.Marshal(struct {
		WindowID      uint64 `json:"window_id"`
		PageNumber    int    `json:"page_number"`
		PageCount     int    `json:"page_count"`
		ZoomPercent   int    `json:"zoom_percent"`
		FitMode       string `json:"fit_mode"`
		Mode          string `json:"mode"`
		Tool          string `json:"tool"`
		Thumbnails    bool   `json:"thumbnails"`
		SearchVisible bool   `json:"search_visible"`
		Changed       bool   `json:"changed"`
	}{
		WindowID: windowID, PageNumber: pageNumber, PageCount: pageCount,
		ZoomPercent: int(math.Round(view.zoom * 100)), FitMode: fitMode,
		Mode: mode, Tool: view.tool, Thumbnails: view.thumbnails, SearchVisible: view.showSearch, Changed: changed,
	})
}

func (a *App) runPDFViewAction(operation string, arguments any) {
	raw, err := json.Marshal(arguments)
	if err == nil {
		_, err = a.applyPDFViewIntent("pdf.view.set", raw)
	}
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "pdf.view.error.operation_failed", Args: i18n.Args{"Operation": operation}}, err, notificationToast)
	}
}
