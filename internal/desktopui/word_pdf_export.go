package desktopui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"glance/internal/document"
	"glance/internal/format/docx"
	wordpdf "glance/internal/format/pdf"
	"glance/internal/wordlayout"
)

type wordPDFPickerRequest struct {
	state *documentSessionState
	key   wordLayoutKey
}

type wordPDFExportResult struct {
	state *documentSessionState
	key   wordLayoutKey
	path  string
	err   error
}

type wordPDFImageSource struct{ document *docx.Document }

func (source wordPDFImageSource) ImageBytes(imageID string) ([]byte, error) {
	return source.document.WordImageBytes(imageID)
}

func (a *App) requestWordPDFExport() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning || state.wordPDFExportRunning {
		return
	}
	if wordHasPendingTextDrafts(state) {
		state.wordPDFExportAfterDrafts = true
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		a.status = "正在提交正文编辑，然后准备 PDF…"
		a.startNextWordTextCommit()
		return
	}
	a.beginWordPDFExport(state)
}

func (a *App) beginWordPDFExport(state *documentSessionState) {
	if state == nil || state.word == nil || state.closed.Load() || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning || state.wordPDFExportRunning {
		return
	}
	if wordHasPendingTextDrafts(state) || state.wordFlushRequested.Load() {
		state.wordPDFExportAfterDrafts = true
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		a.startNextWordTextCommit()
		return
	}
	key, _, ok := currentWordLayoutKey(state)
	if !ok {
		a.status = "当前 DOCX 适配器未提供分页信息，无法导出 PDF"
		return
	}
	if state.wordLayoutAcceptedKey == nil || *state.wordLayoutAcceptedKey != key || state.wordLayoutResult == nil {
		if state.wordLayoutErrorKey != nil && *state.wordLayoutErrorKey == key {
			a.status = "PDF 导出前分页失败：" + state.wordLayoutError
			return
		}
		state.wordPDFExportAfterLayout = true
		a.status = "正在为 PDF 导出计算页面…"
		a.startWordPagination(state)
		if state.wordPDFExportAfterLayout && state.wordLayoutPendingKey == nil && state.wordLayoutErrorKey != nil && *state.wordLayoutErrorKey == key {
			state.wordPDFExportAfterLayout = false
			a.status = "PDF 导出前分页失败：" + state.wordLayoutError
		}
		return
	}
	if !state.wordLayoutResult.CanPreview {
		a.status = blockingWordLayoutMessage(*state.wordLayoutResult)
		return
	}
	for _, diagnostic := range state.wordLayoutResult.Diagnostics {
		if diagnostic.Severity == wordlayout.DiagnosticError {
			a.status = "无法导出 PDF：" + diagnostic.Detail
			return
		}
	}
	if a.documentSessionState != state {
		a.status = "Word 文档已切换，PDF 导出已取消"
		return
	}
	warnings := wordLayoutWarnings(*state.wordLayoutResult)
	if len(warnings) == 0 {
		a.startWordPDFSavePicker(state, key)
		return
	}
	message := fmt.Sprintf("当前分页有 %d 项提示：\n%s\n\n确认后将按照此分页结果导出 PDF。", len(warnings), strings.Join(warnings, "\n"))
	a.showConfirmDialog("导出 PDF 前确认版式提示", message, modalDialogWarningConfirmStyle, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			a.status = "已取消导出 PDF"
			return
		}
		if !wordPDFLayoutStillCurrent(state, key) || wordHasPendingTextDrafts(state) {
			a.status = "文档在确认期间已更改，请重新导出 PDF"
			return
		}
		a.startWordPDFSavePicker(state, key)
	}, modalDialogActionSpec{ID: "cancel", Label: "取消", Style: modalDialogActionSecondary}, modalDialogActionSpec{ID: "confirm", Label: "继续", Style: modalDialogActionPrimary})
	a.status = "等待确认分页提示"
}

