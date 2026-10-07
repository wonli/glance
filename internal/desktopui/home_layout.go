package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/markdowndoc"
	"glance/internal/format/pdfdoc"
	"glance/internal/format/xlsx"
)

type homeSection uint8

const (
	homeSectionRecent homeSection = iota
	homeSectionFavorites
)

type homeFileMetadata struct {
	modified    time.Time
	size        int64
	unavailable bool
}

type homeMetadataResult struct {
	generation uint64
	entries    map[string]homeFileMetadata
}

type homeAction struct {
	id        string
	label     string
	extension string
	icon      string
	iconColor color.NRGBA
	titleSize unit.Sp
	ai        bool
}

var homeActions = []homeAction{
	{id: "docx", label: "AI Docs", extension: ".docx", icon: "file-text", iconColor: color.NRGBA{R: 52, G: 120, B: 209, A: 255}, titleSize: 16, ai: true},
	{id: "xlsx", label: "AI Sheets", extension: ".xlsx", icon: "file-spreadsheet", iconColor: color.NRGBA{R: 79, G: 166, B: 107, A: 255}, titleSize: 16, ai: true},
	{id: "pptx", label: "AI Slides", extension: ".pptx", icon: "presentation", iconColor: color.NRGBA{R: 226, G: 59, B: 39, A: 255}, titleSize: 16, ai: true},
	{id: "md", label: "AI Markdown", extension: ".md", icon: "file-code", iconColor: color.NRGBA{R: 133, G: 88, B: 232, A: 255}, titleSize: 13, ai: true},
	{id: "json", label: "JSON", extension: ".json", icon: "braces", iconColor: color.NRGBA{R: 214, G: 154, B: 45, A: 255}, titleSize: 16},
	{id: "pdf", label: "AI PDF", extension: ".pdf", icon: "file", iconColor: color.NRGBA{R: 240, G: 68, B: 69, A: 255}, titleSize: 16, ai: true},
	{id: "open", label: "打开本地文件", extension: ".docx / .xlsx", icon: "folder-open", iconColor: color.NRGBA{R: 123, G: 134, B: 146, A: 255}, titleSize: 13},
}

var homeFilters = []string{"全部", "文档", "表格", "演示", "PDF", "Markdown", "JSON"}

const (
	homeSearchPlaceholder                 = "搜索文件名..."
	homeTextOpticalCenterTopInset unit.Dp = 6
)

var (
	homeCanvas        = color.NRGBA{R: 251, G: 251, B: 252, A: 255}
	homeSidebarFill   = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	homeSurface       = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	homeBorder        = color.NRGBA{R: 225, G: 229, B: 234, A: 255}
	homeSidebarBorder = color.NRGBA{R: 231, G: 233, B: 236, A: 255}
	homeRowBorder     = color.NRGBA{R: 238, G: 240, B: 242, A: 255}
	homeText          = color.NRGBA{R: 52, G: 59, B: 69, A: 255}
	homeMuted         = color.NRGBA{R: 138, G: 146, B: 157, A: 255}
	homeSelectedFill  = color.NRGBA{R: 241, G: 242, B: 244, A: 255}
	homeHoverFill     = color.NRGBA{R: 247, G: 248, B: 249, A: 255}
	homeFavoriteGold  = color.NRGBA{R: 176, G: 138, B: 55, A: 255}
)

func (a *App) refreshHomeMetadata() {
	a.ensureDesktopRuntime()
	if a.collection == nil {
		return
	}
	recents := a.collection.Recents()
	a.homeMetadataGeneration++
	generation := a.homeMetadataGeneration
	go func() {
		entries := make(map[string]homeFileMetadata, len(recents))
		for _, item := range recents {
			info, err := os.Stat(item.Path)
			if err != nil {
				entries[item.Path] = homeFileMetadata{unavailable: true}
				continue
			}
			entries[item.Path] = homeFileMetadata{modified: info.ModTime(), size: info.Size()}
		}
		select {
		case a.homeMetadataResults <- homeMetadataResult{generation: generation, entries: entries}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) layoutRecent(gtx layout.Context) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return fill(gtx, homeCanvas) },
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					width := gtx.Dp(unit.Dp(250))
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
					return a.layoutHomeSidebar(gtx)
				}),
				layout.Flexed(1, a.layoutHomeContent),
			)
		},
	)
}

