package desktopui

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	sheetpdf "glance/internal/format/pdf"
	"glance/internal/i18n"
)

func (a *App) layoutPDFExportPreview(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview
	previousOrientation, previousPage := prev.Orientation, prev.CurrentPageIndex

	// Handle Orientation Toggle Click Events
	if prev.portraitBtn.Clicked(gtx) && prev.Orientation != "portrait" {
		if prev.Format == "spreadsheet" {
			pagination := sheetpdf.ComputeSpreadsheetPaginationForTrimmedWindow(prev.sheetWindow, sheetpdf.SpreadsheetPDFOptions{
				SheetName:      prev.sheetName,
				Orientation:    "portrait",
				FitToPageWidth: true,
			})
			prev.sheetPagination = pagination
			prev.TotalPages = pagination.TotalPages
			prev.Orientation = "portrait"
			prev.CurrentPageIndex = 0
		} else if prev.Format == "markdown" {
			a.updateMarkdownPDFOrientation("portrait")
		} else if prev.Format == "word" {
			if err := a.updateWordPreviewOrientation("portrait"); err != nil {
				a.setStatusErrorMessage(i18n.Message{ID: "pdf.preview.error.orientation"}, err, notificationToast)
				return layout.Dimensions{}
			}
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	if prev.landscapeBtn.Clicked(gtx) && prev.Orientation != "landscape" {
		if prev.Format == "spreadsheet" {
			pagination := sheetpdf.ComputeSpreadsheetPaginationForTrimmedWindow(prev.sheetWindow, sheetpdf.SpreadsheetPDFOptions{
				SheetName:      prev.sheetName,
				Orientation:    "landscape",
				FitToPageWidth: true,
			})
			prev.sheetPagination = pagination
			prev.TotalPages = pagination.TotalPages
			prev.Orientation = "landscape"
			prev.CurrentPageIndex = 0
		} else if prev.Format == "markdown" {
			a.updateMarkdownPDFOrientation("landscape")
		} else if prev.Format == "word" {
			if err := a.updateWordPreviewOrientation("landscape"); err != nil {
				a.setStatusErrorMessage(i18n.Message{ID: "pdf.preview.error.orientation"}, err, notificationToast)
				return layout.Dimensions{}
			}
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}

	// Handle Pagination Controls
	if prev.prevPageBtn.Clicked(gtx) && prev.CurrentPageIndex > 0 {
		prev.CurrentPageIndex--
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	if prev.nextPageBtn.Clicked(gtx) && prev.CurrentPageIndex < prev.TotalPages-1 {
		prev.CurrentPageIndex++
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	if prev.Format == "presentation" && (previousOrientation != prev.Orientation || previousPage != prev.CurrentPageIndex) {
		a.requestPresentationPDFPreviewPage(prev.CurrentPageIndex, prev.Orientation)
	}

	height := gtx.Dp(unit.Dp(320))
	if height > gtx.Constraints.Max.Y {
		height = gtx.Constraints.Max.Y
	}
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
		layout.Flexed(0.56, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(a.layoutPDFExportSummaryRow),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Flexed(1, a.layoutPDFExportPaperCanvas),
			)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(20)}.Layout),
		layout.Flexed(0.44, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(a.layoutPDFExportOrientationSelector),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
				layout.Rigid(a.layoutConfirmDialogActions),
			)
		}),
	)
}

func (a *App) layoutPDFExportOrientationSelector(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview
	portraitSubtitle := a.text(i18n.Message{ID: "pdf.preview.orientation.portrait.subtitle"})
	landscapeSubtitle := a.text(i18n.Message{ID: "pdf.preview.orientation.landscape.subtitle"})
	if prev.Format == "presentation" {
		portraitSubtitle = a.text(i18n.Message{ID: "pdf.preview.orientation.portrait.subtitle.presentation"})
		landscapeSubtitle = a.text(i18n.Message{ID: "pdf.preview.orientation.landscape.subtitle.presentation"})
	}
	card := func(gtx layout.Context, title, subtitle string, landscape, active bool, btn *widget.Clickable) layout.Dimensions {
		height := gtx.Dp(unit.Dp(64))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return a.layoutOrientationCard(gtx, title, subtitle, landscape, active, btn)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card(gtx, a.text(i18n.Message{ID: "pdf.preview.orientation.portrait"}), portraitSubtitle, false, prev.Orientation == "portrait", &prev.portraitBtn)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card(gtx, a.text(i18n.Message{ID: "pdf.preview.orientation.landscape"}), landscapeSubtitle, true, prev.Orientation == "landscape", &prev.landscapeBtn)
		}),
	)
}

