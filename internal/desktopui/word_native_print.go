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
	"glance/internal/printservice"
	"glance/internal/wordlayout"
)

type wordPrintResult struct {
	state    *documentSessionState
	key      wordLayoutKey
	prepared bool
	data     []byte
	err      error
}

func (a *App) requestWordPrint() {
	state := a.documentSessionState
	if state == nil || state.word == nil || a.wordPrinter == nil || !a.wordPrinter.Available() {
		a.status = "当前平台不支持原生打印"
		return
	}
	if state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning || state.wordPrintPreparing || state.wordPrintRunning {
		return
	}
	if wordHasPendingTextDrafts(state) {
		state.wordPrintAfterDrafts = true
		state.wordForceDraftFlush = true
		state.wordFlushRequested = true
		state.wordFlushAllDrafts = true
		a.status = "正在提交正文编辑，然后准备打印…"
		a.startNextWordTextCommit()
		return
	}
	a.beginWordPrint(state)
}

func (a *App) beginWordPrint(state *documentSessionState) {
	if state == nil || state.word == nil || state.closed.Load() || a.documentSessionState != state || a.wordPrinter == nil || !a.wordPrinter.Available() {
		return
	}
	if state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning || state.wordPrintPreparing || state.wordPrintRunning {
		return
	}
	if wordHasPendingTextDrafts(state) || state.wordFlushRequested {
		state.wordPrintAfterDrafts = true
		state.wordForceDraftFlush = true
		state.wordFlushRequested = true
		state.wordFlushAllDrafts = true
		a.startNextWordTextCommit()
		return
	}
	state.wordViewMode = wordViewPrintLayout
	key, _, ok := currentWordLayoutKey(state)
	if !ok {
		a.status = "当前 DOCX 适配器未提供分页信息，无法打印"
		return
	}
	if state.wordLayoutAcceptedKey == nil || *state.wordLayoutAcceptedKey != key || state.wordLayoutResult == nil {
		if state.wordLayoutErrorKey != nil && *state.wordLayoutErrorKey == key {
			a.status = "打印前分页失败：" + state.wordLayoutError
			return
		}
		state.wordPrintAfterLayout = true
		a.status = "正在为打印准备页面…"
		a.startWordPagination(state)
		return
	}
	if !state.wordLayoutResult.CanPreview {
		a.status = blockingWordLayoutMessage(*state.wordLayoutResult)
		return
	}
	for _, diagnostic := range state.wordLayoutResult.Diagnostics {
		if diagnostic.Severity == wordlayout.DiagnosticError {
			a.status = "无法打印：" + diagnostic.Detail
			return
		}
	}
	if !wordPDFLayoutStillCurrent(state, key) {
		a.status = "文档分页已变化，请重新打印"
		return
	}
	warnings := wordLayoutWarnings(*state.wordLayoutResult)
	if len(warnings) == 0 {
		a.startWordPrintPreparation(state, key)
		return
	}
	message := fmt.Sprintf("当前分页有 %d 项提示：\n%s\n\n确认后将按照此分页结果准备打印。", len(warnings), strings.Join(warnings, "\n"))
	a.showModalDialog("打印前确认版式提示", message, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			a.status = "已取消打印"
			return
		}
		if !wordPDFLayoutStillCurrent(state, key) || wordHasPendingTextDrafts(state) || state.wordFlushRequested {
			a.status = "文档在确认期间已更改，请重新打印"
			return
		}
		a.startWordPrintPreparation(state, key)
	}, modalDialogActionSpec{ID: "cancel", Label: "取消"}, modalDialogActionSpec{ID: "confirm", Label: "继续"})
	a.status = "等待确认分页提示"
}

func (a *App) startWordPrintPreparation(state *documentSessionState, key wordLayoutKey) {
	if state == nil || state.word == nil || state.closed.Load() || a.documentSessionState != state {
		return
	}
	if !wordPDFLayoutStillCurrent(state, key) || wordHasPendingTextDrafts(state) || state.wordFlushRequested || state.formatRunning.Load() {
		a.status = "文档已更改，未准备打印；请重新打印"
		return
	}
	if a.wordPDFFontSource == nil {
		shaper, err := newWordTextShaper()
		if err != nil {
			a.status = "初始化打印字体服务失败：" + err.Error()
			return
		}
		a.wordPDFFontSource = shaper
	}
	readerWithData, ok := any(state.word).(document.WordLayoutReader)
	if !ok {
		a.status = "DOCX 适配器未提供分页信息，无法打印"
		return
	}
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.status = "文档正在处理其他操作，请稍后重试"
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
		a.status = "打印准备不可用"
		return
	}
	stateCopy := state
	go func() {
		defer stateCopy.finishOperation(token)
		result := wordPrintResult{state: stateCopy, key: key, prepared: true}
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
	}()
	a.status = "正在生成打印页面…"
}

func (a *App) beginWordNativePrint(state *documentSessionState, key wordLayoutKey, data []byte) {
	if state == nil || state.word == nil || state.closed.Load() || a.documentSessionState != state || a.wordPrinter == nil || !a.wordPrinter.Available() {
		return
	}
	if !wordPDFLayoutStillCurrent(state, key) || wordHasPendingTextDrafts(state) || state.wordFlushRequested {
		a.status = "文档已更改，打印已取消；请重新打印"
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
	printer := a.wordPrinter
	results, shutdown, window := a.wordPrintResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		cancel()
		state.wordPrintCancel = nil
		state.wordPrintRunning = false
		a.status = "打印服务不可用"
		return
	}
	stateCopy := state
	go func() {
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
	}()
	a.status = "正在打开系统打印面板…"
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
	if state.wordRevisionAtomic.Load() != result.key.Revision || !wordPDFLayoutStillCurrent(state, result.key) {
		if state == a.documentSessionState {
			a.status = "文档已更改，旧打印页面已丢弃"
		}
		return
	}
	if result.err != nil {
		if state == a.documentSessionState {
			if errors.Is(result.err, printservice.ErrCanceled) {
				a.status = "已取消打印"
			} else if errors.Is(result.err, context.Canceled) {
				a.status = "打印准备已取消"
			} else {
				a.status = "打印失败：" + result.err.Error()
			}
		}
		return
	}
	if state != a.documentSessionState {
		return
	}
	if result.prepared {
		if len(result.data) == 0 {
			a.status = "打印页面为空，无法打印"
			return
		}
		pageCount := len(state.wordLayoutResult.Pages)
		message := fmt.Sprintf("共 %d 页。将使用当前打印布局生成的同一份页面内容。继续后打开系统打印面板，可在面板中选择打印机和份数。", pageCount)
		printData := append([]byte(nil), result.data...)
		a.showModalDialog("打印预览已准备", message, func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				a.status = "已取消打印"
				return
			}
			if !wordPDFLayoutStillCurrent(state, result.key) || wordHasPendingTextDrafts(state) || state.wordFlushRequested {
				a.status = "文档在确认期间已更改，请重新打印"
				return
			}
			a.beginWordNativePrint(state, result.key, printData)
		}, modalDialogActionSpec{ID: "cancel", Label: "取消"}, modalDialogActionSpec{ID: "confirm", Label: "打开打印面板"})
		a.status = "等待确认打印页面"
		return
	}
	if result.err == nil {
		a.status = "已发送到系统打印队列"
	}
}
