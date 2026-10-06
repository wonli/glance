package desktopui

import (
	"image"
	"image/color"
	"runtime"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/markdowndoc"
	"glance/internal/format/xlsx"
)

const (
	createMenuWidth  unit.Dp = 480
	createMenuHeight unit.Dp = 390
)

func (a *App) layoutCreateMenuPopover(gtx layout.Context) layout.Dimensions {
	left := a.plusMenuOffset(gtx)
	shift := op.Offset(image.Pt(left, 0)).Push(gtx.Ops)
	dims := a.layoutCreateActions(gtx)
	shift.Pop()
	return dims
}

func (a *App) plusMenuOffset(gtx layout.Context) int {
	ids := a.workspaceTabIDs()
	plusIndex := len(ids) + 1
	x := -a.workspaceTabsList.Position.Offset
	for index := a.workspaceTabsList.Position.First; index < plusIndex; index++ {
		if index == 0 {
			x += gtx.Dp(unit.Dp(112))
		} else {
			x += gtx.Dp(workspaceDocumentTabWidth + workspaceDocumentTabGap)
		}
	}
	x += gtx.Dp(unit.Dp(16)) + gtx.Dp(workspaceDocumentTabGap)
	if a.clientTitlebar {
		if runtime.GOOS == "darwin" {
			x += gtx.Dp(unit.Dp(84))
		} else {
			x += gtx.Dp(unit.Dp(114))
		}
	}
	maxX := gtx.Constraints.Max.X - createMenuWidthPx(gtx) - gtx.Dp(unit.Dp(8))
	if maxX < gtx.Dp(unit.Dp(8)) {
		maxX = gtx.Dp(unit.Dp(8))
	}
	if x > maxX {
		x = maxX
	}
	if x < gtx.Dp(unit.Dp(8)) {
		x = gtx.Dp(unit.Dp(8))
	}
	return x
}

func (a *App) layoutCreateActions(gtx layout.Context) layout.Dimensions {
	width := createMenuWidthPx(gtx)
	height := gtx.Dp(createMenuHeight)
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			radius := gtx.Dp(unit.Dp(12))
			shadowBounds := bounds.Add(image.Pt(0, gtx.Dp(unit.Dp(5))))
			shadow := clip.RRect{Rect: shadowBounds, NE: radius, SE: radius, SW: radius}
			paint.FillShape(gtx.Ops, color.NRGBA{R: 31, G: 41, B: 55, A: 28}, shadow.Op(gtx.Ops))
			panel := clip.RRect{Rect: bounds, NE: radius, SE: radius, SW: radius}
			paint.FillShape(gtx.Ops, contentSurface, panel.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, homeBorder, clip.Stroke{Path: panel.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			seamCover := gtx.Dp(unit.Dp(1))
			connector := clip.Rect{
				Min: image.Pt(0, -seamCover),
				Max: image.Pt(gtx.Dp(workspacePlusTabWidth)+seamCover, seamCover),
			}
			paint.FillShape(gtx.Ops, contentSurface, connector.Op())
			return layout.Dimensions{Size: bounds.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(18), Bottom: unit.Dp(18), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutCreateMenuHeader(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, a.layoutCreateMenuGrid)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, a.layoutCreateMenuDivider)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, a.layoutCreateMenuOpenActions)
					}),
				)
			})
		}),
	)
}

func createMenuWidthPx(gtx layout.Context) int {
	width := gtx.Dp(createMenuWidth)
	available := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(16))
	if available > 0 && width > available {
		return available
	}
	return width
}

func (a *App) layoutCreateMenuHeader(gtx layout.Context) layout.Dimensions {
	if a.actionCloseCreateMenu.Clicked(gtx) {
		a.showNewMenu = false
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeLabelStyle(a.theme, "新建或打开", 16, color.NRGBA{R: 31, G: 37, B: 45, A: 255}, font.SemiBold).Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeLabelStyle(a.theme, "选择一种文件类型开始工作", 11, color.NRGBA{R: 122, G: 131, B: 142, A: 255}, font.Normal).Layout(gtx)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := gtx.Dp(unit.Dp(28))
			gtx.Constraints.Min = image.Pt(size, size)
			gtx.Constraints.Max = gtx.Constraints.Min
			return a.actionCloseCreateMenu.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{Alignment: layout.Center}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(6)))
						paint.FillShape(gtx.Ops, color.NRGBA{R: 242, G: 244, B: 246, A: 255}, shape.Op(gtx.Ops))
						return layout.Dimensions{Size: gtx.Constraints.Max}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							// The × glyph's visible ink sits above the text line box center.
							shift := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(4)))).Push(gtx.Ops)
							dims := homeTextLabel(gtx, a.theme, "×", 17, color.NRGBA{R: 104, G: 113, B: 124, A: 255}, false)
							shift.Pop()
							return dims
						})
					}),
				)
			})
		}),
	)
}

