package desktopui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"glance/internal/document"
	"glance/internal/fontcatalog"
	"glance/internal/format/docx"
	"glance/internal/format/wordpdf"
	"glance/internal/wordlayout"
)

type wordPDFPickerRequest struct {
	state       *documentSessionState
	key         wordLayoutKey
	orientation string
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
		a.setStatus("正在提交正文编辑，然后准备 PDF…")
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
		a.setStatus("当前 DOCX 适配器未提供分页信息，无法导出 PDF")
		return
	}
	if state.wordLayoutAcceptedKey == nil || *state.wordLayoutAcceptedKey != key || state.wordLayoutResult == nil {
		if state.wordLayoutErrorKey != nil && *state.wordLayoutErrorKey == key {
			a.setStatus("PDF 导出前分页失败：" + state.wordLayoutError)
			return
		}
		state.wordPDFExportAfterLayout = true
		a.setStatus("正在为 PDF 导出计算页面…")
		a.startWordPagination(state)
		if state.wordPDFExportAfterLayout && state.wordLayoutPendingKey == nil && state.wordLayoutErrorKey != nil && *state.wordLayoutErrorKey == key {
			state.wordPDFExportAfterLayout = false
			a.setStatus("PDF 导出前分页失败：" + state.wordLayoutError)
		}
		return
	}
	if wordLayoutBlocksPageExport(*state.wordLayoutResult) {
		a.setStatus(blockingWordLayoutMessage(*state.wordLayoutResult))
		return
	}
	if a.documentSessionState != state {
		a.setStatus("Word 文档已切换，PDF 导出已取消")
		return
	}
	warnings := wordLayoutWarnings(*state.wordLayoutResult)
	if len(warnings) == 0 {
		a.openWordPDFExportPreview(state, key)
		return
	}
	message := fmt.Sprintf("当前分页有 %d 项提示：\n%s\n\n确认后将进入 PDF 导出预览。", len(warnings), strings.Join(warnings, "\n"))
	a.showConfirmDialog("导出 PDF 前确认版式提示", message, modalDialogWarningConfirmStyle, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			a.setStatus("已取消导出 PDF")
			return
		}
		if !wordPDFLayoutStillCurrent(state, key) || wordHasPendingTextDrafts(state) {
			a.setStatus("文档在确认期间已更改，请重新导出 PDF")
			return
		}
		a.openWordPDFExportPreview(state, key)
	}, modalDialogActionSpec{ID: "cancel", Label: "取消", Style: modalDialogActionSecondary}, modalDialogActionSpec{ID: "confirm", Label: "继续", Style: modalDialogActionPrimary})
	a.setStatus("等待确认分页提示")
}

