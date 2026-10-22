package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/i18n"
	"io"
	"math"
	"strconv"
	"strings"
)

func (a *App) startPDFCommand(c document.PDFCommand) bool {
	v := a.ensurePDFView()
	if v.pending || a.isSaving() {
		return false
	}
	state := a.documentSessionState
	if state == nil || state.pdfDoc == nil {
		return false
	}
	runtime := a.ensureDesktopRuntime()
	if runtime.applicationRuntimeErr != nil || runtime.applicationRuntime == nil {
		a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_pdf_agent_capability_catalog_is_unavai_c1dcaf"}, feedbackWarning, notificationToast)
		return false
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return false
	}
	if c.Revision != 0 && c.Revision != token.revision {
		state.finishOperation(token)
		a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_pdf_page_changed_select_the_text_again_901e07"}, feedbackInfo, notificationSuppressed)
		return false
	}
	capabilityID, arguments, err := pdfCommandCapability(c)
	if err != nil {
		state.finishOperation(token)
		a.setStatusErrorMessage(i18n.Message{ID: "pdf.error.operation_failed"}, err, notificationToast)
		return false
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		state.finishOperation(token)
		return false
	}
	v.pending = true
	v.lastCommand = c
	window := a.window
	shutdown := a.shutdown
	done := v.workerSignal()
	commandContext, cancel := context.WithCancel(context.Background())
	a.launchManagedBackground(func() {
		defer cancel()
		a.launchManagedBackground(func() {
			select {
			case <-shutdown:
				cancel()
			case <-commandContext.Done():
			}
		})
		invocation, e := runtime.applicationRuntime.InvokeCapability(commandContext, application.CapabilityRequest{
			RequestID:  fmt.Sprintf("pdf-ribbon-%s-%d", token.documentID, token.generation),
			DocumentID: token.documentID, CapabilityID: capabilityID, ExpectedRevision: token.revision,
			Operator: application.Operator{ID: fmt.Sprintf("window-%d", a.windowID), Kind: application.OperatorHuman},
			View: application.ViewContext{
				WindowID: a.windowID, DocumentID: token.documentID, Revision: token.revision,
				Selection: &application.SelectionContext{TargetType: "pdf_page", TargetID: strconv.Itoa(c.Page + 1)},
			},
			Arguments: encoded,
		})
		changed := false
		if e == nil {
			var result struct {
				Changed bool `json:"changed"`
			}
			if json.Unmarshal(invocation.Data, &result) != nil {
				e = application.ErrInvalidCommandResult
			} else {
				changed = result.Changed
			}
		}
		state.finishOperation(token)
		if changed && state.ownerRuntime != nil {
			state.ownerRuntime.invalidateSessionViews(state.documentID)
		}
		result := pdfPageResult{mutation: changed, err: e, info: state.pdfDoc.Info()}
		if e == nil && !changed {
			result.notice = i18n.Message{ID: "pdf.status.no_change"}
		}
		select {
		case v.results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-done:
		case <-shutdown:
		}
	})
	return true
}

type pdfHistoryApplyPayload struct {
	Operation string `json:"operation"`
}

type pdfInkAnnotationPayload struct {
	PageNumber int                     `json:"page_number"`
	Points     []pdfInkAnnotationPoint `json:"points"`
}

type pdfInkAnnotationPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func pdfInkAnnotationPoints(points []document.PDFPoint) []pdfInkAnnotationPoint {
	if len(points) == 0 {
		return nil
	}
	result := make([]pdfInkAnnotationPoint, len(points))
	for i, point := range points {
		result[i] = pdfInkAnnotationPoint{X: point.X, Y: point.Y}
	}
	return result
}

type pdfAnnotationBounds struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type pdfHighlightAnnotationPayload struct {
	PageNumber int                 `json:"page_number"`
	Bounds     pdfAnnotationBounds `json:"bounds"`
	Text       string              `json:"text"`
}

