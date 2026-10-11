package desktopui

import (
	"context"
	"fmt"
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
		a.setStatus("当前平台不支持原生打印")
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
		a.setStatus("请先完成或取消图片裁剪后再导出 PDF")
		return
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		a.setStatus("提交幻灯片文字失败，未打开 PDF 预览：" + err.Error())
		return
	}
	if err := a.flushPresentationNotesDraft(view); err != nil {
		a.setStatus("提交演讲者备注失败，未打开 PDF 预览：" + err.Error())
		return
	}
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatus("演示文稿正在处理其他操作，请稍后重试")
		return
	}
	a.presentationPDFPreviewGen++
	generation := a.presentationPDFPreviewGen
	results, shutdown, window := a.presentationPDFPreviewResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		a.setStatus("演示文稿 PDF 预览不可用")
		return
	}
	stateCopy := state
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
			result.image, result.err = presentationpdf.RenderSlidePreview(ctx, result.snapshot, 0, result.orientation)
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
	a.setStatus("正在准备演示文稿 PDF 预览…")
}

func (a *App) applyPresentationPDFPreviewResult(result presentationPDFPreviewResult) {
	if result.generation != a.presentationPDFPreviewGen {
		return
	}
	if result.state == nil || result.state.closed.Load() || result.state != a.documentSessionState || result.state.pptxDoc == nil {
		return
	}
	if result.err != nil {
		verb := "准备演示文稿 PDF 预览失败："
		if result.printMode {
			verb = "准备演示文稿打印预览失败："
		}
		if !result.openPreview {
			prev := &a.pdfExportPreview
			if prev.Active && prev.Format == "presentation" && prev.presentationPreviewGen == result.generation {
				prev.presentationPreviewPending = false
				prev.presentationPreviewError = result.err.Error()
			}
		}
		a.setStatus(verb + result.err.Error())
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
		title, body, confirmLabel := "导出 PDF 预览", "每张幻灯片适配到一张 A4 页面。预览按 Glance 当前支持的 PPTX 内容子集生成；动画、切换和媒体播放不会导出，未投影的 Office 内容无法在此列出。", "导出 PDF"
		if result.printMode {
			title, body, confirmLabel = "打印预览", fmt.Sprintf("将把全部 %d 张幻灯片发送到系统打印面板。每张幻灯片适配到一张 A4 页面；动画、切换和媒体播放不会打印，音视频仅显示占位符，未投影的 Office 内容无法在此列出。", len(result.snapshot.Slides)), "打开打印面板"
		}
		a.modal.ShowCustom(title, body, 680, a.layoutPDFExportPreview, func(action string) {
			prev := &a.pdfExportPreview
			if action == "confirm" && prev.presentationPreviewPending {
				verb := "导出"
				if prev.presentationPrintMode {
					verb = "打印"
				}
				a.setStatus("正在生成所选幻灯片预览，请稍后" + verb)
				return
			}
			a.modal.Hide()
			prev.Active = false
			if action != "confirm" {
				if prev.presentationPrintMode {
					a.setStatus("已取消打印演示文稿")
				} else {
					a.setStatus("已取消导出演示文稿 PDF")
				}
				return
			}
			if prev.presentationPrintMode {
				a.startPresentationPDFPrint(prev.presentationState, prev.presentationSnapshot, prev.Orientation)
			} else {
				a.startPresentationPDFSavePicker(prev.presentationState, prev.presentationSnapshot, prev.Orientation)
			}
		}, modalDialogActionSpec{ID: "cancel", Label: "取消", Style: modalDialogActionSecondary},
			modalDialogActionSpec{ID: "confirm", Label: confirmLabel, Style: modalDialogActionPrimary})
		a.modal.CustomActionsInLayout = true
		a.setStatus("请检查所选幻灯片的 PDF 预览")
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
		image, err := presentationpdf.RenderSlidePreview(ctx, snapshot, index, orientation)
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
		a.setStatus("演示文稿 PDF 导出状态已过期")
		return
	}
	a.presentationPDFPickerRequest = &presentationPDFPickerRequest{state: state, snapshot: snapshot, orientation: orientation}
	a.startFileCommand(commandExportPDF, "pdf")
}

func (a *App) startPresentationPDFFileExport(request *presentationPDFPickerRequest, path string) {
	if request == nil || request.state == nil || request.state.pptxDoc == nil || request.state.closed.Load() {
		a.setStatus("演示文稿 PDF 导出状态已过期，请重新导出")
		return
	}
	state := request.state
	if state != a.documentSessionState || state.pptxDoc != a.pptxDoc {
		a.setStatus("演示文稿已切换，PDF 导出已取消")
		return
	}
	if !strings.EqualFold(filepath.Ext(path), ".pdf") {
		path += ".pdf"
	}
	if pathsReferToSameFile(path, state.path) {
		a.setStatus("PDF 保存路径不能覆盖源演示文稿")
		return
	}
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatus("演示文稿正在处理其他操作，请稍后重试")
		return
	}
	current := state.pptxDoc.Snapshot()
	if current.Revision != request.snapshot.Revision || token.revision != request.snapshot.Revision {
		state.finishOperation(token)
		a.setStatus("演示文稿在选择保存位置期间已更改，请重新打开 PDF 预览")
		return
	}
	if state.presentationPDFExportRunning {
		state.finishOperation(token)
		return
	}
	if !a.applicationSessionRegistered(state.documentID) {
		state.finishOperation(token)
		a.setStatus("共享应用运行时不可用，无法导出演示文稿 PDF")
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
			"presentation.document.export_pdf", "presentation_document", state.documentID, "",
			map[string]any{"target_path": path, "orientation": request.orientation},
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
	a.setStatus("正在生成演示文稿 PDF…")
}

func (a *App) startPresentationPDFPrint(state *documentSessionState, snapshot document.PresentationSnapshot, orientation string) {
	if a.nativePrinter == nil || !a.nativePrinter.Available() {
		a.setStatus("当前平台不支持原生打印")
		return
	}
	if state == nil || state.pptxDoc == nil || state.closed.Load() || state != a.documentSessionState {
		a.setStatus("演示文稿打印状态已过期，请重新打印")
		return
	}
	if state.presentationPDFExportRunning || a.fileCommandRunning || !a.applicationSessionRegistered(state.documentID) {
		a.setStatus("共享应用运行时不可用，或演示文稿正在处理其他操作")
		return
	}
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatus("演示文稿正在处理其他操作，请稍后重试")
		return
	}
	current := state.pptxDoc.Snapshot()
	if current.Revision != snapshot.Revision || token.revision != snapshot.Revision {
		state.finishOperation(token)
		a.setStatus("演示文稿在确认期间已更改，请重新打开打印预览")
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
			"presentation.document.print", "presentation_document", state.documentID, "",
			map[string]any{"orientation": orientation, "copies": 1},
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
	a.setStatus("正在生成并发送演示文稿打印页面…")
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
		verb := "导出演示文稿 PDF 失败："
		if result.printJob {
			verb = "打印演示文稿失败："
		}
		a.setStatus(verb + result.err.Error())
		return
	}
	if result.printed {
		a.setStatus("已将当前演示文稿修订版本发送到系统打印服务")
	} else {
		a.setStatus("PDF 已导出：" + filepath.Base(result.path))
	}
}

func (a *App) layoutMiniaturePresentationPaper(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview
	if prev.presentationPreviewPending {
		label := material.Caption(a.theme, "正在生成预览…")
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