func (a *App) layoutHomeSidebar(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	a.processHomeNavClicks(gtx)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, homeSidebarFill, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, homeSidebarBorder, clip.Rect{Min: image.Pt(gtx.Constraints.Max.X-1, 0), Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(28), Left: unit.Dp(18), Right: unit.Dp(18), Bottom: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							height := gtx.Dp(unit.Dp(42))
							gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
							return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return homeTextLabel(gtx, a.theme, "Glance", 20, color.NRGBA{R: 18, G: 22, B: 28, A: 255}, true)
									}),
								)
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutHomeNavRow(gtx, "recent", "clock-3", "最近", len(a.homeVisibleRecents(false)), a.homeSection == homeSectionRecent)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutHomeNavRow(gtx, "favorites", "star", "收藏", len(a.homeFavorites), a.homeSection == homeSectionFavorites)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(18), Bottom: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							paint.FillShape(gtx.Ops, homeSidebarBorder, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}.Op())
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							height := gtx.Dp(unit.Dp(30))
							gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
							return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.Y = 0
										return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return homeTextLabel(gtx, a.theme, "文件夹", 14, color.NRGBA{R: 125, G: 135, B: 146, A: 255}, true)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return homeSmallIconButton(gtx, &a.homeNewFolderButton, "folder-plus", 17, homeMuted, func() { a.setStatus("文件夹功能即将开放") })
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Left: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return homeSmallIconButton(gtx, &a.homeFolderAddButton, "plus", 17, homeMuted, func() { a.setStatus("文件夹功能即将开放") })
										})
									}),
								)
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutHomeFolderRow(gtx) }),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
				)
			})
		},
	)
}

func (a *App) homeNavButton(id string) *widget.Clickable {
	if a.homeNavButtons == nil {
		a.homeNavButtons = make(map[string]*widget.Clickable)
	}
	button := a.homeNavButtons[id]
	if button == nil {
		button = new(widget.Clickable)
		a.homeNavButtons[id] = button
	}
	return button
}

func (a *App) processHomeNavClicks(gtx layout.Context) {
	for _, navigation := range []struct {
		id      string
		section homeSection
	}{
		{id: "recent", section: homeSectionRecent},
		{id: "favorites", section: homeSectionFavorites},
	} {
		if a.homeNavButton(navigation.id).Clicked(gtx) {
			a.homeSection = navigation.section
		}
	}
}

func (a *App) layoutHomeNavRow(gtx layout.Context, id, glyph, label string, count int, selected bool) layout.Dimensions {
	button := a.homeNavButton(id)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(42))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bg := color.NRGBA{A: 0}
				if selected {
					bg = homeSelectedFill
				} else if button.Hovered() {
					bg = homeHoverFill
				}
				shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(10)))
				paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return homeIcon(gtx, glyph, 20, color.NRGBA{R: 63, G: 71, B: 82, A: 255})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							return layout.Inset{Left: unit.Dp(12), Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, label, 15, homeText, selected)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							countText := fmt.Sprintf("%d", count)
							gtx.Constraints.Min.Y = 0
							return layout.Inset{Left: unit.Dp(12), Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, countText, 13, homeMuted, false)
							})
						}),
					)
				})
			},
		)
	})
}

func (a *App) layoutHomeFolderRow(gtx layout.Context) layout.Dimensions {
	if a.homeFolderButton.Clicked(gtx) {
		a.setStatus("Glance 文件夹")
	}
	return a.homeFolderButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(36))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeIcon(gtx, "chevron-down", 16, homeMuted)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeIcon(gtx, "folder", 20, color.NRGBA{R: 60, G: 70, B: 82, A: 255})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeTextLabel(gtx, a.theme, "Glance", 15, homeText, false)
						})
					})
				}),
			)
		})
	})
}

func (a *App) layoutHomeContent(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(68), Left: unit.Dp(50), Right: unit.Dp(50)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return a.homeContentList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return a.layoutHomeContentPage(gtx)
		})
	})
}