type createMenuFormat struct {
	click     *widget.Clickable
	id        string
	label     string
	extension string
	icon      string
	accent    color.NRGBA
	enabled   bool
}

// File-type glyphs are SVG exports of the matching icon nodes in Pen frame XGCJN.
func (a *App) createMenuFormats() []createMenuFormat {
	return []createMenuFormat{
		{click: &a.actionNewDOCX, id: "docx", label: "文档", extension: "DOCX", icon: "docx", accent: color.NRGBA{R: 52, G: 120, B: 209, A: 255}, enabled: true},
		{click: &a.actionNewXLSX, id: "xlsx", label: "表格", extension: "XLSX", icon: "xlsx", accent: color.NRGBA{R: 79, G: 166, B: 107, A: 255}, enabled: true},
		{id: "pptx", label: "演示文稿", extension: "PPTX", icon: "pptx", accent: color.NRGBA{R: 228, G: 130, B: 60, A: 255}},
		{click: &a.actionNewMarkdown, enabled: true, id: "markdown", label: "Markdown", extension: "MD", icon: "markdown", accent: color.NRGBA{R: 138, G: 90, B: 194, A: 255}},
		{click: &a.actionNewJSON, id: "json", label: "JSON 数据", extension: "JSON", icon: "json", accent: color.NRGBA{R: 43, G: 157, B: 152, A: 255}, enabled: true},
		{id: "pdf", label: "PDF 文档", extension: "PDF", icon: "pdf", accent: color.NRGBA{R: 240, G: 68, B: 69, A: 255}},
	}
}

func (a *App) layoutCreateMenuGrid(gtx layout.Context) layout.Dimensions {
	formats := a.createMenuFormats()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutCreateMenuFormatRow(gtx, formats[0], formats[1])
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutCreateMenuFormatRow(gtx, formats[2], formats[3])
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutCreateMenuFormatRow(gtx, formats[4], formats[5])
			})
		}),
	)
}

func (a *App) layoutCreateMenuFormatRow(gtx layout.Context, left, right createMenuFormat) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutCreateMenuFormatCard(gtx, left)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gap := gtx.Dp(unit.Dp(10))
			return layout.Dimensions{Size: image.Pt(gap, 0)}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutCreateMenuFormatCard(gtx, right)
		}),
	)
}

func (a *App) layoutCreateMenuFormatCard(gtx layout.Context, item createMenuFormat) layout.Dimensions {
	height := gtx.Dp(unit.Dp(68))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(item.label + " · " + item.extension).Add(gtx.Ops)
	semantic.EnabledOp(item.enabled).Add(gtx.Ops)
	if item.enabled {
		if item.click.Clicked(gtx) {
			a.createNewMenuDocument(item.id)
		}
		return item.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return a.layoutCreateMenuFormatCardVisual(gtx, item, item.click.Hovered())
		})
	}
	return a.layoutCreateMenuFormatCardVisual(gtx, item, false)
}

func (a *App) layoutCreateMenuFormatCardVisual(gtx layout.Context, item createMenuFormat, hovered bool) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			fill := color.NRGBA{R: 250, G: 251, B: 252, A: 255}
			border := color.NRGBA{R: 227, G: 231, B: 235, A: 255}
			if !item.enabled {
				fill = color.NRGBA{R: 246, G: 247, B: 248, A: 255}
				border = color.NRGBA{R: 235, G: 237, B: 239, A: 255}
			} else if hovered {
				fill = homeHoverFill
			}
			shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(9)))
			paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							size := gtx.Dp(unit.Dp(38))
							gtx.Constraints.Min = image.Pt(size, size)
							gtx.Constraints.Max = gtx.Constraints.Min
							return layout.Stack{Alignment: layout.Center}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									fill := item.accent
									if !item.enabled {
										fill = color.NRGBA{R: 223, G: 227, B: 231, A: 255}
									}
									shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(9)))
									paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
									return layout.Dimensions{Size: gtx.Constraints.Max}
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										tint := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
										if !item.enabled {
											tint = color.NRGBA{R: 153, G: 161, B: 170, A: 255}
										}
										return createMenuIcon(gtx, item.icon, 19, tint)
									})
								}),
							)
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								fg := color.NRGBA{R: 45, G: 52, B: 61, A: 255}
								if !item.enabled {
									fg = color.NRGBA{R: 145, G: 153, B: 162, A: 255}
								}
								return homeLabelStyle(a.theme, item.label, 13, fg, font.SemiBold).Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								fg := color.NRGBA{R: 146, G: 154, B: 164, A: 255}
								if !item.enabled {
									fg = color.NRGBA{R: 177, G: 183, B: 190, A: 255}
								}
								return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return homeLabelStyle(a.theme, item.extension, 10, fg, font.Normal).Layout(gtx)
								})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						fg := color.NRGBA{R: 176, G: 183, B: 192, A: 255}
						if !item.enabled {
							fg = color.NRGBA{R: 210, G: 214, B: 218, A: 255}
						}
						return homeTextLabel(gtx, a.theme, "›", 18, fg, false)
					}),
				)
			})
		},
	)
}

