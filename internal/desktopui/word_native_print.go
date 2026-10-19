package desktopui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"glance/internal/document"
	"glance/internal/format/docx"
	wordpdf "glance/internal/format/pdf"
	"glance/internal/i18n"
	"glance/internal/printservice"
)

type wordPrintResult struct {
	state     *documentSessionState
	key       wordLayoutKey
	prepared  bool
	pageCount int
	data      []byte
	err       error
}

func (a *App) requestWordPrint() {
	state := a.documentSessionState
	if state == nil || state.word == nil || a.nativePrinter == nil || !a.nativePrinter.Available() {
		a.setStatusMessage(i18n.Message{ID: "word.print.error.unsupported"}, feedbackWarning, notificationToast)
		return
	}
	if state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning || state.wordPrintPreparing || state.wordPrintRunning {
		return
	}
	if wordHasPendingTextDrafts(state) {
		state.wordPrintAfterDrafts = true
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		a.setStatusMessage(i18n.Message{ID: "word.print.status.commit_before_print"}, feedbackInfo, notificationSuppressed)
		a.startNextWordTextCommit()
		return
	}
	a.beginWordPrint(state)
}

func (a *App) beginWordPrint(state *documentSessionState) {
	if state == nil || state.word == nil || state.closed.Load() || a.documentSessionState != state || a.nativePrinter == nil || !a.nativePrinter.Available() {
		return
	}
	if state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning || state.wordPrintPreparing || state.wordPrintRunning {
		return
	}
	if wordHasPendingTextDrafts(state) || state.wordFlushRequested.Load() {
		state.wordPrintAfterDrafts = true
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		a.startNextWordTextCommit()
		return
	}
	state.wordViewMode = wordViewPrintLayout
	key, _, ok := currentWordLayoutKey(state)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.print.error.pagination_unavailable"}, feedbackWarning, notificationToast)
		return
	}
	if state.wordLayoutAcceptedKey == nil || *state.wordLayoutAcceptedKey != key || state.wordLayoutResult == nil {
		if state.wordLayoutErrorKey != nil && *state.wordLayoutErrorKey == key {
			a.setStatusDetailMessage(i18n.Message{ID: "word.print.error.pagination_failed"}, feedbackError, notificationToast, state.wordLayoutError)
			return
		}
		state.wordPrintAfterLayout = true
		a.setStatusMessage(i18n.Message{ID: "word.print.status.prepare_pages"}, feedbackInfo, notificationSuppressed)
		a.startWordPagination(state)
		return
	}
	if wordLayoutBlocksPageExport(*state.wordLayoutResult) {
		a.setStatusMessage(a.wordBlockingLayoutMessage(*state.wordLayoutResult), feedbackError, notificationToast)
		return
	}
	if !wordPDFLayoutStillCurrent(state, key) {
		a.setStatusMessage(i18n.Message{ID: "word.print.error.layout_changed"}, feedbackWarning, notificationToast)
		return
	}
	warnings := localizedWordLayoutWarnings(a, *state.wordLayoutResult)
	if len(warnings) == 0 {
		a.startWordPrintPreparation(state, key)
		return
	}
	message := i18n.Message{ID: "word.print.confirm.layout_warnings", Args: i18n.Args{"Count": len(warnings), "Warnings": strings.Join(warnings, "\n")}}
	a.modal.ShowConfirmMessage(dialogIDWordPrintWarning, i18n.Message{ID: "word.print.confirm.layout_title"}, message, modalDialogWarningConfirmStyle, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			a.setStatusMessage(i18n.Message{ID: "word.print.status.cancelled"}, feedbackInfo, notificationSuppressed)
			return
		}
		if !wordPDFLayoutStillCurrent(state, key) || wordHasPendingTextDrafts(state) || state.wordFlushRequested.Load() {
			a.setStatusMessage(i18n.Message{ID: "word.print.error.changed_during_confirmation"}, feedbackWarning, notificationToast)
			return
		}
		a.startWordPrintPreparation(state, key)
	}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}, Style: modalDialogActionSecondary}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.continue"}, Style: modalDialogActionPrimary})
	a.setStatusMessage(i18n.Message{ID: "word.print.status.waiting_layout_confirmation"}, feedbackInfo, notificationSuppressed)
}