func (a *App) layoutHomeContentPage(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutHomeGreeting(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(28)}.Layout(gtx, a.layoutHomeActions)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(28)}.Layout(gtx, a.layoutHomeFilters)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, a.layoutHomeRecentTable)
		}),
	)
}

func (a *App) layoutHomeGreeting(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(78))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	gtx.Execute(op.InvalidateCmd{At: nextHomeGreetingChange(gtx.Now)})
	style := material.H4(a.theme, homeGreetingForTime(gtx.Now)+"\n今天想创建点什么？")
	style.TextSize = unit.Sp(34)
	style.LineHeightScale = 1.15
	style.MaxLines = 2
	style.Color = color.NRGBA{R: 35, G: 40, B: 48, A: 255}
	style.Font.Weight = font.Bold
	return style.Layout(gtx)
}

func nextHomeGreetingChange(now time.Time) time.Time {
	for _, hour := range [...]int{5, 12, 14, 18} {
		change := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
		if change.After(now) {
			return change
		}
	}
	tomorrow := now.AddDate(0, 0, 1)
	return time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, now.Location())
}

func homeGreetingForTime(now time.Time) string {
	switch hour := now.Hour(); {
	case hour < 5:
		return "凌晨好"
	case hour < 12:
		return "早上好"
	case hour < 14:
		return "中午好"
	case hour < 18:
		return "下午好"
	default:
		return "晚上好"
	}
}

func (a *App) layoutHomeActions(gtx layout.Context) layout.Dimensions {
	const actionGap = unit.Dp(14)
	const narrowCardMinWidth = unit.Dp(120)
	width := gtx.Constraints.Max.X
	columns := (width + gtx.Dp(actionGap)) / (gtx.Dp(narrowCardMinWidth) + gtx.Dp(actionGap))
	if columns < 1 {
		columns = 1
	}
	if columns > len(homeActions) {
		columns = len(homeActions)
	}
	rows := (len(homeActions) + columns - 1) / columns
	height := gtx.Dp(unit.Dp(132))*rows + gtx.Dp(actionGap)*(rows-1)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	rowChildren := make([]layout.FlexChild, 0, rows)
	for row := 0; row < rows; row++ {
		start, end := homeActionRowRange(row, columns, len(homeActions))
		bottom := unit.Dp(0)
		if row < rows-1 {
			bottom = actionGap
		}
		rowChildren = append(rowChildren, a.homeActionRow(start, end, columns, bottom))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rowChildren...)
}

func homeActionRowRange(row, columns, total int) (start, end int) {
	start = row * columns
	end = start + columns
	if end > total {
		end = total
	}
	return start, end
}

func (a *App) homeActionRow(start, end, columns int, bottom unit.Dp) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: bottom}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gap := gtx.Dp(unit.Dp(14))
			cardWidth := (gtx.Constraints.Max.X - gap*(columns-1)) / columns
			children := make([]layout.FlexChild, 0, end-start+1)
			for index := start; index < end; index++ {
				item := homeActions[index]
				lastInRow := index == end-1
				outerWidth := cardWidth
				if !lastInRow {
					outerWidth += gap
				}
				children = append(children, a.homeActionCardChild(item, cardWidth, outerWidth, !lastInRow))
			}
			if end-start < columns {
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{}
				}))
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
		})
	})
}

func (a *App) homeActionCardChild(item homeAction, cardWidth, outerWidth int, withGap bool) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = outerWidth, outerWidth
		right := unit.Dp(0)
		if withGap {
			right = unit.Dp(14)
		}
		return layout.Inset{Right: right}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = cardWidth, cardWidth
			return a.layoutHomeActionCard(gtx, item)
		})
	})
}

func (a *App) layoutHomeActionCard(gtx layout.Context, item homeAction) layout.Dimensions {
	button := a.homeActionButtons[item.id]
	if button == nil {
		button = new(widget.Clickable)
		a.homeActionButtons[item.id] = button
	}
	if button.Clicked(gtx) {
		a.handleHomeAction(item.id)
	}
	return homeActionCard(gtx, a.theme, button, item)
}

