package desktopui

import (
	"context"
	"fmt"
	"glance/internal/i18n"
	"image"
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/presentationpdf"
)

type presentationPDFPickerRequest struct {
	state       *documentSessionState
	snapshot    document.PresentationSnapshot
	orientation string
}

type presentationPDFPreviewResult struct {
	state       *documentSessionState
	snapshot    document.PresentationSnapshot
	image       image.Image
	index       int
	orientation string
	generation  uint64
	openPreview bool
	printMode   bool
	err         error
}

type presentationPDFExportResult struct {
	state    *documentSessionState
	path     string
	printJob bool
	printed  bool
	err      error
}

func (a *App) requestPresentationPDFExport() {
	a.requestPresentationPDFPreview(false)
}

func (a *App) requestPresentationPDFPrint() {
	if a.nativePrinter == nil || !a.nativePrinter.Available() {
		a.setStatusMessage(i18n.Message{ID: "presentation.print.error.unsupported"}, feedbackWarning, notificationToast)
		return
	}
	a.requestPresentationPDFPreview(true)
}

func (a *App) requestPresentationPDFPreview(printMode bool) {
	state := a.documentSessionState
	if state == nil || state.pptxDoc == nil || state.closed.Load() || state.presentationPDFExportRunning || a.fileCommandRunning {
		return
	}
	view := a.ensurePresentationView()
	if view != nil && view.pictureCrop.active {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.finish_or_cancel_picture_cropping_before_e_dd4cfa"}, feedbackWarning, notificationSuppressed)
		return
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
		return
	}
	if err := a.flushPresentationNotesDraft(view); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
		return
	}
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_is_busy_try_again_shortly_10cf71"}, feedbackWarning, notificationToast)
		return
	}
	a.presentationPDFPreviewGen++
	generation := a.presentationPDFPreviewGen
	results, shutdown, window := a.presentationPDFPreviewResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_pdf_preview_is_unavailabl_4fd241"}, feedbackWarning, notificationToast)
		return
	}
	stateCopy := state
	fontSnapshot := a.currentFontSnapshot()
	a.launchManagedBackground(func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		finished := make(chan struct{})
		defer close(finished)
		a.launchManagedBackground(func() {
			select {
			case <-shutdown:
				cancel()
			case <-finished:
			}
		})
		result := presentationPDFPreviewResult{state: stateCopy, generation: generation, index: 0, openPreview: true, printMode: printMode}
		result.snapshot = stateCopy.pptxDoc.Snapshot()
		if result.snapshot.Revision != token.revision {
			result.err = fmt.Errorf("演示文稿修订已变化，请重新打开导出预览")
		} else if result.snapshot.SlideCount == 0 || len(result.snapshot.Slides) != result.snapshot.SlideCount {
			result.err = fmt.Errorf("演示文稿快照不完整，无法导出")
		} else {
			result.orientation = presentationpdf.DefaultOrientation(result.snapshot)
			result.image, result.err = presentationpdf.RenderSlidePreviewWithFontSnapshot(ctx, result.snapshot, 0, result.orientation, fontSnapshot)
		}
		stateCopy.finishOperation(token)
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.preparing_the_presentation_pdf_preview_1c40d8"}, feedbackInfo, notificationSuppressed)
}