func (a *App) layoutWordForExport(state *documentSessionState, key wordLayoutKey, blocks []document.WordBlock, layoutData document.WordLayoutData, orientation string) (wordlayout.Result, error) {
	if state.wordLayoutResult == nil {
		return wordlayout.Result{}, errors.New("当前分页结果不可用")
	}
	preview := &a.pdfExportPreview
	if preview.wordState == state && preview.wordKey == key && preview.Orientation == orientation && preview.wordResult != nil && !wordLayoutBlocksPageExport(*preview.wordResult) {
		return cloneWordLayoutResult(*preview.wordResult), nil
	}
	pageResult := cloneWordLayoutResult(*state.wordLayoutResult)
	if orientation == "" {
		return pageResult, nil
	}
	isLandscape := strings.ToLower(orientation) == "landscape"
	currLandscape := false
	if len(pageResult.Pages) > 0 && pageResult.Pages[0].Clip.WidthPT > pageResult.Pages[0].Clip.HeightPT {
		currLandscape = true
	}
	if isLandscape == currLandscape {
		return pageResult, nil
	}

	w, h := 595.28, 841.89
	orient := "portrait"
	if isLandscape {
		w, h = 841.89, 595.28
		orient = "landscape"
	}
	setup := document.WordPageSetup{
		WidthPT: w, HeightPT: h, Orientation: orient,
		MarginTopPT: 72, MarginRightPT: 72, MarginBottomPT: 72, MarginLeftPT: 72,
		Set: document.WordPageWidthSet | document.WordPageHeightSet | document.WordPageOrientationSet,
	}
	targetLayoutData := layoutData
	if len(targetLayoutData.Sections) > 0 {
		sections := make([]document.WordSection, len(targetLayoutData.Sections))
		copy(sections, targetLayoutData.Sections)
		sections[0].PageSetup.Orientation = orient
		sections[0].PageSetup.Set |= document.WordPageOrientationSet
		if isLandscape && sections[0].PageSetup.WidthPT < sections[0].PageSetup.HeightPT {
			sections[0].PageSetup.WidthPT, sections[0].PageSetup.HeightPT = sections[0].PageSetup.HeightPT, sections[0].PageSetup.WidthPT
		} else if !isLandscape && sections[0].PageSetup.WidthPT > sections[0].PageSetup.HeightPT {
			sections[0].PageSetup.WidthPT, sections[0].PageSetup.HeightPT = sections[0].PageSetup.HeightPT, sections[0].PageSetup.WidthPT
		}
		targetLayoutData.Sections = sections
	}

	engine := a.wordLayoutEngine
	if engine == nil {
		if ts, ok := a.wordPDFFontSource.(wordlayout.TextShaper); ok {
			engine = wordlayout.NewEngine(ts)
			a.wordLayoutEngine = engine
		}
	}
	if engine != nil {
		res, err := engine.Layout(context.Background(), wordlayout.Input{
			DocumentID:       state.documentID,
			Revision:         key.Revision,
			Blocks:           blocks,
			Layout:           targetLayoutData,
			DefaultPageSetup: setup,
		})
		if err != nil {
			return wordlayout.Result{}, err
		}
		if wordLayoutBlocksPageExport(res) {
			return wordlayout.Result{}, errors.New(blockingWordLayoutMessage(res))
		}
		return res, nil
	}
	return wordlayout.Result{}, errors.New("无法按所选方向重新排版")
}