func (a *App) layoutOrientationCard(gtx layout.Context, title, subtitle string, isLandscape, active bool, btn *widget.Clickable) layout.Dimensions {
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		borderColor := color.NRGBA{R: 226, G: 232, B: 240, A: 255}
		bgColor := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		titleColor := color.NRGBA{R: 51, G: 65, B: 85, A: 255}
		borderWidth := float32(gtx.Dp(unit.Dp(1)))

		if active {
			borderColor = color.NRGBA{R: 52, G: 120, B: 209, A: 255}
			bgColor = color.NRGBA{R: 240, G: 246, B: 255, A: 255}
			titleColor = color.NRGBA{R: 52, G: 120, B: 209, A: 255}
			borderWidth = float32(gtx.Dp(unit.Dp(2)))
		}

		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				bounds := image.Rectangle{Max: gtx.Constraints.Max}
				radius := gtx.Dp(unit.Dp(8))
				shape := clip.RRect{Rect: bounds, NW: radius, NE: radius, SW: radius, SE: radius}
				paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))
				paint.FillShape(gtx.Ops, borderColor, clip.Stroke{Path: shape.Path(gtx.Ops), Width: borderWidth}.Op())
				return layout.Dimensions{Size: bounds.Max}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						// Icon thumbnail
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							w, h := gtx.Dp(unit.Dp(14)), gtx.Dp(unit.Dp(20))
							if isLandscape {
								w, h = gtx.Dp(unit.Dp(20)), gtx.Dp(unit.Dp(14))
							}
							bounds := image.Rect(0, 0, w, h)
							rect := clip.RRect{Rect: bounds, NW: gtx.Dp(unit.Dp(2)), NE: gtx.Dp(unit.Dp(2)), SW: gtx.Dp(unit.Dp(2)), SE: gtx.Dp(unit.Dp(2))}
							paint.FillShape(gtx.Ops, borderColor, clip.Stroke{Path: rect.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1.5)))}.Op())
							return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(22)), gtx.Dp(unit.Dp(22)))}
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									lbl := material.Body2(a.theme, title)
									lbl.Font.Weight = font.SemiBold
									lbl.TextSize = unit.Sp(13)
									lbl.Color = titleColor
									return lbl.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									sub := material.Caption(a.theme, subtitle)
									sub.TextSize = unit.Sp(11)
									sub.Color = color.NRGBA{R: 100, G: 116, B: 139, A: 255}
									return sub.Layout(gtx)
								}),
							)
						}),
					)
				})
			}),
		)
	})
}

func (a *App) layoutPDFExportSummaryRow(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview

	paperLabel := a.text(i18n.Message{ID: "pdf.preview.paper.portrait"})
	if prev.Orientation == "landscape" {
		paperLabel = a.text(i18n.Message{ID: "pdf.preview.paper.landscape"})
	}

	hintText := a.text(i18n.Message{ID: "pdf.preview.hint.reflow"})
	hintColor := color.NRGBA{R: 71, G: 85, B: 105, A: 255}
	if (prev.Format == "word" || prev.Format == "markdown") && prev.wordPreviewPending {
		hintText = a.text(i18n.Message{ID: "pdf.preview.hint.reflowing"})
	}
	if prev.Format == "presentation" {
		hintText = a.text(i18n.Message{ID: "pdf.preview.hint.presentation"})
		if prev.presentationPreviewPending {
			hintText = a.text(i18n.Message{ID: "pdf.preview.hint.presentation_loading"})
		} else if prev.presentationPreviewError != "" {
			hintText = a.text(i18n.Message{ID: "pdf.preview.hint.presentation_error", Args: i18n.Args{"Details": prev.presentationPreviewError}})
		}
	}
	if prev.Format == "spreadsheet" {
		if prev.Orientation == "landscape" && prev.TotalPages == 1 {
			hintText = a.text(i18n.Message{ID: "pdf.preview.hint.spreadsheet.fit_all"})
			hintColor = color.NRGBA{R: 22, G: 101, B: 52, A: 255}
		} else if prev.Orientation == "portrait" && prev.TotalPages > 1 {
			hintText = a.text(i18n.Message{ID: "pdf.preview.hint.spreadsheet.split"})
			hintColor = color.NRGBA{R: 194, G: 65, B: 12, A: 255}
		} else {
			hintText = a.text(i18n.Message{ID: "pdf.preview.hint.spreadsheet.trimmed"})
		}
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return a.layoutBadge(gtx, paperLabel, color.NRGBA{R: 241, G: 245, B: 249, A: 255}, color.NRGBA{R: 51, G: 65, B: 85, A: 255})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							pageBadgeColor := color.NRGBA{R: 234, G: 242, B: 252, A: 255}
							pageTextColor := color.NRGBA{R: 52, G: 120, B: 209, A: 255}
							return a.layoutBadge(gtx, a.text(i18n.Message{ID: "pdf.preview.page_count", Args: i18n.Args{"Count": prev.TotalPages}}), pageBadgeColor, pageTextColor)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(a.theme, hintText)
					lbl.TextSize = unit.Sp(11)
					lbl.Color = hintColor
					return lbl.Layout(gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if prev.TotalPages <= 1 {
				return layout.Dimensions{}
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(a.theme, &prev.prevPageBtn, "‹")
					btn.Background = color.NRGBA{R: 241, G: 245, B: 249, A: 255}
					btn.Color = color.NRGBA{R: 51, G: 65, B: 85, A: 255}
					btn.TextSize = unit.Sp(14)
					btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(8), Right: unit.Dp(8)}
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(a.theme, fmt.Sprintf("%d / %d", prev.CurrentPageIndex+1, prev.TotalPages))
					lbl.TextSize = unit.Sp(12)
					lbl.Color = color.NRGBA{R: 51, G: 65, B: 85, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(a.theme, &prev.nextPageBtn, "›")
					btn.Background = color.NRGBA{R: 241, G: 245, B: 249, A: 255}
					btn.Color = color.NRGBA{R: 51, G: 65, B: 85, A: 255}
					btn.TextSize = unit.Sp(14)
					btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(8), Right: unit.Dp(8)}
					return btn.Layout(gtx)
				}),
			)
		}),
	)
}

func (a *App) layoutBadge(gtx layout.Context, text string, bg, fg color.NRGBA) layout.Dimensions {
	height := gtx.Dp(unit.Dp(22))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			shape := clip.RRect{Rect: bounds, NW: gtx.Dp(unit.Dp(4)), NE: gtx.Dp(unit.Dp(4)), SW: gtx.Dp(unit.Dp(4)), SE: gtx.Dp(unit.Dp(4))}
			paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
			return layout.Dimensions{Size: bounds.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Caption(a.theme, text)
				lbl.TextSize = unit.Sp(11)
				lbl.Font.Weight = font.Medium
				lbl.Color = fg
				return lbl.Layout(gtx)
			})
		}),
	)
}