func (a *App) startWordPrintPreparation(state *documentSessionState, key wordLayoutKey) {
	if state == nil || state.word == nil || state.closed.Load() || a.documentSessionState != state {
		return
	}
	if !wordPDFLayoutStillCurrent(state, key) || wordHasPendingTextDrafts(state) || state.wordFlushRequested.Load() || state.formatRunning.Load() {
		a.setStatusMessage(i18n.Message{ID: "word.print.error.changed_before_prepare"}, feedbackWarning, notificationToast)
		return
	}
	if a.wordPDFFontSource == nil {
		shaper, err := newWordTextShaper()
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "word.print.error.font_service"}, err, notificationToast)
			return
		}
		a.wordPDFFontSource = shaper
	}
	readerWithData, ok := any(state.word).(document.WordLayoutReader)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.print.error.pagination_unavailable"}, feedbackWarning, notificationToast)
		return
	}
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.operation_busy"}, feedbackWarning, notificationToast)
		return
	}
	pageResult := cloneWordLayoutResult(*state.wordLayoutResult)
	blocks := cloneWordBlockSnapshot(wordBlocksForState(state))
	unlock := lockSessionAdapterRead(state)
	layoutData := readerWithData.WordLayoutData()
	unlock()
	doc := state.word
	fontSource := a.wordPDFFontSource
	revision := state.wordRevisionAtomic.Load()
	draftEpoch := state.wordDraftEpoch.Load()
	ctx, cancel := context.WithCancel(context.Background())
	state.wordPrintCancel = cancel
	state.wordPrintPreparing = true
	results, shutdown, window := a.wordPrintResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		cancel()
		state.wordPrintCancel = nil
		state.wordPrintPreparing = false
		a.setStatusMessage(i18n.Message{ID: "word.print.error.preparation_unavailable"}, feedbackError, notificationToast)
		return
	}
	stateCopy := state
	a.launchManagedBackground(func() {
		defer stateCopy.finishOperation(token)
		result := wordPrintResult{state: stateCopy, key: key, prepared: true, pageCount: len(pageResult.Pages)}
		unlock := lockSessionAdapterRead(stateCopy)
		snapshot, err := doc.Snapshot()
		unlock()
		if err != nil {
			result.err = fmt.Errorf("读取 DOCX 打印快照：%w", err)
		} else {
			snapshotDoc, openErr := docx.OpenSnapshot(snapshot)
			if openErr != nil {
				result.err = fmt.Errorf("校验 DOCX 打印快照：%w", openErr)
			} else {
				if !reflect.DeepEqual(snapshotDoc.WordBlocks(), blocks) || !reflect.DeepEqual(snapshotDoc.WordLayoutData(), layoutData) {
					result.err = errors.New("DOCX 快照与当前分页场景不一致，请重新打印")
				} else {
					var output bytes.Buffer
					result.err = wordpdf.WriteWordPDF(ctx, &output, pageResult, fontSource, wordPDFImageSource{document: snapshotDoc})
					if result.err == nil && (ctx.Err() != nil || stateCopy.closed.Load() || stateCopy.wordRevisionAtomic.Load() != revision || stateCopy.wordDraftEpoch.Load() != draftEpoch) {
						result.err = errors.New("Word 文档在准备打印期间已更改，请重新打印")
					}
					if result.err == nil {
						result.data = append([]byte(nil), output.Bytes()...)
					}
				}
				_ = snapshotDoc.Close()
			}
		}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatusMessage(i18n.Message{ID: "word.print.status.generating_pages"}, feedbackInfo, notificationSuppressed)
}

func (a *App) beginWordNativePrint(state *documentSessionState, key wordLayoutKey, data []byte) {
	if state == nil || state.word == nil || state.closed.Load() || a.documentSessionState != state || a.nativePrinter == nil || !a.nativePrinter.Available() {
		return
	}
	if !wordPreparedPrintStillCurrent(state, key) || wordHasPendingTextDrafts(state) || state.wordFlushRequested.Load() {
		a.setStatusMessage(i18n.Message{ID: "word.print.error.changed_print_cancelled"}, feedbackWarning, notificationToast)
		return
	}
	if a.applicationSessionRegistered(state.documentID) {
		a.beginWordApplicationPrint(state, key)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	state.wordPrintCancel = cancel
	state.wordPrintRunning = true
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		cancel()
		state.wordPrintCancel = nil
		state.wordPrintRunning = false
		return
	}
	printer := a.nativePrinter
	results, shutdown, window := a.wordPrintResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		cancel()
		state.wordPrintCancel = nil
		state.wordPrintRunning = false
		a.setStatusMessage(i18n.Message{ID: "word.print.error.service_unavailable"}, feedbackError, notificationToast)
		return
	}
	stateCopy := state
	a.launchManagedBackground(func() {
		defer stateCopy.finishOperation(token)
		result := wordPrintResult{state: stateCopy, key: key}
		result.err = printer.Print(ctx, data, printservice.Options{Copies: 1})
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatusMessage(i18n.Message{ID: "word.print.status.opening_panel"}, feedbackInfo, notificationSuppressed)
}

type wordDocumentPrintArguments struct {
}

func (a *App) beginWordApplicationPrint(state *documentSessionState, key wordLayoutKey) {
	ctx, cancel := context.WithCancel(context.Background())
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		cancel()
		return
	}
	state.wordPrintCancel = cancel
	state.wordPrintRunning = true
	results, shutdown, window := a.wordPrintResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		cancel()
		state.wordPrintCancel = nil
		state.wordPrintRunning = false
		a.setStatusMessage(i18n.Message{ID: "word.print.error.service_unavailable"}, feedbackError, notificationToast)
		return
	}
	a.launchManagedBackground(func() {
		defer state.finishOperation(token)
		result := wordPrintResult{state: state, key: key}
		_, result.err = a.executeRibbonApplicationCommandWithContext(ctx, token, "word.document.print", "word_document", state.documentID, "", wordDocumentPrintArguments{})
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatusMessage(i18n.Message{ID: "word.print.status.sending_revision"}, feedbackInfo, notificationSuppressed)
}

func (a *App) applyWordPrintResult(result wordPrintResult) {
	state := result.state
	if state == nil {
		return
	}
	state.wordPrintPreparing = false
	state.wordPrintRunning = false
	if state.wordPrintCancel != nil {
		state.wordPrintCancel()
		state.wordPrintCancel = nil
	}
	if state.closed.Load() {
		return
	}
	if state.wordRevisionAtomic.Load() != result.key.Revision || !wordPreparedPrintStillCurrent(state, result.key) {
		if state == a.documentSessionState {
			a.setStatusMessage(i18n.Message{ID: "word.print.status.stale_page_discarded"}, feedbackInfo, notificationSuppressed)
		}
		return
	}
	if result.err != nil {
		if state == a.documentSessionState {
			if errors.Is(result.err, printservice.ErrCanceled) {
				a.setStatusMessage(i18n.Message{ID: "word.print.status.cancelled"}, feedbackInfo, notificationSuppressed)
			} else if errors.Is(result.err, context.Canceled) {
				a.setStatusMessage(i18n.Message{ID: "word.print.status.preparation_cancelled"}, feedbackInfo, notificationSuppressed)
			} else {
				a.setStatusErrorMessage(i18n.Message{ID: "word.print.error.failed"}, result.err, notificationToast)
			}
		}
		return
	}
	if state != a.documentSessionState {
		return
	}
	if result.prepared {
		if len(result.data) == 0 {
			a.setStatusMessage(i18n.Message{ID: "word.print.error.empty"}, feedbackError, notificationToast)
			return
		}
		pageCount := result.pageCount
		if pageCount == 0 && state.wordLayoutResult != nil {
			pageCount = len(state.wordLayoutResult.Pages)
		}
		message := i18n.Message{ID: "word.print.confirm.prepared", Args: i18n.Args{"PageCount": pageCount}}
		printData := append([]byte(nil), result.data...)
		a.modal.ShowConfirmMessage(dialogIDWordPrintPrepared, i18n.Message{ID: "word.print.confirm.prepared_title"}, message, modalDialogPrintConfirmStyle, func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				a.setStatusMessage(i18n.Message{ID: "word.print.status.cancelled"}, feedbackInfo, notificationSuppressed)
				return
			}
			if !wordPreparedPrintStillCurrent(state, result.key) || wordHasPendingTextDrafts(state) || state.wordFlushRequested.Load() {
				a.setStatusMessage(i18n.Message{ID: "word.print.error.changed_during_confirmation"}, feedbackWarning, notificationToast)
				return
			}
			a.beginWordNativePrint(state, result.key, printData)
		}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}, Style: modalDialogActionSecondary}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.open_print_panel"}, Style: modalDialogActionPrimary})
		a.setStatusMessage(i18n.Message{ID: "word.print.status.waiting_confirmation"}, feedbackInfo, notificationSuppressed)
		return
	}
	if result.err == nil {
		a.setStatusMessage(i18n.Message{ID: "word.print.status.queued"}, feedbackSuccess, notificationToast)
	}
}

// Prepared PDF bytes retain their font snapshot. Only document changes invalidate them.
func wordPreparedPrintStillCurrent(state *documentSessionState, key wordLayoutKey) bool {
	current, _, ok := currentWordLayoutKey(state)
	current.FontVersion = key.FontVersion
	return ok && current == key
}