func (a *App) startWordPDFFileExport(state *documentSessionState, key wordLayoutKey, path string, orientation string) {
	if state == nil || state.word == nil || state.closed.Load() {
		a.setStatus("PDF 导出状态已过期，请重新导出")
		return
	}
	if !wordPDFLayoutStillCurrent(state, key) || wordHasPendingTextDrafts(state) || state.wordFlushRequested.Load() || state.formatRunning.Load() {
		a.setStatus("文档在选择保存位置期间已更改，未导出 PDF；请重新导出")
		return
	}
	if pathsReferToSameFile(path, state.path) {
		a.setStatus("PDF 保存路径不能覆盖源 DOCX")
		return
	}
	if strings.ToLower(filepath.Ext(path)) != ".pdf" {
		path += ".pdf"
	}
	if a.documentSessionState != state {
		a.setStatus("Word 文档已切换，PDF 导出已取消")
		return
	}
	if a.wordPDFFontSource == nil {
		shaper, err := newWordTextShaper()
		if err != nil {
			a.setStatus("初始化 PDF 字体服务失败：" + err.Error())
			return
		}
		a.wordPDFFontSource = shaper
	}
	reader, ok := any(state.word).(document.WordLayoutReader)
	if !ok {
		a.setStatus("DOCX 适配器未提供分页信息，无法导出 PDF")
		return
	}
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatus("文档正在处理其他操作，请稍后重试")
		return
	}
	blocks := cloneWordBlockSnapshot(wordBlocksForState(state))
	unlock := lockSessionAdapterRead(state)
	layoutData := reader.WordLayoutData()
	unlock()
	revision := state.wordRevisionAtomic.Load()
	pageResult, layoutErr := a.layoutWordForExport(state, key, blocks, layoutData, orientation)
	if layoutErr != nil {
		state.finishOperation(token)
		a.setStatus("PDF 导出前重新排版失败：" + layoutErr.Error())
		return
	}
	doc := state.word
	fontSource := a.wordPDFFontSource
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
		a.setStatus("PDF 导出不可用")
		return
	}
	stateCopy := state
	a.launchManagedBackground(func() {
		result := func() wordPDFExportResult {
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
						_, result.err = wordpdf.ExportScene(ctx, path, pageResult, fontSource, wordPDFImageSource{document: snapshotDoc}, func() (func(), error) {
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
			return result
		}()
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatus("正在生成 PDF…")
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
			a.setStatus("文档已更改，旧分页结果未用于当前文档")
		}
		return
	}
	if result.err != nil {
		if state == a.documentSessionState {
			a.setStatus("导出 PDF 失败：" + result.err.Error())
		}
	} else if state == a.documentSessionState {
		a.setStatus("PDF 已导出：" + filepath.Base(result.path))
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
	return wordLayoutKey{
		DocumentID: state.documentID, Revision: state.wordRevisionAtomic.Load(),
		DraftEpoch: state.wordDraftEpoch.Load(), PageSetupKey: wordLayoutPageSetupKey(data, defaultWordPageSetup), FontVersion: fontcatalog.Default().Version,
	}, data, true
}

func wordPDFLayoutStillCurrent(state *documentSessionState, key wordLayoutKey) bool {
	current, _, ok := currentWordLayoutKey(state)
	return ok && current == key && state.wordLayoutAcceptedKey != nil && *state.wordLayoutAcceptedKey == key && state.wordLayoutResult != nil
}

func wordLayoutWarnings(result wordlayout.Result) []string {
	warnings := make([]string, 0)
	if len(result.SupportReport.Findings) > 0 {
		for _, finding := range result.SupportReport.Findings {
			if finding.Impact != wordlayout.SupportImpactApproximation {
				continue
			}
			detail := wordSupportFindingDetail(finding)
			if detail == "" {
				detail = finding.Code
			}
			fallback := strings.TrimSpace(finding.Fallback)
			if finding.Code == "unsupported-list" {
				fallback = "正文文本仍会显示，编号标记会省略。"
			} else if finding.Code == "invalid-image-size" || finding.Code == "unresolved-image" || finding.Code == "invalid-image-inline" {
				fallback = "此处显示“图片无法显示”占位文本，文档其他内容保留。"
			}
			if fallback != "" {
				detail += "\n 影响：" + fallback
			}
			location := strings.TrimSpace(finding.Location)
			if location == "" {
				location = finding.SourceID
			}
			if location != "" {
				detail += "\n 位置：" + location
			}
			warnings = append(warnings, "• "+detail)
		}
		return warnings
	}
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

func wordSupportFindingDetail(finding wordlayout.SupportFinding) string {
	switch finding.Code {
	case "unsupported-list":
		return "此段落使用了当前未支持的列表编号定义。"
	case "invalid-image-size":
		return "图片尺寸缺失或无效，无法按原尺寸显示。"
	case "unresolved-image":
		return "无法读取这张图片的数据。"
	case "invalid-image-inline":
		return "图片内容结构不完整。"
	default:
		return strings.TrimSpace(finding.Detail)
	}
}

func wordLayoutBlocksPageExport(result wordlayout.Result) bool {
	if !result.CanPreview || result.SupportReport.BlocksExport() {
		return true
	}
	if len(result.SupportReport.Findings) == 0 {
		// Keep synthetic and older cached results safe until they are rebuilt
		// with the structured support report.
		for _, diagnostic := range result.Diagnostics {
			if diagnostic.Severity == wordlayout.DiagnosticError {
				return true
			}
		}
	}
	return false
}

func blockingWordLayoutMessage(result wordlayout.Result) string {
	for _, finding := range result.SupportReport.Findings {
		if finding.Impact == wordlayout.SupportImpactBlocksPreview || finding.Impact == wordlayout.SupportImpactBlocksExport {
			message := "当前页面布局无法安全导出或打印：" + finding.Detail
			location := strings.TrimSpace(finding.Location)
			if location == "" {
				location = strings.TrimSpace(finding.SourceID)
			}
			if location != "" {
				message += "（位置：" + location + "）"
			}
			if fallback := strings.TrimSpace(finding.Fallback); fallback != "" {
				message += "；" + fallback
			}
			return message
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == wordlayout.DiagnosticError {
			return "当前页面布局无法安全导出或打印：" + diagnostic.Detail
		}
	}
	return "当前分页结果不能安全导出或打印"
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
	clone.SupportReport.Findings = append([]wordlayout.SupportFinding(nil), result.SupportReport.Findings...)
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