type pdfNoteAnnotationPayload struct {
	PageNumber int     `json:"page_number"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Text       string  `json:"text"`
}

type pdfTextReplacePayload struct {
	PageNumber int    `json:"page_number"`
	Run        int    `json:"run"`
	Text       string `json:"text"`
}

type pdfPageRotatePayload struct {
	PageNumber int `json:"page_number"`
}

func pdfCommandCapability(command document.PDFCommand) (string, any, error) {
	switch command.Kind {
	case "rotate":
		return "pdf.page.rotate", pdfPageRotatePayload{PageNumber: command.Page + 1}, nil
	case "replace":
		return "pdf.text.replace", pdfTextReplacePayload{PageNumber: command.Page + 1, Run: command.Run, Text: command.Text}, nil
	case "note":
		return "pdf.annotation.note", pdfNoteAnnotationPayload{PageNumber: command.Page + 1, X: command.X, Y: command.Y, Text: command.Text}, nil
	case "highlight":
		return "pdf.annotation.highlight", pdfHighlightAnnotationPayload{PageNumber: command.Page + 1, Bounds: pdfAnnotationBounds{X: command.X, Y: command.Y, W: command.W, H: command.H}, Text: command.Text}, nil
	case "ink":
		return "pdf.annotation.ink", pdfInkAnnotationPayload{PageNumber: command.Page + 1, Points: pdfInkAnnotationPoints(command.Points)}, nil
	case "undo", "redo":
		return "pdf.history.apply", pdfHistoryApplyPayload{Operation: command.Kind}, nil
	default:
		return "", nil, fmt.Errorf("unsupported PDF command %q", command.Kind)
	}
}
func (a *App) pdfTextDialog(kind string) {
	v := a.ensurePDFView()
	p := v.scene
	initial := ""
	run := -1
	x, y := 40.0, 40.0
	if v.selected >= 0 && v.selected < len(p.Text) {
		t := p.Text[v.selected]
		run = t.Run
		x, y = t.Bounds.X, t.Bounds.Y
		if kind == "replace" {
			if !t.Editable {
				a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.this_text_cannot_be_edited_in_place_yet_f672e7"}, feedbackWarning, notificationToast)
				return
			}
			initial = t.Text
		}
	}
	if kind == "replace" && run < 0 {
		return
	}

	a.showPDFDraft(document.PDFCommand{Kind: kind, Page: p.Index, Revision: p.Revision, Run: run, Text: initial, X: x, Y: y})
}

func (a *App) showPDFDraft(command document.PDFCommand) {
	title := i18n.Message{ID: "pdf.annotation.dialog.title"}
	if command.Kind == "replace" {
		title = i18n.Message{ID: "pdf.text.replace.dialog.title"}
	}
	state := a.documentSessionState
	dialogID := "pdf.annotation.draft"
	if command.Kind == "replace" {
		dialogID = "pdf.text.replace"
	}
	a.modal.ShowInputMessage(dialogID, title, i18n.Message{ID: "pdf.draft.dialog.body"}, command.Text, i18n.Message{ID: "pdf.draft.dialog.hint"}, func(action string) {
		if action != "confirm" {
			a.modal.Hide()
			return
		}
		if a.documentSessionState != state {
			return
		}
		command.Text = a.modal.Input.Text()
		if a.startPDFCommand(command) {
			a.modal.Hide()
		} else {
			a.modal.SetMessageRef(i18n.Message{ID: "pdf.draft.error.busy"})
		}
	}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.apply"}, Style: modalDialogActionPrimary})
}

type handlePDFActionSetToolArguments struct {
	Operation string `json:"operation"`
	Tool      string `json:"tool"`
}

type setModeSetModeArguments struct {
	Operation string `json:"operation"`
	Mode      string `json:"mode"`
}

type fitPageFitPageArguments struct {
	Operation string `json:"operation"`
}

type fitWidthFitWidthArguments struct {
	Operation string `json:"operation"`
}

type resetZoomResetZoomArguments struct {
	Operation string `json:"operation"`
}

type zoomInSetZoomArguments struct {
	Operation   string `json:"operation"`
	ZoomPercent int    `json:"zoom_percent"`
}

type zoomOutSetZoomArguments struct {
	Operation   string `json:"operation"`
	ZoomPercent int    `json:"zoom_percent"`
}

type handlePDFActionArguments struct {
	Operation string `json:"operation"`
}

type handlePDFActionSetSearchVisibleArguments struct {
	Operation string `json:"operation"`
	Enabled   bool   `json:"enabled"`
}

type setThumbnailsSetThumbnailsArguments struct {
	Operation string `json:"operation"`
	Enabled   bool   `json:"enabled"`
}

type setPageSetPageArguments struct {
	Operation  string `json:"operation"`
	PageNumber int    `json:"page_number"`
}

func (a *App) handlePDFAction(gtx layout.Context, id string) {
	v := a.ensurePDFView()
	if strings.HasPrefix(id, "page-") {
		n, e := strconv.Atoi(strings.TrimPrefix(id, "page-"))
		if e == nil {
			a.runPDFViewAction("set_page", setPageSetPageArguments{Operation: "set_page", PageNumber: n + 1})
		}
		return
	}
	switch id {
	case "proofread":
		if err := a.openProofreadingWindow(); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "pdf.error.operation_failed"}, err, notificationToast)
		}
	case "open":
		a.startOpenFilePicker()
	case "close":
		a.closeWorkspaceTab(a.activeTabID)
	case "thumbnails":
		a.runPDFViewAction("set_thumbnails", setThumbnailsSetThumbnailsArguments{Operation: "set_thumbnails", Enabled: !v.thumbnails})
	case "search":
		a.runPDFViewAction("search", handlePDFActionSetSearchVisibleArguments{Operation: "set_search_visible", Enabled: !v.showSearch})
	case "find":
		a.startPDFSearch()
	case "previous":
		a.runPDFViewAction("previous", handlePDFActionArguments{Operation: "previous"})
	case "next":
		a.runPDFViewAction("next", handlePDFActionArguments{Operation: "next"})
	case "zoom-out":
		zoom := int(math.Round(min(4, max(.25, v.currentZoom(gtx.Metric)-.1)) * 100))
		a.runPDFViewAction("zoom-out", zoomOutSetZoomArguments{Operation: "set_zoom", ZoomPercent: zoom})
	case "zoom-in":
		zoom := int(math.Round(min(4, v.currentZoom(gtx.Metric)+.1) * 100))
		a.runPDFViewAction("zoom-in", zoomInSetZoomArguments{Operation: "set_zoom", ZoomPercent: zoom})
	case "zoom-indicator", "zoom-100", "actual-size":
		a.runPDFViewAction("reset_zoom", resetZoomResetZoomArguments{Operation: "reset_zoom"})
	case "fit-width":
		a.runPDFViewAction("fit_width", fitWidthFitWidthArguments{Operation: "fit_width"})
	case "fit-page":
		a.runPDFViewAction("fit_page", fitPageFitPageArguments{Operation: "fit_page"})
	case "read", "edit", "annotate", "signature":
		a.runPDFViewAction("set_mode", setModeSetModeArguments{Operation: "set_mode", Mode: id})
	case "ink":
		a.runPDFViewAction("ink", handlePDFActionSetToolArguments{Operation: "set_tool", Tool: "ink"})
	case "highlight":
		start, end := v.selectionRange()
		if start < 0 || end < 0 || len(v.scene.Text) == 0 {
			a.runPDFViewAction("highlight", handlePDFActionSetToolArguments{Operation: "set_tool", Tool: "highlight"})
			return
		}
		start = max(0, min(start, len(v.scene.Text)-1))
		end = max(0, min(end, len(v.scene.Text)-1))
		first := v.scene.Text[start].Bounds
		left, top, right, bottom := first.X, first.Y, first.X+first.W, first.Y+first.H
		for i := start + 1; i <= end; i++ {
			bounds := v.scene.Text[i].Bounds
			left = min(left, bounds.X)
			top = min(top, bounds.Y)
			right = max(right, bounds.X+bounds.W)
			bottom = max(bottom, bounds.Y+bounds.H)
		}
		a.startPDFCommand(document.PDFCommand{
			Kind: "highlight", Page: v.page, Revision: v.scene.Revision,
			X: left, Y: top, W: right - left, H: bottom - top, Text: v.selectedText(),
		})
	case "note":
		a.pdfTextDialog("note")
	case "replace":
		a.pdfTextDialog("replace")
	case "copy":
		text := v.selectedText()
		if text == "" && v.selected >= 0 && v.selected < len(v.scene.Text) {
			text = v.scene.Text[v.selected].Text
		}
		if text != "" {
			gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
		}
	case "rotate", "undo", "redo":
		a.startPDFCommand(document.PDFCommand{Kind: id, Page: v.page, Revision: v.info.Revision})
	case "save":
		if sessionPath(a.documentSessionState) == "" {
			a.startFileCommand(commandSaveAs, "pdf")
		} else {
			a.startSave()
		}
	case "save-as", "download":
		a.startFileCommand(commandSaveAs, "pdf")
	case "print":
		a.printCurrentPDF()
	}
}

type startPDFSearchPayload struct {
	Query      string `json:"query"`
	StartPage  int    `json:"start_page"`
	PageLimit  int    `json:"page_limit"`
	MaxResults int    `json:"max_results"`
}

func (a *App) startPDFSearch() {
	v := a.ensurePDFView()
	query := strings.TrimSpace(v.search.Text())
	state := a.documentSessionState
	if v.pending || query == "" || state == nil || state.pdfDoc == nil {
		return
	}
	v.pending = true
	d := a.pdfDoc
	window := a.window
	shutdown := a.shutdown
	documentID := state.documentID
	revision := state.wordRevisionAtomic.Load()
	startPage := v.page + 1
	a.launchManagedBackground(func() {
		r := pdfPageResult{info: d.Info()}
		ranges := [][2]int{{startPage, r.info.Pages}, {1, startPage - 1}}
		runtime := a.ensureDesktopRuntime()
		if runtime.applicationRuntimeErr != nil || runtime.applicationRuntime == nil {
			r.err = application.ErrRuntimeClosed
		}
		for _, pageRange := range ranges {
			for first := pageRange[0]; r.err == nil && first <= pageRange[1] && len(r.search) == 0; {
				pageLimit := min(100, pageRange[1]-first+1)
				arguments, err := json.Marshal(startPDFSearchPayload{Query: query, StartPage: first, PageLimit: pageLimit, MaxResults: 1})
				if err != nil {
					r.err = err
					break
				}
				invocation, err := runtime.applicationRuntime.InvokeCapability(context.Background(), application.CapabilityRequest{
					RequestID:  fmt.Sprintf("pdf-search-%s-%d-%d", documentID, revision, first),
					DocumentID: documentID, CapabilityID: "pdf.document.search", ExpectedRevision: revision,
					Operator:  application.Operator{ID: fmt.Sprintf("window-%d", a.windowID), Kind: application.OperatorHuman},
					Arguments: arguments,
				})
				if err != nil {
					r.err = err
					break
				}
				var result struct {
					TotalPages int `json:"total_pages"`
					Matches    []struct {
						PageNumber int `json:"page_number"`
					} `json:"matches"`
					PageContentTruncated bool `json:"page_content_truncated"`
				}
				if err := json.Unmarshal(invocation.Data, &result); err != nil {
					r.err = application.ErrInvalidCommandResult
					break
				}
				if len(result.Matches) > 0 {
					r.search = []int{result.Matches[0].PageNumber - 1}
					break
				}
				if result.PageContentTruncated {
					r.notice = i18n.Message{ID: "pdf.status.search_limit_reached"}
					break
				}
				if result.TotalPages < 1 {
					r.err = application.ErrInvalidCommandResult
					break
				}
				first += pageLimit
			}
			if r.err != nil || len(r.search) > 0 || r.notice.ID != "" {
				break
			}
		}
		if len(r.search) == 0 && r.err == nil {
			if r.notice.ID == "" {
				r.err = fmt.Errorf("未找到 %q", query)
				r.notice = i18n.Message{ID: "pdf.status.search_complete"}
			}
		}
		r.info = d.Info()
		select {
		case v.results <- r:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
}

type printCurrentPDFPayload struct {
	Copies int `json:"copies"`
}

func (a *App) printCurrentPDF() {
	v := a.ensurePDFView()
	if v.pending {
		return
	}
	state := a.documentSessionState
	if state == nil || state.pdfDoc == nil || state.closed.Load() {
		v.messageRef = i18n.Message{ID: "pdf.error.session_unavailable"}
		v.message = ""
		return
	}
	if a.nativePrinter == nil || !a.nativePrinter.Available() {
		v.messageRef = i18n.Message{ID: "pdf.error.native_print_unsupported"}
		v.message = ""
		return
	}
	runtime := a.ensureDesktopRuntime()
	if runtime.applicationRuntimeErr != nil || runtime.applicationRuntime == nil || !a.applicationSessionRegistered(state.documentID) {
		v.messageRef = i18n.Message{ID: "pdf.error.runtime_unavailable"}
		v.message = ""
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		v.messageRef = i18n.Message{ID: "pdf.error.operation_in_progress"}
		v.message = ""
		return
	}
	v.pending = true
	done := v.workerSignal()
	window := a.window
	shutdown := a.shutdown
	ctx, cancel := context.WithCancel(context.Background())
	a.launchManagedBackground(func() {
		defer cancel()
		a.launchManagedBackground(func() {
			select {
			case <-shutdown:
				cancel()
			case <-ctx.Done():
			}
		})
		arguments, _ := json.Marshal(printCurrentPDFPayload{Copies: 1})
		invocation, err := runtime.applicationRuntime.InvokeCapability(ctx, application.CapabilityRequest{
			RequestID:  fmt.Sprintf("pdf-print-%s-%d", token.documentID, token.generation),
			DocumentID: token.documentID, CapabilityID: "pdf.document.print", ExpectedRevision: token.revision,
			Operator:  application.Operator{ID: fmt.Sprintf("window-%d", a.windowID), Kind: application.OperatorHuman},
			View:      application.ViewContext{WindowID: a.windowID, DocumentID: token.documentID, Revision: token.revision},
			Arguments: arguments,
		})
		result := pdfPageResult{info: state.pdfDoc.Info(), err: err}
		if err == nil {
			var printed struct {
				Status          string `json:"status"`
				PrintedRevision uint64 `json:"printed_revision"`
			}
			if decodeErr := json.Unmarshal(invocation.Data, &printed); decodeErr != nil || printed.Status != "sent_to_system_print_queue" || printed.PrintedRevision != token.revision {
				result.err = application.ErrInvalidCommandResult
			} else {
				result.notice = i18n.Message{ID: "pdf.status.print_sent", Args: i18n.Args{"Revision": printed.PrintedRevision}}
			}
		}
		state.finishOperation(token)
		select {
		case v.results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-done:
		case <-shutdown:
		}
	})
}