func createMenuIcon(gtx layout.Context, name string, size unit.Dp, tint color.NRGBA) layout.Dimensions {
	pixels := gtx.Dp(size)
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(pixels, pixels), image.Pt(pixels, pixels)
	icon, ok := svgIconTint("pen", name, tint)
	if !ok {
		return layout.Dimensions{Size: image.Pt(pixels, pixels)}
	}
	return widget.Image{
		Src:      paint.NewImageOp(icon),
		Fit:      widget.Contain,
		Position: layout.Center,
		Scale:    float32(size) / 96,
	}.Layout(gtx)
}

func (a *App) createNewMenuDocument(kind string) {
	a.showNewMenu = false
	switch kind {
	case "docx":
		word, err := docx.NewBlank()
		if err != nil {
			a.setStatus(err.Error())
		} else {
			_ = a.installDocument(loadedDocument{word: word})
		}
	case "xlsx":
		book := xlsx.NewBlank()
		_ = a.installDocument(loadedDocument{book: book, sheets: book.SheetNames()})
	case "markdown":
		_ = a.installDocument(loadedDocument{markdownDoc: markdowndoc.NewBlank()})
	case "json":
		_ = a.installDocument(loadedDocument{jsonDoc: jsondoc.NewBlank()})
	}
}

func (a *App) layoutCreateMenuDivider(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(1))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	paint.FillShape(gtx.Ops, color.NRGBA{R: 232, G: 235, B: 239, A: 255}, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
}

func (a *App) layoutCreateMenuOpenActions(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(unit.Dp(12))}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutCreateMenuOpenAction(gtx, &a.actionOpenFile, "打开本地文件", "folder-open", "⌘ O", false)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutCreateMenuOpenAction(gtx, &a.actionOpenFolder, "打开文件夹", "folder", "", true)
		}),
	)
}

func (a *App) layoutCreateMenuOpenAction(gtx layout.Context, click *widget.Clickable, label, icon, shortcut string, folder bool) layout.Dimensions {
	if click.Clicked(gtx) {
		a.showNewMenu = false
		if folder {
			a.startFolderPicker()
		} else {
			a.startOpenFilePicker()
		}
	}
	height := gtx.Dp(unit.Dp(42))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				fill := color.NRGBA{R: 242, G: 246, B: 251, A: 255}
				if click.Hovered() {
					fill = color.NRGBA{R: 235, G: 242, B: 250, A: 255}
				}
				shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(8)))
				paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								size := gtx.Dp(unit.Dp(26))
								gtx.Constraints.Min = image.Pt(size, size)
								gtx.Constraints.Max = gtx.Constraints.Min
								return layout.Stack{Alignment: layout.Center}.Layout(gtx,
									layout.Expanded(func(gtx layout.Context) layout.Dimensions {
										shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(6)))
										paint.FillShape(gtx.Ops, contentSurface, shape.Op(gtx.Ops))
										return layout.Dimensions{Size: gtx.Constraints.Max}
									}),
									layout.Stacked(func(gtx layout.Context) layout.Dimensions {
										return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return homeIcon(gtx, icon, 15, color.NRGBA{R: 52, G: 120, B: 209, A: 255})
										})
									}),
								)
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeLabelStyle(a.theme, label, 12, color.NRGBA{R: 49, G: 95, B: 140, A: 255}, font.SemiBold).Layout(gtx)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if shortcut == "" {
								return layout.Dimensions{}
							}
							gtx.Constraints.Min.Y = 0
							return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeLabelStyle(a.theme, shortcut, 10, color.NRGBA{R: 122, G: 143, B: 168, A: 255}, font.SemiBold).Layout(gtx)
							})
						}),
					)
				})
			},
		)
	})
}

type createActionItem struct {
	click     *widget.Clickable
	label     string
	icon      string
	separator bool
}

func (a *App) createActionItems() []createActionItem {
	buttons := []createActionItem{
		{click: &a.actionOpenFile, label: "打开本地文件", icon: "folder-open"},
		{click: &a.actionOpenFolder, label: "打开文件夹", icon: "folder"},
	}
	buttons = append(buttons,
		createActionItem{separator: true},
		createActionItem{click: &a.actionNewDOCX, label: "文档", icon: "file-text"},
		createActionItem{click: &a.actionNewXLSX, label: "表格", icon: "file-spreadsheet"},
		createActionItem{click: &a.actionNewJSON, label: "JSON 数据", icon: "file-code"},
		createActionItem{click: &a.actionNewMarkdown, label: "Markdown 文本", icon: "file-text"},
	)
	return buttons
}

func (a *App) createActionLabels() []string {
	items := a.createActionItems()
	labels := make([]string, 0, len(items))
	for _, item := range items {
		if !item.separator {
			labels = append(labels, item.label)
		}
	}
	return labels
}
