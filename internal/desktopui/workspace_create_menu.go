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
	"glance/internal/format/pdfdoc"
	"glance/internal/format/pptx"
	"glance/internal/format/xlsx"
	"glance/internal/i18n"
)

const (
	createMenuWidth  unit.Dp = 480
	createMenuHeight unit.Dp = 390
)

func (a *App) layoutCreateMenuPopover(gtx layout.Context) layout.Dimensions {
	if a.createMenuScrim.Clicked(gtx) {
		a.showNewMenu = false
	}
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.createMenuScrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Max
				semantic.Button.Add(gtx.Ops)
				semantic.DescriptionOp(a.text(i18n.Message{ID: "home.create.close_menu"})).Add(gtx.Ops)
				return layout.Dimensions{Size: size}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			left := a.plusMenuOffset(gtx)
			plusX := a.plusButtonOffset(gtx)
			shift := op.Offset(image.Pt(left, 0)).Push(gtx.Ops)
			dims := a.layoutCreateActions(gtx, plusX-left)
			shift.Pop()
			return dims
		}),
	)
}

func (a *App) plusButtonOffset(gtx layout.Context) int {
	ids := a.workspaceTabIDs()
	plusIndex := len(ids) + 1
	x := -a.workspaceTabsList.Position.Offset
	for index := a.workspaceTabsList.Position.First; index < plusIndex; index++ {
		if index == 0 {
			x += gtx.Dp(a.workspaceHomeTabWidth())
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
	return x
}

func (a *App) plusMenuOffset(gtx layout.Context) int {
	x := a.plusButtonOffset(gtx)
	menuWidth := createMenuWidthPx(gtx)
	margin := gtx.Dp(unit.Dp(8))
	maxX := gtx.Constraints.Max.X - menuWidth - margin
	if maxX < margin {
		maxX = margin
	}
	plusWidth := gtx.Dp(workspacePlusTabWidth)
	if x+plusWidth > maxX+menuWidth {
		neededLeft := x + plusWidth - menuWidth
		windowMax := gtx.Constraints.Max.X - menuWidth
		if neededLeft > windowMax {
			neededLeft = windowMax
		}
		if neededLeft > maxX {
			maxX = neededLeft
		}
	}
	if x > maxX {
		x = maxX
	}
	if x < margin {
		x = margin
	}
	return x
}

type createMenuConnectorGeometry struct {
	MinX int
	MaxX int
	NW   int
	NE   int
}

func calculateCreateMenuConnector(bounds image.Rectangle, plusRelX, plusWidth, radius, seamCover int) createMenuConnectorGeometry {
	menuWidth := bounds.Max.X
	start := plusRelX
	end := plusRelX + plusWidth

	if start < 0 {
		start = 0
	}
	if end > menuWidth {
		end = menuWidth
	}
	if end < start {
		end = start
	}

	nw := radius
	if start <= 0 {
		nw = 0
	} else if start < radius {
		nw = start
	}

	ne := radius
	rightMargin := menuWidth - end
	if rightMargin <= 0 {
		ne = 0
	} else if rightMargin < radius {
		ne = rightMargin
	}

	minX := max(0, start)
	maxX := min(menuWidth, end)
	if start <= 0 {
		maxX = min(menuWidth, end+seamCover)
	} else if end >= menuWidth {
		minX = max(0, start-seamCover)
	}

	return createMenuConnectorGeometry{
		MinX: minX,
		MaxX: maxX,
		NW:   nw,
		NE:   ne,
	}
}

func (a *App) layoutCreateActions(gtx layout.Context, plusRelX int) layout.Dimensions {
	width := createMenuWidthPx(gtx)
	height := gtx.Dp(createMenuHeight)
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	for a.createMenuCardClick.Clicked(gtx) {
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			radius := gtx.Dp(unit.Dp(12))
			seamCover := gtx.Dp(unit.Dp(1))
			plusWidth := gtx.Dp(workspacePlusTabWidth)
			conn := calculateCreateMenuConnector(bounds, plusRelX, plusWidth, radius, seamCover)

			shadowBounds := bounds.Add(image.Pt(0, gtx.Dp(unit.Dp(5))))
			shadow := clip.RRect{Rect: shadowBounds, NW: conn.NW, NE: conn.NE, SE: radius, SW: radius}
			paint.FillShape(gtx.Ops, color.NRGBA{R: 31, G: 41, B: 55, A: 28}, shadow.Op(gtx.Ops))

			panel := clip.RRect{Rect: bounds, NW: conn.NW, NE: conn.NE, SE: radius, SW: radius}
			paint.FillShape(gtx.Ops, contentSurface, panel.Op(gtx.Ops))
			paintRoundedBorder(gtx.Ops, panel, float32(gtx.Dp(unit.Dp(1))), homeBorder)

			connector := clip.Rect{
				Min: image.Pt(conn.MinX, -seamCover),
				Max: image.Pt(conn.MaxX, seamCover),
			}
			paint.FillShape(gtx.Ops, contentSurface, connector.Op())
			return layout.Dimensions{Size: bounds.Max}
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.createMenuCardClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Min}
			})
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
					return homeLabelStyle(a.theme, a.text(i18n.Message{ID: "home.create.title"}), 16, color.NRGBA{R: 31, G: 37, B: 45, A: 255}, font.SemiBold).Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeLabelStyle(a.theme, a.text(i18n.Message{ID: "home.create.subtitle"}), 11, color.NRGBA{R: 122, G: 131, B: 142, A: 255}, font.Normal).Layout(gtx)
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
							return homeIcon(gtx, "x", 16, color.NRGBA{R: 104, G: 113, B: 124, A: 255})
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
	labelRef  i18n.Message
	extension string
	icon      string
	accent    color.NRGBA
	enabled   bool
}

