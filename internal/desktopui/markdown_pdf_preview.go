package desktopui

import (
	"context"
	"fmt"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"image"
	"image/color"
	"io"

	wordpdf "glance/internal/format/pdf"
	"glance/internal/wordlayout"
)

func (a *App) openMarkdownPDFExportPreview(result markdownFileResult) {
	v := a.ensureMarkdownView()
	v.exportPending = true
	a.pdfExportPreview = pdfExportPreviewState{
		Active: true, Format: "markdown", Orientation: "portrait",
		TotalPages: len(result.prepared.scene.Pages), wordResult: &result.prepared.scene,
		markdownResult: result,
	}
	a.pdfExportPreview.resourceWarnings.Axis = layout.Vertical
	message := "设置页面方向并预览导出效果"
	label := "导出 PDF"
	if result.missingImages > 0 {
		message = fmt.Sprintf("%d 张图片无法访问。仍然导出时，PDF 将保留图片名称、地址和失败原因。", result.missingImages)
		label = "仍然导出 PDF"
	}
	a.modal.ShowCustom("导出 PDF 预览", message, 680, a.layoutMarkdownPDFExportPreview, func(action string) {
		prev := &a.pdfExportPreview
		if action == "confirm" && prev.wordPreviewPending {
			a.setMarkdownExportStatus("正在按所选方向重新排版，请稍后再导出")
			return
		}
		a.modal.Hide()
		prev.Active = false
		if action != "confirm" {
			v.exportPending = false
			a.setMarkdownExportStatus("已取消导出 PDF")
			return
		}
		a.startMarkdownPDFPicker(prev.markdownResult)
	}, modalDialogActionSpec{ID: "cancel", Label: "取消"}, modalDialogActionSpec{ID: "confirm", Label: label})
	a.modal.CustomActionsInLayout = true
	a.setMarkdownExportStatus("请确认 PDF 预览和页面方向")
}

func (a *App) layoutMarkdownPDFExportPreview(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if prev.markdownResult.missingImages == 0 {
				return layout.Dimensions{}
			}
			return layout.Inset{Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(72)))
				return prev.resourceWarnings.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
					label := material.Body2(a.theme, prev.markdownResult.warnings)
					label.TextSize = unit.Sp(12)
					label.Color = color.NRGBA{R: 128, G: 91, B: 36, A: 255}
					return label.Layout(gtx)
				})
			})
		}), layout.Rigid(a.layoutPDFExportPreview))
}

func (a *App) updateMarkdownPDFOrientation(orientation string) {
	prev := &a.pdfExportPreview
	if !prev.Active || prev.Format != "markdown" || prev.wordPreviewPending || prev.Orientation == orientation {
		return
	}
	prev.wordPreviewPending = true
	result := prev.markdownResult
	result.preflight = false
	result.orientation = orientation
	prepared := *result.prepared
	result.prepared = &prepared
	setup := defaultWordPageSetup
	setup.Orientation = orientation
	if orientation == "landscape" {
		setup.WidthPT, setup.HeightPT = setup.HeightPT, setup.WidthPT
	}
	view, shutdown, window := a.markdownView, a.shutdown, a.window
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-shutdown:
				cancel()
			case <-done:
			}
		}()
		prepared.scene, result.err = wordlayout.NewEngine(prepared.shaper).Layout(ctx, wordlayout.Input{DocumentID: "markdown-pdf", Blocks: prepared.blocks, DefaultPageSetup: setup})
		for _, d := range prepared.scene.Diagnostics {
			if d.Severity == wordlayout.DiagnosticError {
				result.err = fmt.Errorf("PDF 排版：%s", d.Detail)
				break
			}
		}
		if result.err == nil {
			result.err = wordpdf.WriteWordPDF(ctx, io.Discard, prepared.scene, prepared.shaper, prepared.images)
		}
		select {
		case view.exports <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func (a *App) applyMarkdownPDFOrientation(result markdownFileResult) {
	prev := &a.pdfExportPreview
	if !prev.Active || prev.Format != "markdown" || prev.markdownResult.state != result.state {
		return
	}
	a.markdownView.exportPending = true
	prev.wordPreviewPending = false
	if result.err != nil {
		a.setMarkdownExportStatus("切换 PDF 方向失败：" + result.err.Error())
		return
	}
	prev.Orientation = result.orientation
	result.orientation = ""
	prev.markdownResult = result
	prev.wordResult = &result.prepared.scene
	prev.TotalPages = len(result.prepared.scene.Pages)
	prev.CurrentPageIndex = 0
}
