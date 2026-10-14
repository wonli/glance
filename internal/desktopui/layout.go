package desktopui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

var (
	pageBackground           = color.NRGBA{R: 247, G: 248, B: 250, A: 255}
	chromeSurface            = color.NRGBA{R: 241, G: 243, B: 245, A: 255}
	workspaceTabStripSurface = color.NRGBA{R: 238, G: 241, B: 244, A: 255}
	tabSurface               = color.NRGBA{R: 238, G: 241, B: 244, A: 255}
	contentSurface           = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	gridHeaderFill           = color.NRGBA{R: 242, G: 244, B: 246, A: 255}
	gridLine                 = color.NRGBA{R: 216, G: 221, B: 227, A: 255}
	selectedFill             = color.NRGBA{R: 231, G: 240, B: 251, A: 255}
	mutedText                = color.NRGBA{R: 104, G: 113, B: 123, A: 255}
)

func (a *App) layout(gtx layout.Context) layout.Dimensions {
	a.ensureDesktopRuntime()
	if a.markdownDoc != nil && !a.modal.Visible {
		a.updateMarkdownInput(gtx)
	}
	a.processAsyncResults()
	a.reconcileWorkspaceTabs()
	syncWindowViewFromSharedSession(gtx, a.documentSessionState)
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return fill(gtx, pageBackground) },
		func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(a.layoutWorkspaceTabs),
						layout.Flexed(1, a.layoutWorkspaceBody),
					)
				}),
				layout.Stacked(a.layoutWordSymbolPicker),
				layout.Expanded(a.layoutFeedback),
				layout.Expanded(a.layoutModalDialog),
				layout.Expanded(a.layoutAgentConfirmationModal),
			)
		},
	)
}

// layoutWorkspaceBody keeps the document as the only content directly below
// the tabs. Transient actions float over it and do not change its geometry.
func (a *App) layoutWorkspaceBody(gtx layout.Context) layout.Dimensions {
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
				layout.Flexed(1, a.layoutDocument),
				layout.Rigid(a.layoutEditor),
			)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !a.showNewMenu {
				return layout.Dimensions{}
			}
			return a.layoutCreateMenuPopover(gtx)
		}),
		layout.Stacked(a.layoutWordVersionHistory),
	)
}

func (a *App) visibleStatus() string {
	if a.workspaceWriteError != "" {
		return a.workspaceWriteError
	}
	return a.status
}

func (a *App) layoutDocument(gtx layout.Context) layout.Dimensions {
	if a.activeTabID == "recent" || (a.book == nil && a.word == nil && a.pptxDoc == nil && a.jsonDoc == nil && a.markdownDoc == nil && a.pdfDoc == nil) {
		return a.layoutRecent(gtx)
	}
	switch {
	case a.book != nil:
		return a.layoutWorkbook(gtx)
	case a.word != nil:
		return a.layoutWord(gtx)
	case a.pptxDoc != nil:
		return a.layoutPresentationDocument(gtx)
	case a.pdfDoc != nil:
		return a.layoutPDFDocument(gtx)
	case a.markdownDoc != nil:
		return a.layoutMarkdownDocument(gtx)
	case a.jsonDoc != nil:
		return a.layoutJSON(gtx)
	default:
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return material.H6(a.theme, "Glance Office").Layout(gtx)
		})
	}
}

func (a *App) layoutWorkbook(gtx layout.Context) layout.Dimensions {
	if _, ok := a.book.(document.GridReader); ok {
		a.handleSpreadsheetToolbarEvents(gtx)
		return layout.Stack{Alignment: layout.NW}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(a.layoutSpreadsheetToolbar),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutAgentSidebarWorkspace(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(a.layoutGridControls),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, a.layoutVirtualGrid)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, a.layoutSheetTabs)
								}),
							)
						})
					}),
					layout.Rigid(a.layoutSpreadsheetStatusBar),
				)
			}),
			layout.Stacked(a.layoutSpreadsheetToolbarPickerOverlay),
		)
	}
	if len(a.grid) == 0 {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.layoutAgentFallbackToolbar),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return a.layoutAgentSidebarWorkspace(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(a.layoutSheetTabs),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.H6(a.theme, "当前工作表为空，没有读到单元格内容").Layout(gtx)
							})
						}),
					)
				})
			}),
		)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.layoutAgentFallbackToolbar),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutAgentSidebarWorkspace(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(a.layoutSheetTabs),
					layout.Rigid(a.layoutColumnHeader),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return material.List(a.theme, &a.gridList).Layout(gtx, len(a.grid), func(gtx layout.Context, row int) layout.Dimensions {
							return a.layoutGridRow(gtx, row)
						})
					}),
				)
			})
		}),
	)
}

func (a *App) layoutWord(gtx layout.Context) layout.Dimensions {
	a.handleWordToolbarEvents(gtx)
	a.handleWordClipboardEvents(gtx)
	dimensions := a.layoutWordFlow(gtx)
	a.processWordFormatPainterTargetIntent()
	return dimensions
}

func (a *App) layoutEditor(gtx layout.Context) layout.Dimensions {
	if a.jsonDoc != nil || a.pptxDoc != nil {
		return layout.Dimensions{}
	}
	if _, virtual := a.book.(document.GridReader); virtual {
		return layout.Dimensions{}
	}
	if a.book != nil && a.book.Editable() && a.selectedCell != "" {
		if a.applyButton.Clicked(gtx) {
			if err := a.applyCellEdit(); err != nil {
				a.setStatus("修改失败：" + err.Error())
			} else {
				a.setStatus(a.selectedCell + " 已修改；点击保存写入原文件")
			}
		}
		return editorPanel(gtx, a, a.selectedCell, &a.cellEditor, "应用单元格", &a.applyButton)
	}
	if a.word != nil {
		return layout.Dimensions{}
	}
	return layout.Dimensions{}
}

func editorPanel(gtx layout.Context, a *App, label string, editor *widget.Editor, action string, button *widget.Clickable) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Caption(a.theme, label).Layout(gtx)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.Editor(a.theme, editor, "文本").Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Button(a.theme, button, action)
					if a.word != nil && !a.selectedBlockEditable {
						style.Background = color.NRGBA{R: 190, G: 194, B: 201, A: 255}
					}
					return style.Layout(gtx)
				})
			}),
		)
	})
}