func (a *App) startWordPDFFileExport(state *documentSessionState, key wordLayoutKey, path string) {
	if state == nil || state.word == nil || state.closed.Load() {
		a.status = "PDF 导出状态已过期，请重新导出"
		return
	}
	if !wordPDFLayoutStillCurrent(state, key) || wordHasPendingTextDrafts(state) || state.wordFlushRequested.Load() || state.formatRunning.Load() {
		a.status = "文档在选择保存位置期间已更改，未导出 PDF；请重新导出"
		return
	}
	if pathsReferToSameFile(path, state.path) {
		a.status = "PDF 保存路径不能覆盖源 DOCX"
		return
	}
	if strings.ToLower(filepath.Ext(path)) != ".pdf" {
		path += ".pdf"
	}
	if a.documentSessionState != state {
		a.status = "Word 文档已切换，PDF 导出已取消"
		return
	}
	if a.wordPDFFontSource == nil {
		shaper, err := newWordTextShaper()
		if err != nil {
			a.status = "初始化 PDF 字体服务失败：" + err.Error()
			return
		}
		a.wordPDFFontSource = shaper
	}
	reader, ok := any(state.word).(document.WordLayoutReader)
	if !ok {
		a.status = "DOCX 适配器未提供分页信息，无法导出 PDF"
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
	layoutData := reader.WordLayoutData()
	unlock()
	doc := state.word
	fontSource := a.wordPDFFontSource
	revision := state.wordRevisionAtomic.Load()
	draftEpoch := state.wordDraftEpoch.Load()
	ctx, cancel := context.WithCancel(context.Background())
	state.wordPDFExportCancel = cancel
	state.wordPDFExportRunning = true
	results, shutdown, window := a.wordPDFExportResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		cancel()
		state.wordPDFExportCancel = nil
		state.wordPDFExportRunning = false
		a.status = "PDF 导出不可用"
		return
	}
	stateCopy := state
	go func() {
		defer stateCopy.finishOperation(token)
		result := wordPDFExportResult{state: stateCopy, key: key, path: path}
		unlock := lockSessionAdapterRead(stateCopy)
		snapshot, err := doc.Snapshot()
		unlock()
		if err != nil {
			result.err = fmt.Errorf("读取 DOCX 导出快照：%w", err)
		} else {
			snapshotDoc, openErr := docx.OpenSnapshot(snapshot)
			if openErr != nil {
				result.err = fmt.Errorf("校验 DOCX 导出快照：%w", openErr)
			} else {
				if !reflect.DeepEqual(snapshotDoc.WordBlocks(), blocks) || !reflect.DeepEqual(snapshotDoc.WordLayoutData(), layoutData) {
					result.err = errors.New("DOCX 快照与当前分页场景不一致，请重新导出")
				} else {
					result.err = wordpdf.ExportWordPDFFileWithCommitGuard(ctx, path, pageResult, fontSource, wordPDFImageSource{document: snapshotDoc}, func() (func(), error) {
						stateCopy.wordExportCommitMu.Lock()
						release := stateCopy.wordExportCommitMu.Unlock
						if ctx.Err() != nil {
							release()
							return nil, ctx.Err()
						}
						if stateCopy.closed.Load() || stateCopy.wordRevisionAtomic.Load() != revision || stateCopy.wordDraftEpoch.Load() != draftEpoch {
							release()
							return nil, errors.New("Word 文档在 PDF 导出期间已更改，未覆盖目标文件")
						}
						return release, nil
					})
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
	a.status = "正在生成 PDF…"
}

func (a *App) applyWordPDFExportResult(result wordPDFExportResult) {
	state := result.state
	if state == nil {
		return
	}
	state.wordPDFExportRunning = false
	state.wordPDFExportCancel = nil
	if state.closed.Load() {
		return
	}
	if state.wordRevisionAtomic.Load() != result.key.Revision {
		if state == a.documentSessionState {
			a.status = "文档已更改，旧分页结果未用于当前文档"
		}
		return
	}
	if result.err != nil {
		if state == a.documentSessionState {
			a.status = "导出 PDF 失败：" + result.err.Error()
		}
	} else if state == a.documentSessionState {
		a.status = "PDF 已导出：" + filepath.Base(result.path)
	}
}

func currentWordLayoutKey(state *documentSessionState) (wordLayoutKey, document.WordLayoutData, bool) {
	if state == nil || state.word == nil {
		return wordLayoutKey{}, document.WordLayoutData{}, false
	}
	data, ok := wordLayoutDataForState(state)
	if !ok {
		return wordLayoutKey{}, document.WordLayoutData{}, false
	}
	return wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevisionAtomic.Load(), PageSetupKey: wordLayoutPageSetupKey(data, defaultWordPageSetup)}, data, true
}

func wordPDFLayoutStillCurrent(state *documentSessionState, key wordLayoutKey) bool {
	current, _, ok := currentWordLayoutKey(state)
	return ok && current == key && state.wordLayoutAcceptedKey != nil && *state.wordLayoutAcceptedKey == key && state.wordLayoutResult != nil
}

func wordLayoutWarnings(result wordlayout.Result) []string {
	warnings := make([]string, 0)
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == wordlayout.DiagnosticWarning {
			detail := strings.TrimSpace(diagnostic.Detail)
			if detail == "" {
				detail = diagnostic.Code
			}
			warnings = append(warnings, "• "+detail)
		}
	}
	return warnings
}

func blockingWordLayoutMessage(result wordlayout.Result) string {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == wordlayout.DiagnosticError {
			return "无法导出 PDF：" + diagnostic.Detail
		}
	}
	return "当前分页结果不能安全导出 PDF"
}

func pathsReferToSameFile(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	leftAbs, leftErr := filepath.Abs(filepath.Clean(left))
	rightAbs, rightErr := filepath.Abs(filepath.Clean(right))
	if leftErr != nil || rightErr != nil {
		return false
	}
	return strings.EqualFold(leftAbs, rightAbs)
}

func cloneWordLayoutResult(result wordlayout.Result) wordlayout.Result {
	clone := result
	clone.Diagnostics = append([]wordlayout.Diagnostic(nil), result.Diagnostics...)
	clone.Pages = make([]wordlayout.PageScene, len(result.Pages))
	for pageIndex, page := range result.Pages {
		pageClone := page
		pageClone.Text = make([]wordlayout.TextFragment, len(page.Text))
		for fragmentIndex, fragment := range page.Text {
			fragmentClone := fragment
			fragmentClone.Glyphs = make([]wordlayout.PositionedGlyph, len(fragment.Glyphs))
			for glyphIndex, glyph := range fragment.Glyphs {
				glyphClone := glyph
				glyphClone.Outline = append([]wordlayout.GlyphPathCommand(nil), glyph.Outline...)
				fragmentClone.Glyphs[glyphIndex] = glyphClone
			}
			pageClone.Text[fragmentIndex] = fragmentClone
		}
		pageClone.Images = append([]wordlayout.ImagePlacement(nil), page.Images...)
		pageClone.TableCells = append([]wordlayout.TableCellPlacement(nil), page.TableCells...)
		pageClone.ParagraphBorders = append([]wordlayout.ParagraphBorderPlacement(nil), page.ParagraphBorders...)
		clone.Pages[pageIndex] = pageClone
	}
	if result.Locations != nil {
		clone.Locations = make(map[wordlayout.SourcePosition]wordlayout.Position, len(result.Locations))
		for source, position := range result.Locations {
			clone.Locations[source] = position
		}
	}
	return clone
}