func (a *App) handleHomeAction(id string) {
	switch id {
	case "docx":
		word, err := docx.NewBlank()
		if err != nil {
			a.setStatus(err.Error())
			return
		}
		_ = a.installDocument(loadedDocument{word: word})
	case "json":
		_ = a.installDocument(loadedDocument{jsonDoc: jsondoc.NewBlank()})
	case "xlsx":
		book := xlsx.NewBlank()
		_ = a.installDocument(loadedDocument{book: book, sheets: book.SheetNames()})
	case "open":
		a.startOpenFilePicker()
	case "pptx":
		a.setStatus("演示文稿创建功能尚未开放")
	case "md":
		_ = a.installDocument(loadedDocument{markdownDoc: markdowndoc.NewBlank()})
	case "html":
		a.setStatus("HTML 创建功能尚未开放")
	case "pdf":
		doc, err := pdfdoc.NewBlank()
		if err != nil {
			a.setStatus(err.Error())
			return
		}
		_ = a.installDocument(loadedDocument{pdfDoc: doc})
	}
}

func (a *App) layoutHomeFilters(gtx layout.Context) layout.Dimensions {
	width := gtx.Dp(unit.Dp(1090))
	if width > gtx.Constraints.Max.X {
		width = gtx.Constraints.Max.X
	}
	if width < gtx.Dp(unit.Dp(760)) {
		return a.layoutHomeFiltersCompact(gtx, width)
	}
	scale := float32(width) / float32(gtx.Dp(unit.Dp(1090)))
	if scale <= 0 {
		scale = 1
	}
	height := gtx.Dp(unit.Dp(44 * scale))
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	items := a.homeVisibleRecents(true)
	countText := "最近使用"
	if a.homeSection == homeSectionFavorites {
		countText = "收藏"
	}
	countText = fmt.Sprintf("%s  %d 个文件", countText, len(items))
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeFilterGroup(gtx, a.theme, a.homeFilterButtons, &a.homeTypeFilter, scale, func(filter string) {
				a.homeTypeFilter = filter
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return layout.Inset{Left: unit.Dp(18 * scale)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeSearchField(gtx, a.theme, &a.homeSearch, 240*scale, 44*scale)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.homeSortButton.Clicked(gtx) {
				a.homeSortOpenedDesc = !a.homeSortOpenedDesc
			}
			return layout.Inset{Left: unit.Dp(18 * scale)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeSortButton(gtx, &a.homeSortButton, 20*scale, 44*scale)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return layout.Inset{Left: unit.Dp(18 * scale), Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeRecentCountStyle(a.theme, countText, scale).Layout(gtx)
			})
		}),
	)
}

func (a *App) layoutHomeFiltersCompact(gtx layout.Context, width int) layout.Dimensions {
	scale := float32(width) / float32(gtx.Dp(unit.Dp(650)))
	if scale > 1 {
		scale = 1
	}
	if scale <= 0 {
		scale = 1
	}
	rowHeightDp := 44 * scale
	rowHeight := gtx.Dp(unit.Dp(rowHeightDp))
	rowGap := gtx.Dp(unit.Dp(8))
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = rowHeight*2+rowGap, rowHeight*2+rowGap
	items := a.homeVisibleRecents(true)
	countText := "最近使用"
	if a.homeSection == homeSectionFavorites {
		countText = "收藏"
	}
	countText = fmt.Sprintf("%s  %d 个文件", countText, len(items))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeFilterGroup(gtx, a.theme, a.homeFilterButtons, &a.homeTypeFilter, scale, func(filter string) {
						a.homeTypeFilter = filter
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return homeSearchField(gtx, a.theme, &a.homeSearch, 0, rowHeightDp)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(10 * scale)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if a.homeSortButton.Clicked(gtx) {
								a.homeSortOpenedDesc = !a.homeSortOpenedDesc
							}
							return homeSortButton(gtx, &a.homeSortButton, 20*scale, rowHeightDp)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						return layout.Inset{Left: unit.Dp(10 * scale), Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeRecentCountStyle(a.theme, countText, scale).Layout(gtx)
						})
					}),
				)
			})
		}),
	)
}