// File-type glyphs are SVG exports of the matching icon nodes in Pen frame XGCJN.
func (a *App) createMenuFormats() []createMenuFormat {
	return []createMenuFormat{
		{click: &a.actionNewDOCX, id: "docx", label: "文档", labelRef: i18n.Message{ID: "home.create.format.docx"}, extension: "DOCX", icon: "docx", accent: a.tokens().Colors.OfficeAccent.Word.Accent, enabled: true},
		{click: &a.actionNewXLSX, id: "xlsx", label: "表格", labelRef: i18n.Message{ID: "home.create.format.xlsx"}, extension: "XLSX", icon: "xlsx", accent: a.tokens().Colors.OfficeAccent.Spreadsheet.Accent, enabled: true},
		{click: &a.actionNewPPTX, id: "pptx", label: "演示文稿", labelRef: i18n.Message{ID: "home.create.format.pptx"}, extension: "PPTX", icon: "pptx", accent: color.NRGBA{R: 228, G: 130, B: 60, A: 255}, enabled: true},
		{click: &a.actionNewMarkdown, enabled: true, id: "markdown", label: "Markdown", labelRef: i18n.Message{ID: "home.create.format.markdown"}, extension: "MD", icon: "markdown", accent: color.NRGBA{R: 138, G: 90, B: 194, A: 255}},
		{click: &a.actionNewJSON, id: "json", label: "JSON 数据", labelRef: i18n.Message{ID: "home.create.format.json"}, extension: "JSON", icon: "json", accent: color.NRGBA{R: 43, G: 157, B: 152, A: 255}, enabled: true},
		{click: &a.actionNewPDF, id: "pdf", label: "PDF 文档", labelRef: i18n.Message{ID: "home.create.format.pdf"}, extension: "PDF", icon: "pdf", accent: color.NRGBA{R: 240, G: 68, B: 69, A: 255}, enabled: true},
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
	item.label = a.messageText(item.labelRef, item.label)
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
			paintRoundedBorder(gtx.Ops, shape, float32(gtx.Dp(unit.Dp(1))), border)
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
						return homeIcon(gtx, "chevron-right", 16, fg)
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
		Src:      iconImageOp(icon),
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
			a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
		} else {
			_ = a.installDocument(loadedDocument{word: word})
		}
	case "xlsx":
		book := xlsx.NewBlank()
		_ = a.installDocument(loadedDocument{book: book, sheets: book.SheetNames()})
	case "pptx":
		doc, err := pptx.NewBlank()
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
		} else {
			_ = a.installDocument(loadedDocument{pptxDoc: doc})
		}
	case "markdown":
		_ = a.installDocument(loadedDocument{markdownDoc: markdowndoc.NewBlank()})
	case "json":
		_ = a.installDocument(loadedDocument{jsonDoc: jsondoc.NewBlank()})
	case "pdf":
		doc, err := pdfdoc.NewBlank()
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
		} else {
			_ = a.installDocument(loadedDocument{pdfDoc: doc})
		}
	}
}

func (a *App) layoutCreateMenuDivider(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(1))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	paint.FillShape(gtx.Ops, color.NRGBA{R: 232, G: 235, B: 239, A: 255}, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
}

type createActionItem struct {
	click     *widget.Clickable
	label     string
	labelRef  i18n.Message
	icon      string
	separator bool
}