func (a *App) applyPresentationPDFPreviewResult(result presentationPDFPreviewResult) {
	if result.generation != a.presentationPDFPreviewGen {
		return
	}
	if result.state == nil || result.state.closed.Load() || result.state != a.documentSessionState || result.state.pptxDoc == nil {
		return
	}
	if result.err != nil {
		if !result.openPreview {
			prev := &a.pdfExportPreview
			if prev.Active && prev.Format == "presentation" && prev.presentationPreviewGen == result.generation {
				prev.presentationPreviewPending = false
				prev.presentationPreviewError = result.err.Error()
			}
		}
		a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.openPreview {
		a.pdfExportPreview = pdfExportPreviewState{
			Active: true, Format: "presentation", Orientation: result.orientation,
			CurrentPageIndex: result.index, TotalPages: len(result.snapshot.Slides),
			presentationState: result.state, presentationSnapshot: result.snapshot,
			presentationPreviewImage: result.image, presentationPreviewIndex: result.index,
			presentationPreviewGen: result.generation,
			presentationPrintMode:  result.printMode,
		}
		title := i18n.Message{ID: "presentation.export.preview.title"}
		body := i18n.Message{ID: "presentation.export.preview.body"}
		confirmLabel := i18n.Message{ID: "common.action.export_pdf"}
		if result.printMode {
			title = i18n.Message{ID: "presentation.export.print.title"}
			body = i18n.Message{ID: "presentation.export.print.body", Args: i18n.Args{"Count": len(result.snapshot.Slides)}}
			confirmLabel = i18n.Message{ID: "common.action.open_print_panel"}
		}
		a.modal.ShowCustomMessage("presentation.pdf.preview", title, body, 680, a.layoutPDFExportPreview, func(action string) {
			prev := &a.pdfExportPreview
			if action == "confirm" && prev.presentationPreviewPending {
				if prev.presentationPrintMode {
					a.setStatusMessage(i18n.Message{ID: "presentation.export.status.preview_pending_print"}, feedbackInfo, notificationSuppressed)
				} else {
					a.setStatusMessage(i18n.Message{ID: "presentation.export.status.preview_pending_export"}, feedbackInfo, notificationSuppressed)
				}
				return
			}
			a.modal.Hide()
			prev.Active = false
			if action != "confirm" {
				if prev.presentationPrintMode {
					a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.presentation_printing_canceled_29cda8"}, feedbackSuccess, notificationSuppressed)
				} else {
					a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.presentation_pdf_export_canceled_8ec621"}, feedbackSuccess, notificationSuppressed)
				}
				return
			}
			if prev.presentationPrintMode {
				a.startPresentationPDFPrint(prev.presentationState, prev.presentationSnapshot, prev.Orientation)
			} else {
				a.startPresentationPDFSavePicker(prev.presentationState, prev.presentationSnapshot, prev.Orientation)
			}
		}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}, Style: modalDialogActionSecondary},
			modalDialogActionSpec{ID: "confirm", LabelRef: confirmLabel, Style: modalDialogActionPrimary})
		a.modal.CustomActionsInLayout = true
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.review_the_selected_slides_in_the_pdf_prev_dad618"}, feedbackWarning, notificationToast)
		return
	}
	prev := &a.pdfExportPreview
	if !prev.Active || prev.Format != "presentation" || prev.presentationState != result.state || prev.presentationPreviewGen != result.generation || prev.CurrentPageIndex != result.index || prev.Orientation != result.orientation {
		return
	}
	prev.presentationPreviewPending = false
	prev.presentationPreviewError = ""
	prev.presentationPreviewImage = result.image
	prev.presentationPreviewIndex = result.index
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) requestPresentationPDFPreviewPage(index int, orientation string) {
	prev := &a.pdfExportPreview
	if !prev.Active || prev.Format != "presentation" || prev.presentationState == nil || prev.presentationState.closed.Load() {
		return
	}
	if index < 0 || index >= len(prev.presentationSnapshot.Slides) {
		return
	}
	prev.presentationPreviewPending = true
	prev.presentationPreviewError = ""
	a.presentationPDFPreviewGen++
	generation := a.presentationPDFPreviewGen
	prev.presentationPreviewGen = generation
	state, snapshot, printMode := prev.presentationState, prev.presentationSnapshot, prev.presentationPrintMode
	fontSnapshot := a.currentFontSnapshot()
	results, shutdown, window := a.presentationPDFPreviewResults, a.shutdown, a.window
	a.launchManagedBackground(func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		finished := make(chan struct{})
		defer close(finished)
		a.launchManagedBackground(func() {
			select {
			case <-shutdown:
				cancel()
			case <-finished:
			}
		})
		image, err := presentationpdf.RenderSlidePreviewWithFontSnapshot(ctx, snapshot, index, orientation, fontSnapshot)
		result := presentationPDFPreviewResult{state: state, image: image, index: index, orientation: orientation, generation: generation, printMode: printMode, err: err}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
}

func (a *App) startPresentationPDFSavePicker(state *documentSessionState, snapshot document.PresentationSnapshot, orientation string) {
	if state == nil || state.pptxDoc == nil || state.closed.Load() || a.fileCommandRunning {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_pdf_export_request_expire_60b651"}, feedbackWarning, notificationToast)
		return
	}
	a.presentationPDFPickerRequest = &presentationPDFPickerRequest{state: state, snapshot: snapshot, orientation: orientation}
	a.startFileCommand(commandExportPDF, "pdf")
}

type presentationDocumentExportPdfArguments struct {
	TargetPath  string `json:"target_path"`
	Orientation string `json:"orientation"`
}

func (a *App) startPresentationPDFFileExport(request *presentationPDFPickerRequest, path string) {
	if request == nil || request.state == nil || request.state.pptxDoc == nil || request.state.closed.Load() {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_pdf_export_request_expire_0e9637"}, feedbackWarning, notificationToast)
		return
	}
	state := request.state
	if state != a.documentSessionState || state.pptxDoc != a.pptxDoc {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_active_presentation_changed_pdf_export_932da1"}, feedbackInfo, notificationSuppressed)
		return
	}
	if !strings.EqualFold(filepath.Ext(path), ".pdf") {
		path += ".pdf"
	}
	if pathsReferToSameFile(path, state.path) {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_pdf_destination_cannot_overwrite_the_s_61cd27"}, feedbackInfo, notificationToast)
		return
	}
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_is_busy_try_again_shortly_10cf71"}, feedbackWarning, notificationToast)
		return
	}
	current := state.pptxDoc.Snapshot()
	if current.Revision != request.snapshot.Revision || token.revision != request.snapshot.Revision {
		state.finishOperation(token)
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_changed_while_choosing_a_de7186"}, feedbackInfo, notificationToast)
		return
	}
	if state.presentationPDFExportRunning {
		state.finishOperation(token)
		return
	}
	if !a.applicationSessionRegistered(state.documentID) {
		state.finishOperation(token)
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_shared_application_runtime_is_unavaila_b0d4dd"}, feedbackWarning, notificationToast)
		return
	}
	state.presentationPDFExportRunning = true
	results, shutdown, window := a.presentationPDFExportResults, a.shutdown, a.window
	ctx, cancel := context.WithCancel(context.Background())
	a.launchManagedBackground(func() {
		select {
		case <-shutdown:
			cancel()
		case <-ctx.Done():
		}
	})
	a.launchManagedBackground(func() {
		defer cancel()
		defer state.finishOperation(token)
		_, err := a.executeRibbonApplicationCommandWithContext(ctx, token,
			"presentation.document.export_pdf", "presentation_document", state.documentID, "", presentationDocumentExportPdfArguments{TargetPath: path, Orientation: request.orientation},
		)
		result := presentationPDFExportResult{state: state, path: path, err: err}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.generating_the_presentation_pdf_5054ca"}, feedbackInfo, notificationSuppressed)
}

type presentationDocumentPrintArguments struct {
	Orientation string `json:"orientation"`
	Copies      int    `json:"copies"`
}

func (a *App) startPresentationPDFPrint(state *documentSessionState, snapshot document.PresentationSnapshot, orientation string) {
	if a.nativePrinter == nil || !a.nativePrinter.Available() {
		a.setStatusMessage(i18n.Message{ID: "presentation.print.error.unsupported"}, feedbackWarning, notificationToast)
		return
	}
	if state == nil || state.pptxDoc == nil || state.closed.Load() || state != a.documentSessionState {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_print_request_expired_sta_f8e7b9"}, feedbackWarning, notificationToast)
		return
	}
	if state.presentationPDFExportRunning || a.fileCommandRunning || !a.applicationSessionRegistered(state.documentID) {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_shared_application_runtime_is_unavaila_52b405"}, feedbackWarning, notificationToast)
		return
	}
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_is_busy_try_again_shortly_10cf71"}, feedbackWarning, notificationToast)
		return
	}
	current := state.pptxDoc.Snapshot()
	if current.Revision != snapshot.Revision || token.revision != snapshot.Revision {
		state.finishOperation(token)
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_changed_during_confirmati_a91262"}, feedbackInfo, notificationToast)
		return
	}
	state.presentationPDFExportRunning = true
	results, shutdown, window := a.presentationPDFExportResults, a.shutdown, a.window
	ctx, cancel := context.WithCancel(context.Background())
	a.launchManagedBackground(func() {
		select {
		case <-shutdown:
			cancel()
		case <-ctx.Done():
		}
	})
	a.launchManagedBackground(func() {
		defer cancel()
		defer state.finishOperation(token)
		_, err := a.executeRibbonApplicationCommandWithContext(ctx, token,
			"presentation.document.print", "presentation_document", state.documentID, "", presentationDocumentPrintArguments{Orientation: orientation, Copies: 1},
		)
		result := presentationPDFExportResult{state: state, printJob: true, printed: err == nil, err: err}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.generating_and_sending_presentation_print_bef299"}, feedbackInfo, notificationSuppressed)
}

func (a *App) applyPresentationPDFExportResult(result presentationPDFExportResult) {
	state := result.state
	if state == nil {
		return
	}
	state.presentationPDFExportRunning = false
	if state.closed.Load() || state != a.documentSessionState {
		return
	}
	if result.err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
		return
	}
	if result.printed {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.sent_the_current_presentation_revision_to_a6cf76"}, feedbackSuccess, notificationToast)
	} else {
		a.setStatusMessage(i18n.Message{ID: "presentation.export.status.exported", Args: i18n.Args{"File": filepath.Base(result.path)}}, feedbackSuccess, notificationToast)
	}
}

func (a *App) layoutMiniaturePresentationPaper(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview
	if prev.presentationPreviewPending {
		label := material.Caption(a.theme, a.textID("presentation.preview.generating"))
		label.TextSize = unit.Sp(11)
		return label.Layout(gtx)
	}
	if prev.presentationPreviewError != "" || prev.presentationPreviewImage == nil {
		message := prev.presentationPreviewError
		if message == "" {
			message = "当前幻灯片没有可显示的预览"
		}
		label := material.Caption(a.theme, message)
		label.TextSize = unit.Sp(11)
		return label.Layout(gtx)
	}
	return widget.Image{Src: paint.NewImageOp(prev.presentationPreviewImage), Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
}
